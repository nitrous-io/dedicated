package tender

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"sync"

	proxyerror "github.com/nitrous-io/dedicated/pkg/errorpages/proxy"
	"github.com/nitrous-io/dedicated/pkg/proxy"
)

var (
	errHostNotFound = errors.New("Host not found")
	hostsCache      HostsMap
	muHosts         sync.Mutex
	// serves as identity to allow caching failed lookups
	hostNotFound = &Host{}
)

// FreighterProxyServer let user proxy requests to the freighterd host. This is
// useful when the client want to access the freighterd's API without having to
// instal its SSL certificates for example.
type FreighterProxyServer struct {
	// direct access to database so we can fetch stuffs as fast as possible
	db *sql.DB
	// expected DNS based domain for host's request. Request coming from this
	// domain will be proxied to freighterd server on that host.
	domain string
	// expected domain that is used when we create the TLS certificates in webapp
	// `nitrouspro.com`. This is used when communicating with the freighterd
	// server on the host.
	tlsDomain string
	// default tls config to talk to remote freighterd server. whenever we
	// proxy to a server, we have to copy this and set a server name
	tlsConfig *tls.Config

	// ports specified in this map will be forwarded.
	// E.g. an entry of 443: 65333 will forward port 443 to 65333.
	portMap map[int]int
}

func RefreshHostsCache(db *sql.DB) error {
	m, err := getHosts(db)
	if err != nil {
		return err
	}
	muHosts.Lock()
	hostsCache = m
	muHosts.Unlock()
	return nil
}

func hostGet(slug string) (*Host, bool) {
	muHosts.Lock()
	h, ok := hostsCache[slug]
	muHosts.Unlock()
	return h, ok
}

func hostSet(slug string, host *Host) {
	muHosts.Lock()
	hostsCache[slug] = host
	muHosts.Unlock()
}

// NewFreighterProxyServer returns a `http.Handler` which can be used to proxy
// requests to a freighterd server on a Nitrous.IO host
func NewFreighterProxyServer(baseDomain, tlsDomain string, portMap map[int]int,
	db *sql.DB, caBytes []byte) http.Handler {

	s := &FreighterProxyServer{
		db:        db,
		domain:    baseDomain,
		tlsDomain: tlsDomain,
		portMap:   portMap,
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

// ServerHTTP lets FreighterProxyServer acts as a http handler
func (s *FreighterProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.proxy(w, r); err != nil {
		handleError(w, r, err)
		return
	}
}

// proxy the request to the right boxhost. We will switch the proxy strategy
// depending on the type of the request
// - if this is a normal HTTP, use `http.ReverseProxy`
// - if the request wants to open a websocket, use `proxyWebsocket`
func (s *FreighterProxyServer) proxy(w http.ResponseWriter, r *http.Request) (err error) {
	slug, err := getHostname(r.Host, s.domain)
	if err != nil {
		log.Printf("[WARN] domain: %s but got request for %q that we can't handle", s.domain, r.Host)
		return err
	}
	log.Printf("[INFO] request %v to %s", r, slug)

	fp := parsePort(r)
	if p, ok := s.portMap[fp]; ok {
		fp = p
	}

	// get host id and host ip
	// host ip will be the new destination for current request
	host, ok := hostGet(slug)
	if !ok {
		host, err = findHost(s.db, slug)
		if err != nil {
			host = hostNotFound
		}
		hostSet(slug, host)
	}
	if host == hostNotFound {
		return errHostNotFound
	}

	dest := hostToIP(r, host.IP, host.Port)

	// copy tls config and set serverName
	// serverName is the name we use to generate private certs on the host
	// this must be the same as Hostname in
	// https://github.com/nitrous-io/nitrous/blob/master/models/host/host.go
	var tlsConfig *tls.Config
	if s.tlsConfig != nil {
		tlsConfig = &tls.Config{}
		*tlsConfig = *s.tlsConfig
		tlsConfig.ServerName = slug + s.tlsDomain
	}

	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(dest, tlsConfig)(w, r)
	} else {
		proxy.ProxyHTTP(dest, tlsConfig, proxyerror.ErrorHandler)(w, r)
	}
	return nil
}
