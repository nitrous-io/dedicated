package tender

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	proxyerror "github.com/nitrous-io/dedicated/pkg/errorpages/proxy"
	"github.com/nitrous-io/dedicated/pkg/proxy"
)

const (
	previewPortEnvVar = "NITROUS_PREVIEW_PORT"
)

var (
	// Requests on these ports will be mapped to the NITROUS_PREVIEW_PORT.
	// E.g. if preview is running on port 3000, requests on port 443 will be treated
	// as if they were made on port 3000.
	previewForwardPorts = []int{80, 443}

	errContainerNotFound      = errors.New("Host not found")
	containersCache           ContainersMap
	containersCacheLastUpdate time.Time
	muContainers              sync.Mutex
	// serves as identity to allow caching failed lookups
	containerNotFound = &Container{}
)

// PreviewProxyServer let user proxy preview requests to the host.
// The server is awared of the exposed ports on the host and proxy requests
// accordingly
type PreviewProxyServer struct {
	// direct access to database so we can fetch stuffs as fast as possible
	db *sql.DB
	// expected DNS based domain for host's request. Request coming from this
	// domain will be proxied to freighterd server on that host.
	domain string
	// expected domain that is used when we create the TLS certificates in webapp
	// `nitrouspro.com`. This is used when communicating with the freighterd
	// server on the host.
	tlsDomain string
	// default tls config to talk to remote freighterd server.
	tlsConfig *tls.Config
	// store the CA so we can build freighterclient
	ca []byte

	freighterdPort int
}

func RefreshContainersCache(db *sql.DB) error {
	timeStart := time.Now()
	m, err := getContainers(db)
	if err != nil {
		return err
	}
	n := len(m)
	muContainers.Lock()
	containersCache = m
	muContainers.Unlock()
	containersCacheLastUpdate = time.Now()
	LogVerbosef("got %d containers in %s\n", n, time.Since(timeStart))
	return nil
}

func DumpContainersCache() {
	muContainers.Lock()
	defer muContainers.Unlock()
	log.Printf("containersCache: %d containers, last updated %s ago\n", len(containersCache), time.Since(containersCacheLastUpdate))
	for slug, c := range containersCache {
		log.Printf(" %s: %s:%d\n", slug, c.HostIP, c.HostPort)
	}
}

func containerCacheGet(slug string) (*Container, bool) {
	muContainers.Lock()
	c, ok := containersCache[slug]
	muContainers.Unlock()
	return c, ok
}

func containerCacheSet(slug string, c *Container) {
	muContainers.Lock()
	containersCache[slug] = c
	muContainers.Unlock()
}

func containerGet(db *sql.DB, slug string) (*Container, error) {
	var err error
	cn, ok := containerCacheGet(slug)
	if !ok {
		cn, err = findContainer(db, slug)
		if err != nil {
			cn = containerNotFound
		}
		containerCacheSet(slug, cn)
	}
	if cn == containerNotFound {
		return nil, errContainerNotFound
	}
	return cn, nil
}

// NewPreviewProxyServer returns a `http.Handler` which can be used to proxy
// preview request to` a Nitrous.IO host
func NewPreviewProxyServer(baseDomain, tlsDomain string, freighterdPort int,
	db *sql.DB, caBytes []byte) http.Handler {

	s := &PreviewProxyServer{
		tlsDomain:      tlsDomain,
		domain:         baseDomain,
		freighterdPort: freighterdPort,
		db:             db,
		ca:             caBytes,
	}
	// set the default tls config for proxied connections
	if len(caBytes) > 0 {
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caBytes)
		s.tlsConfig = &tls.Config{
			RootCAs: certPool,
		}
	}
	return s
}

func (s *PreviewProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.proxy(w, r); err != nil {
		handleError(w, r, err)
		return
	}
}

type containerWithInfo struct {
	*Container
	ports map[string][]string
	env   []string
}

// try CNAME resolve host and do getHostname() on resolved name (if any)
func getCNAMEHostname(host, domain string) (string, error) {
	var err error
	if strings.Contains(host, ":") {
		host, _, err = net.SplitHostPort(host)
		if err != nil {
			return "", err
		}
	}
	cname, err := resolveDNSCname(host)
	if err != nil {
		return "", err
	}
	return getHostname(cname.Target, domain)
}

func (s *PreviewProxyServer) proxy(w http.ResponseWriter, r *http.Request) (err error) {
	slug, err := getHostname(r.Host, s.domain)
	if err != nil {
		slug, err = getCNAMEHostname(r.Host, s.domain)
		if err != nil {
			log.Printf("[WARN] domain: %s but got request for %q that we can't handle", s.domain, r.Host)
			return err
		}
	}

	log.Printf("[INFO] request %+v to %s", r, slug)

	cn, err := containerGet(s.db, slug)
	if err != nil {
		return err
	}

	fhost := freighterclient.NewHost(
		cn.HostIP.String(),
		cn.HostPort,
		cn.HostSlug+s.tlsDomain,
		s.ca,
	)
	// TODO: extract and pass UUID?
	fclient, err := freighterclient.NewClient(fhost, cn.HostSecret, "")
	if err != nil {
		return fmt.Errorf("failed to initialize freighter client for %s: %v", cn.Slug, err)
	}
	info, err := fclient.Info(cn.CID)
	if err != nil {
		return fmt.Errorf("failed to fetch container info %s: %v", cn.Slug, err)
	}

	container := containerWithInfo{
		Container: cn,
		ports:     info.Ports,
		env:       info.Env,
	}

	incomingPort := parsePort(r)
	containerPort, err := s.getContainerPort(container, incomingPort)
	if err != nil {
		return err
	}

	// set X-FORWARDED-PORT for the request
	r.Header.Set(freighter.ForwardPortHeader, containerPort)
	destAddr := net.JoinHostPort(container.HostIP.String(), containerPort)

	// For TLS requests, we proxy to Freighter on the host, which will then
	// proxy to the container.
	var tlsConfig *tls.Config
	if r.TLS != nil && s.tlsConfig != nil {
		tlsConfig = &tls.Config{}
		*tlsConfig = *s.tlsConfig
		tlsConfig.ServerName = slug + s.tlsDomain
		destAddr = net.JoinHostPort(container.HostIP.String(), strconv.Itoa(s.freighterdPort))
	}

	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(destAddr, tlsConfig)(w, r)
	} else {
		proxy.ProxyHTTP(destAddr, tlsConfig, proxyerror.ErrorHandler)(w, r)
	}
	return nil
}

func (s *PreviewProxyServer) getContainerPort(container containerWithInfo, port int) (string, error) {
	// Try to map requests on previewForwardPorts to the port specified by the
	// container's NITROUS_PREVIEW_PORT env var.
	for _, p := range previewForwardPorts {
		if port != p {
			continue
		}
		if previewPort, ok := envValue(container.env, previewPortEnvVar); ok {
			p, err := getPublicFreighterdPort(container.ports, previewPort)
			if err == nil {
				return p, nil
			}
		}
	}

	return getPublicFreighterdPort(container.ports, strconv.Itoa(port))
}

func envValue(env []string, envVar string) (string, bool) {
	for _, val := range env {
		split := strings.SplitN(val, "=", 2)
		if len(split) == 2 && split[0] == envVar {
			return split[1], true
		}
	}
	return "", false
}
