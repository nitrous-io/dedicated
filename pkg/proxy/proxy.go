package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

var HTTPFlushInterval = 100 * time.Millisecond

func IsWebsocket(r *http.Request) bool {
	return headerContainsValue(r.Header, "Upgrade", "websocket") &&
		headerContainsValue(r.Header, "Connection", "upgrade")
}

type nopCloser struct {
	io.Reader
}

func (c nopCloser) Close() error {
	return nil
}

// DefaultTransportErrorHandler writes the error from the RoundTrip as a
// text/plain response.
func DefaultTransportErrorHandler(res *http.Response, err error) {
	res.StatusCode = http.StatusServiceUnavailable

	h := http.Header{}
	h.Set("Content-Type", "text/plain")
	res.Header = h
	res.Body = nopCloser{strings.NewReader(fmt.Sprintf("Reverse proxy failed: %v", err))}
}

// customTransport delegates to the underlying http.RoundTripper, but if we
// specify the errorFunc, then it will be called when there's an error
// performing the RoundTrip.
type customTransport struct {
	http.RoundTripper
	errorFunc func(res *http.Response, err error)
}

func (t *customTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := t.RoundTripper.RoundTrip(r)
	if err != nil && t.errorFunc != nil {
		if res == nil {
			res = &http.Response{Request: r}
		}
		t.errorFunc(res, err)
		return res, nil
	}
	return res, err
}

// ProxyHTTP return a http handler that reverse proxies request from provided
// address. It also accepts a function that can be used to customize response
// when we cant perform a roundtrip request to the server ( for example, the
// remote server is not running )
func ProxyHTTP(addr string, tlsConfig *tls.Config, transportErrorFn func(res *http.Response, err error)) http.HandlerFunc {
	p := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			log.Printf("[INFO] http: proxy %s to %s", r.Host, addr)
			r.URL.Host = addr
			if tlsConfig != nil {
				r.URL.Scheme = "https"
			} else {
				r.URL.Scheme = "http"
			}
		},
		Transport: &customTransport{
			RoundTripper: &http.Transport{TLSClientConfig: tlsConfig},
			errorFunc:    transportErrorFn,
		},
		FlushInterval: HTTPFlushInterval,
	}
	return p.ServeHTTP
}

func ProxyWebsocket(addr string, tlsConfig *tls.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var serverConn net.Conn
		var err error
		if tlsConfig != nil {
			serverConn, err = tls.Dial("tcp", addr, tlsConfig)
		} else {
			serverConn, err = net.Dial("tcp", addr)
		}
		if err != nil {
			handleError(w, r, "websocket: could not connect to backend", err)
			return
		}
		defer serverConn.Close()

		if err = r.Write(serverConn); err != nil {
			handleError(w, r, "websocket: writing websocket request failed", err)
			return
		}

		clientConn, err := hijackServer(w)
		if err != nil {
			handleError(w, r, "websocket: hijack failed", err)
			return
		}
		defer clientConn.Close()

		bidirectionalCopy(serverConn, clientConn)
	}
}

func bidirectionalCopy(rw1, rw2 io.ReadWriter) {
	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errChan <- err
	}
	go cp(rw1, rw2)
	go cp(rw2, rw1)
	<-errChan
}

func handleError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	log.Printf("[ERROR] Request %v failed with err %v, msg: %s", r, err, msg)
	w.WriteHeader(http.StatusInternalServerError)
}

func hijackServer(w http.ResponseWriter) (io.ReadWriteCloser, error) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func headerContainsValue(header http.Header, name, value string) bool {
	for _, v := range header[name] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(value, strings.TrimSpace(s)) {
				return true
			}
		}
	}
	return false
}
