package server

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var jsonNotFound = []byte(`{"error":"not_found","error_description":"not found"}`)

// route namespace stack
var nsStack []string = []string{}

type Server struct {
	router *mux.Router
	root   string
	logger *logger.Logger
	db     *db.Database
}

func NewServer(root string, db *db.Database, l *logger.Logger) *Server {
	if l == nil {
		panic("server.NewServer: invalid argument")
	}

	svr := &Server{root: root, db: db, logger: l, router: mux.NewRouter()}
	svr.drawRoutes()
	return svr
}

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) Start(host string, port uint16) {
	http.Handle("/", s.router)
	svr := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	err := svr.ListenAndServe()
	s.logger.FatalExit(err)
}

func (s *Server) namespace(ns string, f func()) {
	nsStack = append(nsStack, ns)
	f()
	nsStack = nsStack[:len(nsStack)-1]
}

func (s *Server) route(route string, mw middleware.Middleware, methods ...string) {
	route = strings.Join(nsStack, "") + route
	h := func(w http.ResponseWriter, r *http.Request) {
		s.logger.Infof("Request: %v %v", r.Method, r.URL.String())
		err := mw(&middleware.Env{
			Root:   s.root,
			Logger: s.logger,
			DB:     s.db,
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
		s.logger.Infof("Request: %v %v", r.Method, r.URL.String())
		env := &middleware.Env{
			Root:   s.root,
			Logger: s.logger,
			DB:     s.db,
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
		if strings.HasSuffix(r.URL.Path, "/") {
			s.RespondJSON(w, http.StatusNotFound, jsonNotFound)
			return
		}
		_, err := os.Stat(path.Join(s.root, dir, r.URL.Path))
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
	s.logger.Infof("Response: %v %v", status, string(data))
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
