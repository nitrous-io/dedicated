package tender

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	proxyerror "github.com/nitrous-io/dedicated/pkg/errorpages/proxy"
)

const (
	HostDomain = iota
	ContainerDomain
)

var (
	DefaultStartPort int64 = 1024
	DefaultEndPort   int64 = 9999
	MinPort          int64 = 1024

	VerboseLog int32 // a bool as int32 so that we can use atomic ops

	ErrDomainMismatched = errors.New("mismatched domain")

	portMap = map[string]int{
		"http":  80,
		"https": 443,
	}
)

// handle error returned by the server HTTP Handler
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	if err == ErrDomainMismatched {
		// Treat domain mismatch as a 404 instead of a server error.
		http.Error(w, "", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Header().Add("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		io.Copy(w, bufio.NewReader(bytes.NewBuffer(proxyerror.GzippedErrorPage)))
	} else {
		io.Copy(w, bufio.NewReader(bytes.NewBuffer(proxyerror.ErrorPage)))
	}
	w.WriteHeader(http.StatusServiceUnavailable)
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
//
// If there's mismatched in domain, the error will be ErrDomainMismatched
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

func ToggleVerboseLog() {
	v := atomic.LoadInt32(&VerboseLog)
	if v == 0 {
		v = 1
	} else {
		v = 0
	}
	atomic.StoreInt32(&VerboseLog, v)
	log.Printf("Toggled verbose logging to %d", v)
}

func LogVerbosef(format string, args ...interface{}) {
	v := atomic.LoadInt32(&VerboseLog)
	if v == 0 {
		return
	}
	log.Printf(format, args...)
}
