package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
)

var priCACert string

func init() {
	priCACert = envStr("PRI_CRT")
}

func envStr(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing env var " + name)
	}
	return v
}

func tlsConfig() *tls.Config {
	certPool := x509.NewCertPool()
	caBytes, err := ioutil.ReadFile(priCACert)
	if err != nil {
		log.Fatalf("tender: Failed to read private SSL certificate %s", err)
	}

	certPool.AppendCertsFromPEM(caBytes)
	return &tls.Config{
		RootCAs: certPool,
	}
}

func ProxyHTTP(addr string) http.HandlerFunc {
	p := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			log.Printf("[INFO] http: proxy %s to %v", r.URL.Host, addr)
			if r.URL.Scheme == "" {
				r.URL.Scheme = "http"
			}
		},
		Transport: &http.Transport{Proxy: nil},
	}
	return p.ServeHTTP
}

func ProxyWebsocket(addr string) http.HandlerFunc {
	t := tlsConfig()
	return func(w http.ResponseWriter, r *http.Request) {
		serverConn, err := tls.Dial("tcp", addr, t)
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

func ProxyStream(addr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get the HTTP stream from remote server
		dial, err := net.Dial("tcp", addr)
		if err != nil {
			handleError(w, r, "stream: failed to open remote server", err)
			return
		}
		c := httputil.NewClientConn(dial, nil)
		defer c.Close()

		// we ignore error because the connection will be hijacked
		resp, _ := c.Do(r)
		if resp.StatusCode != http.StatusOK {
			handleError(w, r, "stream: failed to open server stream", fmt.Errorf("Status: %d", resp.StatusCode))
			return
		}

		// hijack the http . We have the stream writer and reader now
		serverConn, _ := c.Hijack()
		defer serverConn.Close()

		// create the stream for the HTTP client
		clientConn, err := hijackServer(w)
		if err != nil {
			handleError(w, r, "stream: failed to open client stream", err)
			return
		}
		fmt.Fprintf(clientConn, "HTTP/1.1 200 OK\r\nContent-Type: application/vnd.proxy.raw-stream\r\n\r\n")
		defer clientConn.Close()

		bidirectionalCopy(clientConn, serverConn)
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
