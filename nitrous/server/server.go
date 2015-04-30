package server

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/facebookgo/httpdown"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/hostbuilder/pkg/queue"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/uuid"
	"github.com/nitrous-io/nitrous-common/logger"
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

type Server struct {
	Events      *pubsub.Pubsub
	Provisioner *provisioner.Provisioner
	Tracker     analytics.Tracker

	// StopTimeout is the duration before we begin force closing connections.
	StopTimeout time.Duration
	// KillTimeout is the duration before which we completely give up and abort
	// even though we still have connected clients. This is useful when a large
	// number of client connections exist and closing them can take a long time.
	// Note, this is in addition to the StopTimeout.
	KillTimeout time.Duration

	router *mux.Router
	root   string
	logger *logger.Logger
	db     *db.Database
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
		StopTimeout: defaultStopTimeout,
		KillTimeout: defaultKillTimeout,
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

func (s *Server) HostQueueHandler() func(qc *queue.QueueConsumer, m *queue.Message) error {
	return func(qc *queue.QueueConsumer, m *queue.Message) error {
		db := s.db
		if queue.Create.IsA(m.Action) {
			var hostInfo queue.HostQueuePublisher
			err := json.Unmarshal([]byte(m.Payload), &hostInfo)
			if err != nil {
				return err
			}
			uid := hostInfo.UserID
			slug := hostInfo.Slug

			u, err := user.FindByID(db, uid)
			if err != nil {
				return err
			}

			h, err := host.Find(db, u, slug)
			if err != nil {
				return err
			}

			s.Events.Publish(
				controllers.NewEvent("host.creating", map[string]*serializers.Host{
					"host": serializers.SerializeHost(h),
				}),
				u.Topic(),
			)
		}
		return nil
	}
}

func (s *Server) Start(host string, port uint16) error {
	if err := s.Provisioner.Start(); err != nil {
		switch err {
		case provisioner.ErrPreprovisionerNotConfigured:
			s.logger.Warn("nitrous/server: Pre-provisioner not configured")
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
	s.logger.Debugf("nitrous/server: starting HTTP server with stop timeout: %v, kill timeout: %v",
		s.StopTimeout, s.KillTimeout)
	return httpdown.ListenAndServe(srv, hd)
}

func (s *Server) Stop() {
	s.Provisioner.Stop()
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
			s.HandleError(w, r, err)
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
			s.HandleError(w, r, err)
			return
		}
		if env.Status == 0 || env.Data == nil {
			s.HandleError(w, r, errors.New("Status and/or Data not set"))
			return
		}

		s.RespondEncodeJSON(w, r, env.Status, env.Data)
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
		s.logger.Infof("Static asset: %v", r.URL.Path)
		http.StripPrefix(prefix, http.FileServer(http.Dir(path.Join(s.root, dir)))).ServeHTTP(w, r)
	})
}

// process the request error. If the error is of type *httperror.Error, then we
// convert and serialize that to JSON, otherwise, the error will be
// `500 internal_server_error`
func (s *Server) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	e, ok := err.(*httperror.Error)
	if !ok {
		e = s.serverError(err).(*httperror.Error)
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
	s.logger.Infof("uuid=%s Response: %v %v", uuid, status, str)
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
