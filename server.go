package tender

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	_ "github.com/lib/pq"
	"github.com/nitrous-io/tender/pkg/httperror"
	"github.com/nitrous-io/tender/pkg/proxy"
)

// env of the request. This should be created once for each request and valid
// during the request duration
type env struct {
	userID int
	host   string
}

func (e *env) authenticated() bool {
	return e.userID > 0
}

type Server struct {
	// direct access to database so we can fetch stuffs as fast as possible
	// FIXME:
	// - considering replace this with distributed datastore such as etcd
	db *sql.DB
	// port that requires authentication. This should be the port where we
	// exposed container management apis
	authedPort int
	// expected DNS based domain that we want to receive requests to, i.e.
	// `.nitrousapp.com`
	baseDomain string
}

// first check if the request endpoints requires authentication, then proxy it
// to the respected host
// This enable *Server to act as a http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	env := &env{}
	if err := s.authenticated(env, w, r); err != nil {
		s.handleError(w, err)
		return
	}
	if err := s.proxy(env, w, r); err != nil {
		s.handleError(w, err)
		return
	}
}

// Returns a `http.Handler` which can be used to proxy requests to a Nitrous.IO
// host
func NewServer(baseDomain, databaseURL string, authedPort int) (http.Handler, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to open a db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Failed to get a valid db connection %v", err)
	}
	return &Server{
		db:         db,
		authedPort: authedPort,
		baseDomain: baseDomain,
	}, nil
}

// Instead of using the standard Authorization header for OAuth token, we use a
// non-standard header `X-Nitrous-Authorization` so it doesnt clash with normal
// `Authorization` header when the hosted app requires it.
//
// This is used only when we want to proxy container management's api
func (s *Server) authenticated(env *env, w http.ResponseWriter, r *http.Request) error {
	port := parsePort(r)
	// if we dont request a port that requires authentication, passthrough.
	// This most likely is for preview.
	if port != s.authedPort {
		return nil
	}
	authHeader := r.Header.Get("X-Nitrous-Authorization")
	prefix := "Bearer "
	if len(authHeader) <= len(prefix) ||
		(!strings.HasPrefix(authHeader, prefix) &&
			!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
		return fmt.Errorf("Invalid authorization token %s", authHeader)
	}

	userID, err := user(s.db, authHeader[len(prefix):])
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
		return fmt.Errorf("Invalid authorization token %s", authHeader)
	}
	env.userID = userID
	return nil
}

// proxy the request to respected boxhost. We will switch the proxy strategy
// depends on the type of the request
// - if this is a normal HTTP, use `http.ReverseProxy`
// - if the request as `X-NITROUS-STREAM` header which indicates that it wants
// to open a bidirectional HTTP stream, use `proxyStream`
// - if the request wants to open a websocket, use `proxyWebsocket`
func (s *Server) proxy(env *env, w http.ResponseWriter, r *http.Request) (err error) {
	port := parsePort(r)
	log.Printf("Request received: %v, for port %d", r, port)

	hostName, err := getHost(r.Host, s.baseDomain)
	if err != nil {
		return err
	}
	log.Printf("Got request to %s", hostName)

	// get host id and host ip
	// host ip will be the new destination for current request
	var hostIP net.IP
	if env.authenticated() {
		hostIP, err = host(s.db, hostName)
	} else {
		hostIP, err = hostByUser(s.db, env.userID, hostName)
	}
	if err != nil {
		return err
	}
	env.host = hostToIP(r, hostIP)

	if r.Header.Get("X-NITROUS-STREAM") == "true" {
		proxy.ProxyStream(env.host)(w, r)
	} else if isWebSocket(r) {
		proxy.ProxyWebsocket(env.host)(w, r)
	} else {
		proxy.ProxyHTTP(env.host)(w, r)
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
	log.Printf("[ERROR] %v %v", errHash, e.Error())
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
