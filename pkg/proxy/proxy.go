package proxy

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"time"
)

var (
	HTTPFlushInterval = 100 * time.Millisecond
)

func ProxyHTTP(addr string, tlsConfig *tls.Config) http.HandlerFunc {
	p := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			log.Printf("[INFO] http: proxy %s to %s", r.URL, addr)
			r.URL.Host = addr
			if tlsConfig != nil {
				r.URL.Scheme = "https"
			} else {
				r.URL.Scheme = "http"
			}
		},
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
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
