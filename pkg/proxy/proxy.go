package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"runtime"
	"strings"
	"sync"
	"time"
)

var HTTPFlushInterval = 100 * time.Millisecond

var TransportCleanupInterval = 30 * time.Second

type hostTransport struct {
	Transport *customTransport
	Timer     *time.Timer
}

var (
	// maps "%s-%s" (host domain, ip address) to custom transport
	hostTransportMap     = map[string]*hostTransport{}
	hostTransportMapLock sync.Mutex
)

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
	*http.Transport
	errorFunc func(res *http.Response, err error)
}

func (t *customTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := t.RoundTrip(r)
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
	hostTransportMapLock.Lock()
	k := fmt.Sprintf("%s-%s", addr, tlsConfig.ServerName)
	t, ok := hostTransportMap[k]
	if ok {
		// reschedule cleanup
		t.Timer.Reset(TransportCleanupInterval)
	} else {
		tr := &customTransport{
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
			errorFunc: transportErrorFn,
		}
		t = &hostTransport{
			Transport: tr,
			Timer: time.AfterFunc(TransportCleanupInterval, func() {
				tr.CloseIdleConnections()
			}),
		}
		hostTransportMap[k] = t
	}
	hostTransportMapLock.Unlock()

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
		Transport:     t.Transport,
		FlushInterval: HTTPFlushInterval,
	}
	return p.ServeHTTP
}

var (
	dot       = []byte(".")
	centerDot = []byte("·")
)

func functionFromPC(pc uintptr) string {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return ""
	}
	name := []byte(fn.Name())
	// The name includes the path name to the package, which is unnecessary
	// since the file name is already included.  Plus, it has center dots.
	// That is, we see
	//      runtime/debug.*T·ptrmethod
	// and want
	//      *T.ptrmethod
	if period := bytes.Index(name, dot); period >= 0 {
		name = name[period+1:]
	}
	name = bytes.Replace(name, centerDot, dot, -1)
	return string(name)
}

// log error message with file name + line number of a function callerNo
// levels up the stack
func logErrorHelper(callerNo int, format string, arg ...interface{}) {
	s := "[ERROR] " + fmt.Sprintf(format, arg...)
	if pc, _, _, ok := runtime.Caller(callerNo); ok {
		s = functionFromPC(pc) + ": " + s
	}
	log.Print(s)
}

// logs error message with file name + line number of the function one level
// up the stack
func logError(format string, arg ...interface{}) {
	logErrorHelper(1, format, arg...)
}

// logs error message with file name + line number of the function two levels
// up the stack
func logError2(format string, arg ...interface{}) {
	logErrorHelper(2, format, arg...)
}

// the assumption here is that io.EOF means a clean connection shutdown
func isIoError(err error) bool {
	if err == nil || err == io.EOF {
		return false
	}
	return true
}

var (
	verboseLog = true
)

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
			msg := fmt.Sprintf("websocket: could not connect to backend %s", addr)
			handleError(w, r, msg, err)
			return
		}
		defer serverConn.Close()

		if err = r.Write(serverConn); err != nil {
			msg := fmt.Sprintf("websocket: writing websocket request to backend '%s' failed", addr)
			handleError(w, r, msg, err)
			return
		}

		clientConn, err := hijackServer(w)
		if err != nil {
			handleError(w, r, "websocket: hijack of client connection failed", err)
			return
		}
		clientAddr := clientConn.RemoteAddr()
		defer clientConn.Close()

		if verboseLog {
			log.Printf("[INFO] temp: new websocket proxy connection, from client: %s to backend %s\n", clientAddr, addr)
		}

		sErr, cErr := bidirectionalCopy(serverConn, clientConn)
		if verboseLog && !isIoError(sErr) && !isIoError(cErr) {
			log.Printf("[INFO] cleanly shut down connection, client: %s, backend: %s\n", clientAddr, addr)
		}
		if isIoError(sErr) {
			logError("websocket: io.Copy() clientConn (%s) -> serverConn (%s) failed, err: '%v' (type: '%T')", clientAddr, addr, sErr, sErr)
		}
		if isIoError(cErr) {
			logError("websocket: io.Copy() serverConn (%s) -> clientConn (%s) failed, err: '%v' (type: '%T')", addr, clientAddr, cErr, cErr)
		}
	}
}

type errIdx struct {
	err error
	idx int
}

func bidirectionalCopy(rw1, rw2 io.ReadWriter) (error, error) {
	var errors [2]error
	chErr := make(chan errIdx, 2)
	cp := func(dst io.Writer, src io.Reader, idx int) {
		_, err := io.Copy(dst, src)
		chErr <- errIdx{err, idx}
	}
	go cp(rw1, rw2, 0)
	go cp(rw2, rw1, 1)

	// wait for first connection to error out
	errIdx := <-chErr
	errors[errIdx.idx] = errIdx.err

	// wait for second connection to finish for up to a second. Since they are
	// 2 ends of the same connection, they should exit at the same time so
	// 1 second should be more than enough
	select {
	case errIdx = <-chErr:
		errors[errIdx.idx] = errIdx.err
	case <-time.After(1 * time.Second):
	}
	return errors[0], errors[1]
}

func handleError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	logError2("Request %v failed with err %v, msg: %s", r, err, msg)
	w.WriteHeader(http.StatusInternalServerError)
}

func hijackServer(w http.ResponseWriter) (net.Conn, error) {
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
