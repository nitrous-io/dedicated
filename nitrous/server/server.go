package server

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mailgun/manners"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/uuid"
	"github.com/nitrous-io/nitrous-common/logger"
)

var jsonNotFound = []byte(`{"error":"not_found","error_description":"not found"}`)

// route namespace stack
var nsStack []string = []string{}

type Server struct {
	Events      *pubsub.Pubsub
	Provisioner *provisioner.Provisioner
	Tracker     analytics.Tracker

	router   *mux.Router
	root     string
	logger   *logger.Logger
	db       *db.Database
	listener net.Listener
}

func NewServer(root string, db *db.Database, l *logger.Logger, conf *provisioner.Config) *Server {
	if l == nil {
		panic("server.NewServer: invalid argument")
	}
	events := pubsub.New(128)
	svr := &Server{
		Events:      events,
		Provisioner: provisioner.NewProvisioner(db, l, events, conf),
		Tracker:     analytics.NewMixpanelTracker(envvars.MixpanelToken),
		root:        root,
		db:          db,
		logger:      l,
		router:      mux.NewRouter(),
	}
	svr.drawRoutes()
	return svr
}

func (s *Server) Router() *mux.Router {
	return s.router
}

type listener struct {
	net.Listener
	closed chan struct{}
}

var ErrClosed = errors.New("listener has been gracefully closed")

func newListener(ln net.Listener) net.Listener {
	return &listener{Listener: ln, closed: make(chan struct{})}
}

func (ln *listener) Accept() (net.Conn, error) {
	for {
		select {
		case <-ln.closed:
			return nil, ErrClosed
		default:
		}

		// Set a deadline so Accept doesn't block forever, which gives us an
		// opportunity to stop gracefully.
		ln.Listener.(tcpKeepAliveListener).SetDeadline(time.Now().Add(100 * time.Millisecond))

		c, err := ln.Listener.Accept()
		if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
			continue
		}
		return c, err
	}
}

func (ln *listener) Close() error {
	close(ln.closed)
	return nil
}

func (s *Server) Start(ln net.Listener) error {
	if err := s.Provisioner.Start(); err != nil {
		switch err {
		case provisioner.ErrPreprovisionerNotConfigured:
			s.logger.Warn("nitrous/server: Pre-provisioner not configured")
		default:
			return err
		}
	}

	ln = tcpKeepAliveListener{ln.(*net.TCPListener)}
	srv := manners.NewWithServer(&http.Server{
		Handler: s.router,
	})
	s.listener = manners.NewListener(newListener(ln))

	return srv.Serve(s.listener)
}

func (s *Server) Stop() {
	s.Provisioner.Stop()
	s.listener.Close()
}

func (s *Server) namespace(ns string, f func()) {
	nsStack = append(nsStack, ns)
	f()
	nsStack = nsStack[:len(nsStack)-1]
}

// debugHandler prints out debug information in json format.
func (s *Server) debugHandler(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	psInfo := s.Events.Info()
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"num_topics":    psInfo.Topics,
		"num_consumers": psInfo.Consumers,
		"num_fds":       s.getNumFds(),
		"num_goroutine": runtime.NumGoroutine(),
	}
	return nil
}

func (s *Server) sleep(w http.ResponseWriter, r *http.Request) {
	var duration int
	if d := r.FormValue("duration"); d != "" {
		duration, _ = strconv.Atoi(d)
	}
	s.logger.Debugf("sleeping for %d seconds", duration)
	time.Sleep(time.Duration(duration) * time.Second)
	w.Write([]byte(fmt.Sprintf("slept for %d seconds", duration)))
}

func (s *Server) route(route string, mw middleware.Middleware, methods ...string) {
	route = strings.Join(nsStack, "") + route
	h := func(w http.ResponseWriter, r *http.Request) {
		mh.Meter("requests.nitrous").Mark(1)
		s.logger.Infof("Request: %v %v", r.Method, r.URL.String())
		err := mw(&middleware.Env{
			Root:        s.root,
			Logger:      s.logger,
			DB:          s.db,
			Events:      s.Events,
			Provisioner: s.Provisioner,
			Tracker:     s.Tracker,
			RemoteAddr:  middleware.GetRemoteAddr(r),
		}, w, r)
		if err != nil {
			s.HandleError(w, err)
		}
	}
	s.router.HandleFunc(route, h).Methods(methods...)
}

