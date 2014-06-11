package server

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous/pkg/logger"
)

type Server struct {
	logger *logger.Logger
	router *mux.Router
}

func NewServer(l *logger.Logger) *Server {
	if l == nil {
		panic("server.NewServer: invalid argument")
	}

	svr := &Server{logger: l, router: mux.NewRouter()}
	svr.drawRoutes()
	return svr
}

func (s *Server) Start(host string, port uint16) {
	http.Handle("/", s.router)
	svr := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	err := svr.ListenAndServe()
	s.logger.FatalExit(err)
}

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) route(route string, handler http.HandlerFunc, methods ...string) {
	h := func(w http.ResponseWriter, r *http.Request) {
		s.logger.Infof("Request: %v %v", r.Method, r.URL.String())
		handler(w, r)
	}
	s.router.HandleFunc(route, h).Methods(methods...)
}

func (s *Server) drawRoutes() {
	s.route("/", s.home, "GET")
}
