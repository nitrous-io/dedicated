package server

import (
	"net/http"
	"net/http/pprof"
)

func (s *Server) pprof(w http.ResponseWriter, r *http.Request) {
	pprof.Index(w, r)
}
