package tender

import (
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
)

const (
	HostDomain = iota
	ContainerDomain
)

var (
	DefaultStartPort = freighterconfig.MinPreviewPort
	DefaultEndPort   = freighterconfig.MaxPreviewPort

	ErrDomainMismatched = errors.New("mismatched domain")
)

var portMap = map[string]int{
	"http":  80,
	"https": 443,
}

// getPublicFreighterdPort parses the output of freighterd's port list for a
// container, looking for the wanted port. If the wanted port does have a
// mapped public port then returns the public port, otherwise returns an error
func getPublicFreighterdPort(ports map[string][]string, wantedPort string) (string, error) {
	exposedPorts, ok := ports[wantedPort]
	if !ok {
		return "", fmt.Errorf("port %s is not exposed", wantedPort)
	}
	for _, portString := range exposedPorts {
		parts := strings.Split(portString, ":")
		if len(parts) != 3 {
			return "", fmt.Errorf("exposed ports %s is invalid", portString)
		}
		if parts[1] == "" {
			return "", fmt.Errorf("exposed ports %s is invalid", portString)
		}
		if parts[0] == "public" {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("port %s is not exposed publicly", wantedPort)
}

// parsePort returns the port of an HTTP request. If there is no port specified
// in the request, 80 or 443 (HTTPS) is assumed.
func parsePort(req *http.Request) int {
	if _, portStr, err := net.SplitHostPort(req.Host); err == nil {
		log.Println("got port", portStr)
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
	return setRequestHost(req, ip, strconv.Itoa(port))
}

func setRequestHost(r *http.Request, ip net.IP, port string) string {
	h := net.JoinHostPort(ip.String(), port)
	r.URL.Host = h
	return h
}

// getHostOrContainer parse the request's domain and return the host subdomain
// with host type when it matches the hostDomain, or the container subdomain
// with container type when it matched the containerDomain
func getHostOrContainer(url, hostDomain, containerDomain string) (name string, hostType int, err error) {
	sub, err := getHost(url, hostDomain)
	if err == ErrDomainMismatched {
		sub, err = getHost(url, containerDomain)
		return sub, ContainerDomain, err
	}
	return sub, HostDomain, err
}

// Given a request with host:
// winterfell.yammer.nitrousapp.com:8000, it will return host name as "yammer"
// yammer.nitrousapp.com:65333, it will return host name as "yammer"
func getHost(url, baseDomain string) (string, error) {
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

	if domain[len(domain)-len(baseDomain):] != baseDomain {
		return "", ErrDomainMismatched
	}

	prefix := domain[:len(domain)-len(baseDomain)]
	splits := strings.Split(prefix, ".")
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

	if e == "" {
		end = DefaultEndPort
	} else {
		ep, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid end port %s. Error %s", e, err)
		}
		end = ep
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
		return fmt.Errorf("tender: Failed to listen to %q, error: %q", addr, err)
	}
	ln = tls.NewListener(tcpKeepAliveListener{ln.(*net.TCPListener)}, tlsConfig)
	err = srv.Serve(ln)
	if err != nil {
		return fmt.Errorf("tender: Failed to serve %q, error: %q", addr, err)
	}
	return nil
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
//
// This is copied from net/http package
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
