package server

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/facebookgo/httpdown"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/uuid"
)

var jsonNotFound = []byte(`{"error":"not_found","error_description":"not found"}`)

// route namespace stack
var nsStack []string = []string{}

var (
	defaultStopTimeout = 15 * time.Minute
	defaultKillTimeout = 1 * time.Minute
	// to prevent over-filling log file, we shorten how much of the response we log
	maxLoggedResponseLen = 120
)

const (
	routeJSON           = 1 + iota // route may be encoded as json if Status and Data provided
	routeStrictJSON                // route will be encoded as json; Status and Data *must* be provided
	routeWritesResponse            // route handles its own response writing
)

type Server struct {
	Events      *pubsub.Pubsub
	Provisioner *provisioner.Provisioner
	Tracker     analytics.Tracker

	// this channel will be notified when the server is shutting down
	QueueStopCh chan struct{}

	// StopTimeout is the duration before we begin force closing connections.
	StopTimeout time.Duration
	// KillTimeout is the duration before which we completely give up and abort
	// even though we still have connected clients. This is useful when a large
	// number of client connections exist and closing them can take a long time.
	// Note, this is in addition to the StopTimeout.
	KillTimeout time.Duration

	SubscriptionProvider subscriptions.Provider

	router *mux.Router
	root   string
	db     *db.Database
}

func NewServer(root string, db *db.Database, conf *provisioner.Config) *Server {
	events := pubsub.New(128)
	svr := &Server{
		Events:               events,
		Provisioner:          provisioner.NewProvisioner(db, events, conf),
		Tracker:              analytics.NewMixpanelTracker(envvars.MixpanelToken),
		QueueStopCh:          make(chan struct{}),
		StopTimeout:          defaultStopTimeout,
		KillTimeout:          defaultKillTimeout,
		SubscriptionProvider: subscriptions.NewProvider(),
		root:                 root,
		db:                   db,
		router:               mux.NewRouter(),
	}
	svr.drawRoutes()
	return svr
}

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) Start(host string, port uint16) error {
	if err := s.Provisioner.Start(); err != nil {
		switch err {
		case provisioner.ErrPreprovisionerNotConfigured:
			logrus.Warn("nitrous/server: Pre-provisioner not configured")
		default:
			return err
		}
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	hd := &httpdown.HTTP{
		StopTimeout: s.StopTimeout,
		KillTimeout: s.KillTimeout,
	}
	logrus.Debugf("nitrous/server: starting HTTP server with stop timeout: %v, kill timeout: %v",
		s.StopTimeout, s.KillTimeout)

	return ListenAndServe(srv, hd, func() error {
		// wait until the queue consumer finish processing its current message and is stopped
		logrus.Debug("shutting down queue consumer")
		s.QueueStopCh <- struct{}{}
		<-s.QueueStopCh
		return nil
	})
}

func (s *Server) Stop() {
	s.Provisioner.Stop()
}

func (s *Server) namespace(ns string, f func()) {
	nsStack = append(nsStack, ns)
	f()
	nsStack = nsStack[:len(nsStack)-1]
}

func (s *Server) pprofHandler(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/v0")
	pprof.Index(w, r)
	return nil
}

func (s *Server) profileHandler(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pprof.Profile(w, r)
	return nil
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

func (s *Server) stackHandler(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// default trace 100KiB of stack traces
	bufferSize := 102400

	sizeInput := r.FormValue("buffer_size")
	if sizeInput != "" {
		if size, err := strconv.Atoi(sizeInput); err == nil {
			bufferSize = size
		}
	}

	buffer := make([]byte, bufferSize)
	written := runtime.Stack(buffer, true)
	buffer = buffer[:written]

	w.Header().Set("content-type", "text/plain; charset=utf8")
	w.Write(buffer)

	return nil
}

func (s *Server) sleep(w http.ResponseWriter, r *http.Request) {
	var duration int
	if d := r.FormValue("duration"); d != "" {
		duration, _ = strconv.Atoi(d)
	}
	logrus.Debugf("sleeping for %d seconds", duration)
	time.Sleep(time.Duration(duration) * time.Second)
	w.Write([]byte(fmt.Sprintf("slept for %d seconds", duration)))
}

func (s *Server) route(route string, respondMode int, mw middleware.Middleware, methods ...string) {
	route = strings.Join(nsStack, "") + route
	h := func(w http.ResponseWriter, r *http.Request) {
		mh.Meter("requests.nitrous").Mark(1)
		remoteAddr := middleware.GetRemoteAddr(r)
		uuidStr := uuid.Make()
		// allow for consumption by `RespondJSON`
		w.Header().Set(constants.NitrousUUIDHeader, uuidStr)
		// freighter ping responses do not get logged
		if strings.HasSuffix(r.URL.Path, "freighterd_hook") && r.URL.Query().Get("nolog") == "1" {
			w.Header().Set(constants.NitrousUUIDHeader, "nolog")
		} else {
			logrus.Infof("uuid=%s ip=%s Request: %v %v", uuidStr, remoteAddr, r.Method, r.URL.String())
		}
		env := &middleware.Env{
			Root:        s.root,
			DB:          s.db,
			Events:      s.Events,
			Provisioner: s.Provisioner,
			Tracker:     s.Tracker,
			RemoteAddr:  remoteAddr,
			UUID:        uuidStr,
		}
		if err := mw(env, w, r); err != nil {
			s.HandleError(w, r, err)
			return
		}
		routeIsJSON := env.Status != 0 && env.Data != nil
		if respondMode == routeWritesResponse {
			return
		}
		if respondMode == routeStrictJSON && !routeIsJSON {
			s.HandleError(w, r, errors.New("Status and/or Data not set"))
			return
		}
		if routeIsJSON {
			s.RespondEncodeJSON(w, r, env.Status, env.Data)
		}
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
			s.RespondJSON(w, r, http.StatusNotFound, jsonNotFound)
			return
		}
		logrus.Infof("Static asset: %v", r.URL.Path)
		http.StripPrefix(prefix, http.FileServer(http.Dir(path.Join(s.root, dir)))).ServeHTTP(w, r)
	})
}

