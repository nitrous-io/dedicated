package server

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/util"
)

type Server struct {
	logger   *util.Logger
	provider provider.Provider
	callback Callback
	router   *mux.Router
}

func NewServer(logger *util.Logger, provider provider.Provider, callback Callback) *Server {
	svr := &Server{logger: logger, provider: provider, callback: callback}
	svr.router = newRouter(provider, callback)
	return svr
}

func (s *Server) Start(host string, port uint16) {
	http.Handle("/", s.router)
	svr := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	err := svr.ListenAndServe()
	if s.logger != nil {
		s.logger.FatalExit(err)
	}
}

func (s *Server) Router() *mux.Router {
	return s.router
}

func newRouter(provider provider.Provider, callback Callback) *mux.Router {
	a := RestAPI{Provider: provider, Callback: callback}
	r := mux.NewRouter()

	r.HandleFunc("/", a.Home).Methods("GET")

	r.HandleFunc("/containers", a.ListContainers).Methods("GET")
	r.HandleFunc("/containers", a.CreateContainer).Methods("POST")
	r.HandleFunc("/containers/{id}", a.GetContainerInfo).Methods("GET")
	r.HandleFunc("/containers/{id}", a.TerminateContainer).Methods("DELETE")
	r.HandleFunc("/containers/{id}/start", a.StartContainer).Methods("POST")
	r.HandleFunc("/containers/{id}/stop", a.StopContainer).Methods("POST")
	r.HandleFunc("/containers/{id}/restart", a.RestartContainer).Methods("POST")
	r.HandleFunc("/containers/{id}/public_keys", a.ListPublicKeys).Methods("GET")
	r.HandleFunc("/containers/{id}/public_keys", a.AddPublicKeys).Methods("POST", "PUT")
	r.HandleFunc("/containers/{id}/public_keys/{key_fingerprint}", a.RemovePublicKey).Methods("DELETE")

	return r
}
