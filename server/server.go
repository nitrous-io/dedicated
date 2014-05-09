package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
)

type Server struct{}

func (s *Server) Start(host string, port uint16) {
	http.Handle("/", NewRouter(&provider.DockerProvider{}))
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	log.Fatal(server.ListenAndServe())
}

func NewRouter(provider provider.Provider) *mux.Router {
	a := RestAPI{Provider: provider}
	r := mux.NewRouter()

	r.HandleFunc("/", a.Home).Methods("GET")

	r.HandleFunc("/containers", a.CreateContainer).Methods("POST")
	r.HandleFunc("/containers/{name}", a.GetContainerInfo).Methods("GET")

	return r
}
