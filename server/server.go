package server

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
)

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

func (s *Server) route(route string, handler http.HandlerFunc, methods ...string) {
	route = strings.Join(nsStack, "") + route
	h := func(w http.ResponseWriter, r *http.Request) {
		s.logger.Infof("Request: %v %v", r.Method, r.URL.String())
		handler(w, r)
	}
	s.router.HandleFunc(route, h).Methods(methods...)
}

func (s *Server) static(prefix string, dir string) {
	s.router.PathPrefix(prefix).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		s.logger.Infof("Static asset: %v", r.URL.Path)
		http.StripPrefix(prefix, http.FileServer(http.Dir(path.Join(s.root, dir)))).ServeHTTP(w, r)
	})
}

func (s *Server) action(fn func(controllers.Helper, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(s, w, r)
	}
}

func (s *Server) authedAction(fn func(controllers.Helper, *user.User, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			s.Respond(w, http.StatusBadRequest, []byte(`{"error":"invalid_request"}`))
			return
		}

		tok := authHeader[len(prefix):]
		t, err := token.FindByToken(s.db, tok)
		if err != nil {
			s.logger.Error(err)
			s.RespondWith500(w, "db_read_failed")
			return
		}
		if t == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			s.Respond(w, http.StatusUnauthorized, []byte(`{"error":"invalid_token"}`))
			return
		}

		u, err := user.FindByID(s.db, t.UserID)
		if err != nil {
			s.logger.Error(err)
			s.RespondWith500(w, "db_read_failed")
			return
		}
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			s.Respond(w, http.StatusUnauthorized, []byte(`{"error":"invalid_token"}`))
			return
		}

		fn(s, u, w, r)
	}
}
