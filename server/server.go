package server

import (
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
)

type Server struct{}

func (s *Server) Start(host string, port uint16) {
	http.Handle("/", NewRouter())
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port)}
	log.Fatal(server.ListenAndServe())
}

func NewRouter() *mux.Router {
	a := RestAPI{}
	r := mux.NewRouter()

	r.HandleFunc("/", a.Home).Methods("GET")

	return r
}
