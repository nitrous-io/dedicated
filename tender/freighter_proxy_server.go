package tender

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/pkg/proxy"
)

// FreighterProxyServer let user proxy requests to the freighterd host. This is
// useful when the client want to access the freighterd's API without having to
// instal its SSL certificates for example.
type FreighterProxyServer struct {
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
	// default tls config to talk to remote freighterd server. whenever we
	// proxy to a server, we have to copy this and set a server name
	tlsConfig *tls.Config
}

// Returns a `http.Handler` which can be used to proxy requests to a freighterd
// server on a Nitrous.IO host
func NewFreighterProxyServer(baseDomain, tlsDomain string,
	db *sql.DB, caBytes []byte) (http.Handler, error) {
	s := &FreighterProxyServer{
		db:        db,
		domain:    baseDomain,
		tlsDomain: tlsDomain,
	}
	// set the default tls config for proxied connections
	if len(caBytes) > 0 {
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caBytes)
		s.tlsConfig = &tls.Config{
			RootCAs: certPool,
		}
	}
	return s, nil
}

// ServerHTTP lets FreighterProxyServer acts as a http handler
func (s *FreighterProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.proxy(w, r); err != nil {
		handleError(w, err)
		return
	}
}

// proxy the request to respected boxhost. We will switch the proxy strategy
// depends on the type of the request
// - if this is a normal HTTP, use `http.ReverseProxy`
// - if the request wants to open a websocket, use `proxyWebsocket`
func (s *FreighterProxyServer) proxy(w http.ResponseWriter, r *http.Request) (err error) {
	name, err := getHostname(r.Host, s.domain)
	if err != nil {
		return err
	}
	log.Printf("[INFO] request %v to %s", r, name)

	// set X-FORWARDED-PORT for the request
	fp := strconv.Itoa(parsePort(r))
	r.Header.Set(config.ForwardPortHeader, fp)
	log.Printf("[INFO] set forward port to %s", fp)

	// get host id and host ip
	// host ip will be the new destination for current request
	// TODO: since this will never change, cache this here for the duration of
	// the server
	host, err := findHost(s.db, name)
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
		tlsConfig.ServerName = fmt.Sprintf("%s%s", name, s.tlsDomain)
	}

	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(dest, tlsConfig)(w, r)
	} else {
		proxy.ProxyHTTP(dest, tlsConfig)(w, r)
	}
	return nil
}