// process the request error. If the error is of type *httperror.Error, then we
// convert and serialize that to JSON, otherwise, the error will be
// `500 internal_server_error`
func (s *Server) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	e, ok := err.(*httperror.Error)
	if !ok {
		uuid := w.Header().Get(constants.NitrousUUIDHeader)
		e = s.serverError(err, uuid).(*httperror.Error)
	}
	j, err := e.JSON()
	if err != nil {
		j = []byte(`{"error":"internal_server_error"}`)
	}
	s.RespondJSON(w, r, e.Code, j)
}

func (s *Server) RespondJSON(w http.ResponseWriter, r *http.Request, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	w.Write(data)
	uuid := w.Header().Get(constants.NitrousUUIDHeader)

	str := string(data)
	if len(str) > maxLoggedResponseLen {
		// truncate respons for /api/v0/admin/* and /api/v0/templates
		shouldTruncate := strings.Contains(r.URL.Path, "/admin/") || strings.HasSuffix(r.URL.Path, "/templates")
		if shouldTruncate {
			str = fmt.Sprintf("TRUNC (from: %d) %s...", len(str), str[:maxLoggedResponseLen])
		}
	}
	if uuid != "nolog" {
		logrus.Infof("uuid=%s Response: %v %v", uuid, status, str)
	}
}

func (s *Server) RespondEncodeJSON(w http.ResponseWriter, r *http.Request, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		s.HandleError(w, r, err)
		return
	}
	s.RespondJSON(w, r, status, j)
}

// This is called when the error is not a *httperror.Error*, i.e. there's
// something wrong with the controller. We transform that error into a
// httperror.Error and log it.
func (s *Server) serverError(err error, uuid string) error {
	errMsg := err.Error()
	errHash := fmt.Sprintf("%x", sha1.Sum([]byte(errMsg)))
	e := httperror.New(http.StatusInternalServerError, "internal_server_error", "", errHash)
	logrus.WithFields(logrus.Fields{"uuid": uuid}).Error(errHash, errMsg)
	return e
}

func (s *Server) getNumFds() int {
	pid := os.Getpid()
	fds, err := ioutil.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		logrus.Errorf("Error opening /proc/%d/fd: %s", pid, err)
		return -1
	}
	return len(fds)
}

// ListenAndServe is a convenience function to serve and wait for a SIGTERM
// or SIGINT before shutting down.
// It also accepts a callback hook to perform additional cleanup when the server is shutting down
func ListenAndServe(s *http.Server, hd *httpdown.HTTP, stopCallback func() error) error {
	if hd == nil {
		hd = &httpdown.HTTP{}
	}
	hs, err := hd.ListenAndServe(s)
	if err != nil {
		return err
	}
	logrus.Infof("serving on http://%s/ with pid %d", s.Addr, os.Getpid())

	waiterr := make(chan error, 1)
	go func() {
		defer close(waiterr)
		waiterr <- hs.Wait()
	}()

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-waiterr:
		if err != nil {
			return err
		}
	case s := <-signals:
		signal.Stop(signals)
		logrus.Debugf("signal received: %s", s)
		if err := hs.Stop(); err != nil {
			return err
		}
		if err := stopCallback(); err != nil {
			return err
		}
		if err := <-waiterr; err != nil {
			return err
		}
	}
	return nil
}
