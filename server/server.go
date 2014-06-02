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
	svr.router = svr.newRouter(provider, callback)
	return svr
}

func (self *Server) Start(host string, port uint16) {
	http.Handle("/", self.router)
	svr := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	err := svr.ListenAndServe()
	if self.logger != nil {
		self.logger.FatalExit(err)
	}
}

func (self *Server) Router() *mux.Router {
	return self.router
}

func (self *Server) newRouter(provider provider.Provider, callback Callback) *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/", self.home).Methods("GET")

	r.HandleFunc("/containers", self.listContainers).Methods("GET")
	r.HandleFunc("/containers", self.createContainer).Methods("POST")
	r.HandleFunc("/containers/{id}", self.getContainerInfo).Methods("GET")
	r.HandleFunc("/containers/{id}", self.terminateContainer).Methods("DELETE")
	r.HandleFunc("/containers/{id}/start", self.startContainer).Methods("POST")
	r.HandleFunc("/containers/{id}/stop", self.stopContainer).Methods("POST")
	r.HandleFunc("/containers/{id}/restart", self.restartContainer).Methods("POST")
	r.HandleFunc("/containers/{id}/public_keys", self.listPublicKeys).Methods("GET")
	r.HandleFunc("/containers/{id}/public_keys", self.addPublicKeys).Methods("POST", "PUT")
	r.HandleFunc("/containers/{id}/public_keys/{key_fingerprint}", self.removePublicKey).Methods("DELETE")

	return r
}
