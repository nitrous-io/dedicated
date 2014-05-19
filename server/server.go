package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/util"
)

type Server struct {
	Logger *util.Logger
}

func (s *Server) Start(host string, port uint16) {
	http.Handle("/", NewRouter(&provider.DockerProvider{}, &HTTPCallback{}))
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	err := server.ListenAndServe()
	if s.Logger != nil {
		s.Logger.Fatal(err)
	}
	os.Exit(1)
}

func NewRouter(provider provider.Provider, callback Callback) *mux.Router {
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

	return r
}
