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
	if logger == nil || provider == nil || callback == nil {
		panic("server.NewServer: invalid argument")
	}
	svr := &Server{logger: logger, provider: provider, callback: callback}
	svr.router = mux.NewRouter()
	svr.drawRoutes()
	return svr
}

func (self *Server) Start(host string, port uint16) {
	http.Handle("/", self.router)
	svr := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	err := svr.ListenAndServe()
	self.logger.FatalExit(err)
}

func (self *Server) Router() *mux.Router {
	return self.router
}

func (self *Server) route(route string, handler http.HandlerFunc, methods ...string) {
	h := func(w http.ResponseWriter, r *http.Request) {
		self.logger.Infof("Request: %v %v", r.Method, r.URL.String())
		handler(w, r)
	}
	self.router.HandleFunc(route, h).Methods(methods...)
}

func (self *Server) drawRoutes() {
	self.route("/", self.home, "GET")
	self.route("/containers", self.listContainers, "GET")
	self.route("/containers", self.createContainer, "POST")
	self.route("/containers/{id}", self.getContainerInfo, "GET")
	self.route("/containers/{id}", self.terminateContainer, "DELETE")
	self.route("/containers/{id}/start", self.startContainer, "POST")
	self.route("/containers/{id}/stop", self.stopContainer, "POST")
	self.route("/containers/{id}/restart", self.restartContainer, "POST")
	self.route("/containers/{id}/public_keys", self.listPublicKeys, "GET")
	self.route("/containers/{id}/public_keys", self.addPublicKeys, "POST", "PUT")
	self.route("/containers/{id}/public_keys/{key_fingerprint}", self.removePublicKey, "DELETE")
}
