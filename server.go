package tender

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

const (
	// get the user id if we know the access token of that user
	userQuery = `
	SELECT users.id
	FROM users INNER JOIN tokens ON tokens.user_id = users.id
	WHERE tokens.token = $1
	`

	// get the host
	hostQuery = `
	SELECT hosts.ip_address
	FROM hosts
	WHERE hosts.name = $1
	`

	// get the host id and ip address if we know the user and the host name
	hostUserQuery = `
	SELECT hosts.ip_address
	FROM hosts INNER JOIN users ON hosts.user_id = users.id
	WHERE users.id = $1 AND hosts.name = $2
	`
)

type env struct {
	userID int
	host   string
}

func (e *env) authenticated() bool {
	return e.userID > 0
}

func user(db *sql.DB, token string) (int, error) {
	var id int
	err := db.QueryRow(userQuery, token).Scan(&id)
	if os.Getenv("DEBUG") == "true" {
		return 1, nil
	}
	return id, err
}

func host(db *sql.DB, hostName string) (net.IP, error) {
	var ip string
	if os.Getenv("DEBUG") == "true" {
		return net.ParseIP("172.17.0.24"), nil
	}
	err := db.QueryRow(hostQuery, hostName).Scan(&ip)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(ip), nil
}

func hostUser(db *sql.DB, userID int, hostName string) (net.IP, error) {
	var ip string
	if os.Getenv("DEBUG") == "true" {
		return net.ParseIP("172.17.0.24"), nil
	}
	err := db.QueryRow(hostUserQuery, userID, hostName).Scan(&ip)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(ip), nil
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

// use when the client expects a HTTP stream
func (s *Server) proxyStream(env *env, w http.ResponseWriter, r *http.Request) (err error) {
	// create the stream for the HTTP client
	clientIn, clientOut, err := hijackServer(w)
	if err != nil {
		return err
	}
	// close clientIn
	defer func() {
		if tcpc, ok := clientIn.(*net.TCPConn); ok {
			tcpc.CloseWrite()
		} else {
			clientIn.Close()
		}
	}()
	// close clientOut
	defer func() {
		if tcpc, ok := clientOut.(*net.TCPConn); ok {
			tcpc.CloseWrite()
		} else if closer, ok := clientOut.(io.Closer); ok {
			closer.Close()
		}
	}()
	log.Println("we have raw stream for client now")

	log.Println("starting to proxy stream")
	// get the HTTP stream from remote server
	dial, err := net.Dial("tcp", env.host)
	if err != nil {
		return err
	}
	c := httputil.NewClientConn(dial, nil)
	defer c.Close()
	// we ignore error because the connection will be hijacked
	resp, _ := c.Do(r)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed to connect to remote server %s, status %d",
			env.host, resp.StatusCode)
	}
	log.Println("connected to upstream", resp)

	// hijack the http . We have the stream writer and reader now
	serverStream, br := c.Hijack()
	defer serverStream.Close()

	// Now we have
	// - the stream for client
	// - the read/writecloser for server
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(serverStream, clientIn)
		log.Println("read from client", err)
		done <- err
	}()
	go func() {
		_, err := io.Copy(clientOut, br)
		done <- err
	}()
	<-done
	return nil
}

func (s *Server) proxyHTTP(env *env, w http.ResponseWriter, r *http.Request) (err error) {
	// start reverse proxy the request
	p := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = "http"
			log.Printf("Request to %s is mapped to %v", r.URL.Host, env.host)
		},
		Transport: &http.Transport{Proxy: nil},
	}
	p.ServeHTTP(w, r)
	return nil
}

func (s *Server) proxy(env *env, w http.ResponseWriter, r *http.Request) (err error) {
	port := parsePort(r)
	log.Printf("Request received: %v, for port %d", r, port)

	hostName, err := s.getHost(r.Host)
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
		hostIP, err = hostUser(s.db, env.userID, hostName)
	}
	if err != nil {
		return err
	}
	env.host = hostToIP(r, hostIP)

	if r.Header.Get("X-NITROUS-STREAM") == "true" {
		return s.proxyStream(env, w, r)
	} else {
		return s.proxyHTTP(env, w, r)
	}
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
	if port != s.authedPort || os.Getenv("DEBUG") == "true" {
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

// first check if the request endpoints requires authentication, then proxy it
// to the respected host
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	env := &env{}
	if err := s.authenticated(env, w, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	if err := s.proxy(env, w, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

// Given a request with host:
// winterfell.yammer.nitrousapp.com:8000, it will return host name as "yammer"
// yammer.nitrousapp.com:65333, it will return host name as "yammer"
func (s *Server) getHost(url string) (string, error) {
	var domain string
	var err error
	if strings.Contains(url, ":") {
		domain, _, err = net.SplitHostPort(url)
		if err != nil {
			return "", err
		}
	} else {
		domain = url
	}
	log.Printf("Domain: %s, base: %s", domain, s.baseDomain)
	prefix := domain[:len(domain)-len(s.baseDomain)]
	splits := strings.Split(prefix, ".")
	log.Printf("Prefix: %s", prefix, splits, len(splits))
	// request only specified host
	if len(splits) == 1 {
		return splits[0], nil
	}
	// request specified both container and host
	if len(splits) == 2 {
		return splits[1], nil
	}
	return "", fmt.Errorf("Invalid format %s", url)
}

func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.EqualFold(r.Header.Get("Connection"), "upgrade")
}

var portMap = map[string]int{
	"http":  80,
	"https": 443,
}

// parsePort returns the port of an HTTP request. If there is no port specified
// in the request, 80 or 443 (HTTPS) is assumed.
func parsePort(req *http.Request) int {
	log.Printf("Host: %s %s %s", req.Host, req.URL.Host, req.URL.Scheme)
	if _, portStr, err := net.SplitHostPort(req.Host); err == nil {
		if p, err := strconv.Atoi(portStr); err == nil {
			return p
		}
	}
	return portMap[req.URL.Scheme]
}

// ConvertToIPBased modifies an HTTP request to use an IP address as its host.
// The "Host" header is preserved so that receiver will correctly recognize the
// original host instead of the ip address
func hostToIP(req *http.Request, ip net.IP) string {
	// Change Host to "IP:Port".
	port := parsePort(req)
	h := net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
	req.URL.Host = h
	return h
}

func hijackServer(w http.ResponseWriter) (io.ReadCloser, io.Writer, error) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, nil, err
	}
	// Flush the options to make sure the client sets the raw mode
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/vnd.nitrous.raw-stream\r\n\r\n")
	return conn, conn, nil
}
