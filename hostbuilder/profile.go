package hostbuilder

import (
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/gorilla/mux"
)

func (s *Server) AddDebugRoutes(r *mux.Router) {
	r.HandleFunc("/debug/pprof/{path:.*}", s.pprof).Methods("GET")
}

func (s *Server) pprof(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/debug")
	pprof.Index(w, r)
}