func (s *Server) routeJSON(route string, mw middleware.Middleware, methods ...string) {
	route = strings.Join(nsStack, "") + route
	h := func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := middleware.GetRemoteAddr(r)
		uuidStr := uuid.Make()
		// allow for consumption by `RespondJSON`
		w.Header().Set(constants.NitrousUUIDHeader, uuidStr)
		mh.Meter("requests.nitrous").Mark(1)
		s.logger.Infof("uuid=%s ip=%s Request: %v %v", uuidStr, remoteAddr, r.Method, r.URL.String())
		env := &middleware.Env{
			Root:        s.root,
			Logger:      s.logger,
			DB:          s.db,
			Events:      s.Events,
			Provisioner: s.Provisioner,
			Tracker:     s.Tracker,
			RemoteAddr:  remoteAddr,
			UUID:        uuidStr,
		}
		if err := mw(env, w, r); err != nil {
			s.HandleError(w, err)
			return
		}
		if env.Status == 0 || env.Data == nil {
			s.HandleError(w, errors.New("Status and/or Data not set"))
			return
		}

		s.RespondEncodeJSON(w, env.Status, env.Data)
	}
	s.router.HandleFunc(route, h).Methods(methods...)
}

func (s *Server) static(prefix string, dir string) {
	s.router.PathPrefix(prefix).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rpath := r.URL.Path
		if strings.HasSuffix(rpath, "/") {
			// in dev, nginx will not proxy to Go server unless path prefix is /api/v0
			// in production, assets are compiled and will be in public dir

			// Default nginx setup has `index index.html index.htm` which
			// redirects `/foo/index.html` to `/foo/` .
			// To support webapp's /app/index.html in production, we append back the index.html here
			rpath += "index.html"
		}
		_, err := os.Stat(path.Join(s.root, dir, rpath))
		if os.IsNotExist(err) {
			s.RespondJSON(w, http.StatusNotFound, jsonNotFound)
			return
		}
		s.logger.Infof("Static asset: %v", r.URL.Path)
		http.StripPrefix(prefix, http.FileServer(http.Dir(path.Join(s.root, dir)))).ServeHTTP(w, r)
	})
}

// process the request error. If the error is of type *httperror.Error, then we
// convert and serialize that to JSON, otherwise, the error will be
// `500 internal_server_error`
func (s *Server) HandleError(w http.ResponseWriter, err error) {
	e, ok := err.(*httperror.Error)
	if !ok {
		e = s.serverError(err).(*httperror.Error)
	}
	j, err := e.JSON()
	if err != nil {
		j = []byte(`{"error":"internal_server_error"}`)
	}
	s.RespondJSON(w, e.Code, j)
}

func (s *Server) RespondJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	w.Write(data)
	uuid := w.Header().Get(constants.NitrousUUIDHeader)
	s.logger.Infof("uuid=%s Response: %v %v", uuid, status, string(data))
}

func (s *Server) RespondEncodeJSON(w http.ResponseWriter, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		s.HandleError(w, err)
		return
	}
	s.RespondJSON(w, status, j)
}

// This is called when the error is not a *httperror.Error*, i.e. there's
// something wrong with the controller. We transform that error into a
// httperror.Error and log it.
func (s *Server) serverError(err error) error {
	errMsg := err.Error()
	errHash := fmt.Sprintf("%x", sha1.Sum([]byte(errMsg)))
	e := httperror.New(http.StatusInternalServerError, "internal_server_error", "", errHash)
	s.logger.Error(errHash, errMsg)
	return e
}

func (s *Server) getNumFds() int {
	pid := os.Getpid()
	fds, err := ioutil.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		s.logger.Errorf("Error opening /proc/%d/fd: %s", pid, err)
		return -1
	}
	return len(fds)
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
//
// Copied from the net/http package.
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
