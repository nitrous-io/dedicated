package tender

import (
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	freighterconfig "github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/tender/pkg/httperror"
)

const (
	HostDomain = iota
	ContainerDomain
)

var (
	DefaultStartPort       = freighterconfig.MinPreviewPort
	DefaultEndPort         = freighterconfig.MaxPreviewPort
	MinPort          int64 = 1024

	ErrDomainMismatched = errors.New("mismatched domain")

	portMap = map[string]int{
		"http":  80,
		"https": 443,
	}
)

// This is called when the error is not a *httperror.Error*, i.e. there's
// something wrong with the controller. We transform that error into a
// httperror.Error and log it.
func serverError(err error) error {
	code := http.StatusInternalServerError
	errMsg := err.Error()
	errHash := fmt.Sprintf("%x", sha1.Sum([]byte(errMsg)))
	e := httperror.New(code, "internal_server_error", "", errHash)
	log.Printf("[ERROR] %s %s %s", errHash, errMsg, e.Error())
	return e
}

// handle error returned by the server HTTP Handler
func handleError(w http.ResponseWriter, err error) {
	e, ok := err.(*httperror.Error)
	if !ok {
		e = serverError(err).(*httperror.Error)
	}
	j, err := e.JSON()
	if err != nil {
		j = []byte(`{"error":"internal_server_error"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	http.Error(w, string(j), e.Code)
}

// getPublicFreighterdPort parses the output of freighterd's port list for a
// container, looking for the wanted port.
func getPublicFreighterdPort(ports map[string][]string, wantedPort string) (string, error) {
	exposedPorts, ok := ports[wantedPort]
	if !ok || len(exposedPorts) == 0 {
		return "", fmt.Errorf("port %s is not exposed", wantedPort)
	}
	return exposedPorts[0], nil
}

// parsePort returns the port of an HTTP request. If there is no port specified
// in the request, 80 or 443 (HTTPS) is assumed.
func parsePort(req *http.Request) int {
	if _, portStr, err := net.SplitHostPort(req.Host); err == nil {
		if p, err := strconv.Atoi(portStr); err == nil {
			return p
		}
	}
	if req.URL.Scheme != "" {
		return portMap[req.URL.Scheme]
	}
	if req.TLS != nil {
		return 443
	} else {
		return 80
	}
}

// ConvertToIPBased modifies an HTTP request to use an IP address as its host.
// The "Host" header is preserved so that receiver will correctly recognize the
// original host instead of the ip address
func hostToIP(req *http.Request, ip net.IP, port int) string {
	h := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	req.URL.Host = h
	return h
}

// Given a request with host:
// winterfell.yammer.nitrousapp.com:8000, it will return host name as "yammer"
// yammer.nitrousapp.com:65333, it will return host name as "yammer"
// foo.bar.yammer.nitrousapp.com:65333, it will return host name as "yammer"
func getHostname(url, baseDomain string) (string, error) {
	var err error
	domain := url
	if strings.Contains(url, ":") {
		domain, _, err = net.SplitHostPort(url)
		if err != nil {
			return "", err
		}
	}

	dLen := len(domain)
	bdLen := len(baseDomain)

	if dLen <= bdLen {
		return "", ErrDomainMismatched
	}

	if domain[dLen-bdLen:] != baseDomain {
		return "", ErrDomainMismatched
	}

	prefix := domain[:dLen-bdLen]
	splits := strings.Split(prefix, ".")

	return splits[len(splits)-1], nil
}

// portsRange converts start/end port string into number with sensible default
func PortsRange(s, e string) (start int64, end int64, err error) {
	if s == "" {
		start = DefaultStartPort
	} else {
		sp, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid start port %s. Error %s", s, err)
		}
		start = sp
	}
	if start < MinPort {
		start = MinPort
	}

	if e == "" {
		end = DefaultEndPort
	} else {
		ep, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid end port %s. Error %s", e, err)
		}
		end = ep
	}
	if end < MinPort {
		end = MinPort
	}

	return start, end, nil
}

// Spin up a HTTPS Server listening to an address
func ServeTLS(port int64, tlsConfig *tls.Config, s http.Handler) error {
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:      addr,
		Handler:   s,
		TLSConfig: tlsConfig,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen to %q, error: %q", addr, err)
	}

	ln = tls.NewListener(TcpKeepAliveListener{ln.(*net.TCPListener)}, tlsConfig)
	return srv.Serve(ln)
}

// TcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
//
// This is copied from net/http package
type TcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln TcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
