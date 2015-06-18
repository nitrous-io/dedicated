package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter"
	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/pkg/tcputils"
	"github.com/nitrous-io/dedicated/pkg/uuid"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

const NitrousUUIDHeader = "X-NITROUS-UUID"

var (
	// default server's configuration when we dont have a valid configuration
	DefaultConfig *freighter.Config

	MaxRequestSize int64 = 20 << 20 // 20 MB
)

func init() {
	DefaultConfig = &freighter.Config{
		DataPath:  "/var/lib/freighter",
		TLSVerify: true,
		CA:        "/etc/freighterd/ca.crt",
		Cert:      "/etc/freighterd/cert.crt",
		Key:       "/etc/freighterd/key.crt",
		Secret:    "/etc/freighterd/secret",
		NotifyURL: "",

		Listen: []freighter.HostAddress{
			{Proto: "unix", Address: "/var/run/freighterd.sock"},
			{Proto: "tcp", Address: "0.0.0.0:65333"},
		},
	}
}

type Server struct {
	provider  provider.Provider
	router    *mux.Router
	conf      *freighter.Config
	secret    string
	listeners []net.Listener
	runner    Runner
}

func NewServer(provider provider.Provider, conf *freighter.Config) *Server {
	if provider == nil || conf == nil {
		panic("server.NewServer: invalid argument")
	}
	svr := &Server{
		provider: provider,
		router:   mux.NewRouter(),
		conf:     conf,
		runner:   &realRunner{},
	}
	svr.drawRoutes()
	return svr
}

func (s *Server) Start() error {
	for _, h := range s.conf.Listen {
		var ln net.Listener
		var err error

		proto, addr := h.Proto, h.Address
		switch proto {
		case "unix":
			ln, err = s.unixListener(addr)
			if err != nil {
				return err
			}
		case "tcp":
			ln, err = s.tcpListener(addr)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown protocol: %s", h.Proto)
		}

		svr := &http.Server{Addr: addr, Handler: s.router}
		go func() {
			logrus.Infof("Listening on %s %s", proto, addr)
			s.listeners = append(s.listeners, ln)
			err := svr.Serve(ln)
			if err != nil {
				logrus.Errorf("Listener on %s %s failed with %v", proto, addr, err)
			}
		}()
	}

	go func() {
		data := map[string]interface{}{
			"event":              "started",
			"freighterd_version": freighter.Version,
		}
		if v, err := s.provider.Version(); err != nil {
			data["docker_version"] = v
		}

		err := s.notifyWebhook(data)
		if err != nil {
			logrus.Errorf("failed to notify webhook, err: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() {
	for _, ln := range s.listeners {
		ln.Close()
	}
}

func (s *Server) unixListener(addr string) (net.Listener, error) {
	if err := syscall.Unlink(addr); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("server: cannot remove %s, %v", addr, err)
	}
	return net.Listen("unix", addr)
}

func (s *Server) tcpListener(addr string) (net.Listener, error) {
	var l net.Listener
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("server: cannot listen to tcp %v, %v", addr, err)
	}
	// set keepalive to the tcp listener
	l = tcputils.TcpKeepAliveListener{l.(*net.TCPListener)}
	if s.conf.TLSVerify {
		tlsConfig, err := getTLSConfig(s.conf.CA, s.conf.Cert, s.conf.Key)
		if err != nil {
			return nil, fmt.Errorf("server: invalid tls configs %v", err)
		}
		l = tls.NewListener(l, tlsConfig)
	}
	if s.conf.Secret != "" {
		secret, err := ioutil.ReadFile(s.conf.Secret)
		if err != nil {
			return nil, fmt.Errorf("Error reading secret at %s: %s", s.conf.Secret, err)
		}
		s.secret = strings.TrimSpace(string(secret))
	}
	return l, nil
}

func (s *Server) notifyWebhook(data map[string]interface{}) error {
	if s.conf.NotifyURL == "" {
		return nil
	}

	verifier := msgverifier.New(s.secret)
	msg, err := verifier.Generate(data)
	if err != nil {
		return err
	}

	params := &url.Values{}
	params.Set("msg", msg)

	req, err := http.NewRequest("POST", s.conf.NotifyURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if b, err := ioutil.ReadAll(resp.Body); err == nil {
			return fmt.Errorf("non-HTTP OK response from webhook, resp: %s", b)
		}
		return errors.New("non-HTTP OK response form webhook")
	}

	return nil
}

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) makeLogUUIDHandler(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuidStr := r.Header.Get(NitrousUUIDHeader)
		if uuidStr == "" {
			uuidStr = uuid.Make()
			r.Header.Set(NitrousUUIDHeader, uuidStr)
		}
		// so that is available in s.respond()
		w.Header().Set(NitrousUUIDHeader, uuidStr)
		logrus.Infof("uuid=%s Request: %v %v\n", uuidStr, r.Method, r.URL.String())
		handlerFunc(w, r)
		logrus.Infof("uuid=%s Request: done\n", uuidStr)
	}
}

func (s *Server) secretRoute(route string, handler http.HandlerFunc, methods ...string) {
	h := func(w http.ResponseWriter, r *http.Request) {
		if secret := r.Header.Get("X-Freighter-Secret"); secret != s.secret {
			errMsg := "invalid_secret"
			if secret == "" {
				errMsg = "missing_secret"
			}
			s.respondEncodeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": errMsg})
			return
		}

		// authentication succeed, process the middleware as normal now
		handler(w, r)
	}
	h2 := s.makeLogUUIDHandler(h)
	s.router.HandleFunc(route, h2).Methods(methods...)
}

// unauthedRoute is for routes that have no secret-based authentication (either
// no authentication at all, or handled within the route itself)
func (s *Server) route(route string, handler http.HandlerFunc, methods ...string) {
	s.router.HandleFunc(route, s.makeLogUUIDHandler(handler)).Methods(methods...)
}

// given:
// - a certificate authority
// - a certificate signed by the CA
// - a private key for the certificate
// return a proper tls config that can be used with tls.NewListener to secure a
// net.Listener
func getTLSConfig(caPath, certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("Couldn't load X509 key pair (%s, %s): %s. Key encrypted?",
			certPath, keyPath, err)
	}
	file, err := ioutil.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("Couldn't read CA certificate %s: %s", caPath, err)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(file); !ok {
		return nil, fmt.Errorf("Couldn't append CA certificate %s", caPath)
	}

	// by default, we dont require any client cert i.e. no authentication
	// as long as the client supplies the correct CA, it's allowed to talk to
	// the server's endpoints
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS10,
	}

	return tlsConfig, nil
}

func limitRequestSize(h http.HandlerFunc, limit int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if limit > 0 && r.ContentLength > limit {
			http.Error(w, "request body too large", 413)
			return
		}
		// limit the size of request body. if request is too large,
		// reading r.Body will fail ( such as ParseMultipartForm )
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		h(w, r)
	}
}
