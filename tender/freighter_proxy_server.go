package tender

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/deferpanic/deferclient/deferstats"
	proxyerror "github.com/nitrous-io/dedicated/pkg/errorpages/proxy"
	"github.com/nitrous-io/dedicated/pkg/proxy"
)

var (
	errHostNotFound      = errors.New("Host not found")
	hostsCache           HostsMap
	hostsCacheLastUpdate time.Time
	muHosts              sync.Mutex
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

	// deferpanic stats client for error handling
	deferstatsClient *deferstats.Client
}

func RefreshHostsCache(db *sql.DB) error {
	timeStart := time.Now()
	m, err := getHosts(db)
	if err != nil {
		return err
	}
	n := len(m)
	muHosts.Lock()
	for slug, _ := range hostsCache {
		if _, ok := m[slug]; !ok {
			m[slug] = hostNotFound
		}
	}
	hostsCache = m
	muHosts.Unlock()
	hostsCacheLastUpdate = time.Now()
	LogVerbosef("got %d hosts in %s", n, time.Since(timeStart))
	return nil
}

func hostCacheGet(slug string) (*Host, bool) {
	muHosts.Lock()
	h, ok := hostsCache[slug]
	muHosts.Unlock()
	return h, ok
}

func hostCacheSet(slug string, host *Host) {
	muHosts.Lock()
	hostsCache[slug] = host
	muHosts.Unlock()
}

func hostGet(db *sql.DB, slug string) (*Host, error) {
	var err error
	// get host id and host ip
	// host ip will be the new destination for current request
	host, ok := hostCacheGet(slug)
	if !ok {
		host, err = findHost(db, slug)
		if err != nil {
			host = hostNotFound
		}
		hostCacheSet(slug, host)
	}
	if host == hostNotFound {
		return nil, errHostNotFound
	}
	return host, nil
}

// NewFreighterProxyServer returns a `http.Handler` which can be used to proxy
// requests to a freighterd server on a Nitrous.IO host
func NewFreighterProxyServer(baseDomain, tlsDomain string,
	db *sql.DB, caBytes []byte, deferstatsClient *deferstats.Client) http.Handler {
	s := &FreighterProxyServer{
		db:               db,
		domain:           baseDomain,
		tlsDomain:        tlsDomain,
		deferstatsClient: deferstatsClient,
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
		if s.deferstatsClient != nil {
			s.deferstatsClient.Wrap(err)
		}
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
		logrus.Warnf("domain: %s but got request for %q that we can't handle", s.domain, r.Host)
		return err
	}
	logrus.Debugf("proxying freighter request %+v to %s", r, slug)

	host, err := hostGet(s.db, slug)
	if err != nil {
		return err
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
