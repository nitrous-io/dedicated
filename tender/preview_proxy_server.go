package tender

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	_ "github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/pkg/proxy"
)

// PreviewProxyServer let user proxy preview requests to the host.
// The server is awared of the exposed ports on the host and proxy requests
// accordingly
type PreviewProxyServer struct {
	// direct access to database so we can fetch stuffs as fast as possible
	// FIXME:
	// - considering replace this with distributed datastore such as etcd
	db *sql.DB
	// expected DNS based domain for host's request. Request coming from this
	// domain will be proxied to freighterd server on that host.
	domain string
	// expected domain that is used when we create the TLS certificates in webapp
	// `nitrouspro.com`. This is used when communicating with the freighterd
	// server on the host.
	tlsDomain string
	// store the CA so we can build freighterclient
	ca []byte
}

// Returns a `http.Handler` which can be used to proxy preview request to
// a Nitrous.IO host
func NewPreviewProxyServer(baseDomain, tlsDomain string,
	db *sql.DB, caBytes []byte) (http.Handler, error) {
	s := &PreviewProxyServer{
		db:        db,
		domain:    baseDomain,
		tlsDomain: tlsDomain,
		ca:        caBytes,
	}
	return s, nil
}

func (s *PreviewProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.proxy(w, r); err != nil {
		handleError(w, err)
		return
	}
}

type containerWithPorts struct {
	*Container
	ports map[string][]string
}

// Naïve cache - replace with something like groupcache.
type containerCache struct {
	lock sync.RWMutex
	data map[string]containerWithPorts
}

func newContainerCache() *containerCache {
	return &containerCache{
		lock: sync.RWMutex{},
		data: make(map[string]containerWithPorts),
	}
}

func (c *containerCache) Get(slug string) (containerWithPorts, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	cn, ok := c.data[slug]
	return cn, ok
}

func (c *containerCache) Set(slug string, cn containerWithPorts) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.data[slug] = cn
}

func (c *containerCache) Delete(slug string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.data, slug)
}

var cnCache = newContainerCache()

func (s *PreviewProxyServer) proxy(w http.ResponseWriter, r *http.Request) (err error) {
	slug, err := getHostname(r.Host, s.domain)
	if err != nil {
		return err
	}

	defer func() {
		// Delete container from cache if there's any error.
		if err != nil {
			cnCache.Delete(slug)
		}
	}()
	log.Printf("[INFO] request %v to %s", r, slug)

	var container containerWithPorts
	// FIXME Evict container from cache when container is terminated.
	container, ok := cnCache.Get(slug)
	if !ok {
		cn, err := findContainer(s.db, slug)
		if err != nil {
			return err
		}

		fhost := freighterclient.NewHost(
			cn.HostIP.String(),
			cn.HostPort,
			cn.HostSlug+s.tlsDomain,
			s.ca,
		)
		fclient, err := freighterclient.NewClient(fhost, cn.HostSecret)
		if err != nil {
			return fmt.Errorf("failed to initialize freighter client for %s: %v", cn.Slug, err)
		}
		info, err := fclient.Info(cn.CID)
		if err != nil {
			return fmt.Errorf("failed to fetch container info %s: %v", cn.Slug, err)
		}

		container = containerWithPorts{
			Container: cn,
			ports:     info.Ports,
		}
		cnCache.Set(slug, container)
	}

	requestedPort := strconv.Itoa(parsePort(r))
	publicPort, err := getPublicFreighterdPort(container.ports, requestedPort)
	if err != nil {
		return err
	}

	// set X-FORWARDED-PORT for the request
	r.Header.Set(config.ForwardPortHeader, requestedPort)
	dest := setRequestHost(r, container.HostIP, publicPort)

	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(dest, nil)(w, r)
	} else {
		proxy.ProxyHTTP(dest, nil)(w, r)
	}
	return nil
}
