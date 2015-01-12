package tender

import (
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"

	_ "github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/pkg/proxy"
	"github.com/nitrous-io/dedicated/tender/pkg/httperror"
)

// env of the request. This should be created once for each request and valid
// during the request duration
type env struct {
	host string
}

type Server struct {
	// direct access to database so we can fetch stuffs as fast as possible
	// FIXME:
	// - considering replace this with distributed datastore such as etcd
	db *sql.DB
	// remote freighterd port
	freighterdPort int64
	// expected DNS based domain for host's request. Request coming from this
	// domain will be proxied to freighterd server on that host.
	hostBaseDomain string
	// expected DNS based doamin for container's preview request. Request
	// coming from this domain will be proxied to the container's host
	// directly.
	containerBaseDomain string
	// expected domain that is used when we create the TLS certificates in webapp
	// `nitrouspro.com`. This is used when communicating with the freighterd
	// server on the host.
	tlsDomain string
	// default tls config to talk to remote freighterd server. whenever we
	// proxy to a server, we have to copy this and set a server name
	tlsConfig *tls.Config
}

// first check if the request endpoints requires authentication, then proxy it
// to the respected host
// This enable *Server to act as a http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	env := &env{}
	if err := s.proxy(env, w, r); err != nil {
		s.handleError(w, err)
		return
	}
}

// Returns a `http.Handler` which can be used to proxy requests to a Nitrous.IO
// host
func NewServer(hostBaseDomain, containerBaseDomain, tlsDomain,
	databaseURL string, freighterdPort int64, caBytes []byte) (http.Handler, error) {
	log.Printf("Connecting to database %s", databaseURL)
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to open a db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Failed to get a valid db connection %v", err)
	}
	s := &Server{
		db:                  db,
		freighterdPort:      freighterdPort,
		hostBaseDomain:      hostBaseDomain,
		containerBaseDomain: containerBaseDomain,
		tlsDomain:           tlsDomain,
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

func (s *Server) proxy(env *env, w http.ResponseWriter, r *http.Request) (err error) {
	hostName, err := getHost(r.Host, s.hostBaseDomain)
	if err != nil {
		return err
	}

	return s.proxyHostRequest(hostName, w, r)
}

// proxy the request to respected boxhost. We will switch the proxy strategy
// depends on the type of the request
// - if this is a normal HTTP, use `http.ReverseProxy`
// - if the request wants to open a websocket, use `proxyWebsocket`
func (s *Server) proxyHostRequest(hostName string, w http.ResponseWriter, r *http.Request) (err error) {
	log.Printf("[INFO] request %v to %s", r, hostName)

	// set X-FORWARDED-PORT for the request
	fp := strconv.Itoa(parsePort(r))
	r.Header.Set(config.ForwardPortHeader, fp)
	log.Printf("[INFO] set forward port to %s", fp)

	// get host id and host ip
	// host ip will be the new destination for current request
	var ip net.IP
	var port int
	ip, port, err = hostIP(s.db, hostName)
	if err != nil {
		return err
	}
	host := hostToIP(r, ip, port)

	// copy tls config and set serverName
	// serverName is the name we use to generate private certs on the host
	// this must be the same as Hostname in
	// https://github.com/nitrous-io/nitrous/blob/master/models/host/host.go
	var tlsConfig *tls.Config

	if s.tlsConfig != nil {
		tlsConfig = &tls.Config{}
		*tlsConfig = *s.tlsConfig
		tlsConfig.ServerName = fmt.Sprintf("%s%s", hostName, s.tlsDomain)
	}

	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(host, tlsConfig)(w, r)
	} else {
		proxy.ProxyHTTP(host, tlsConfig)(w, r)
	}
	return nil
}

// This is called when the error is not a *httperror.Error*, i.e. there's
// something wrong with the controller. We transform that error into a
// httperror.Error and log it.
func (s *Server) serverError(err error) error {
	code := http.StatusInternalServerError
	errMsg := err.Error()
	errHash := fmt.Sprintf("%x", sha1.Sum([]byte(errMsg)))
	e := httperror.New(code, "internal_server_error", "", errHash)
	log.Printf("[ERROR] %s %s %s", errHash, errMsg, e.Error())
	return e
}

// handle error returned by the server HTTP Handler
func (s *Server) handleError(w http.ResponseWriter, err error) {
	e, ok := err.(*httperror.Error)
	if !ok {
		e = s.serverError(err).(*httperror.Error)
	}
	j, err := e.JSON()
	if err != nil {
		j = []byte(`{"error":"internal_server_error"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	http.Error(w, string(j), e.Code)
}
