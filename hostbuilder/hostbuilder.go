package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/nitrous-io/hostbuilder"
)

type providerConfig map[string]string

type config struct {
	Providers []map[string]providerConfig `json:"providers"`
}

func main() {
	flConfig := flag.String("config", "", "path to the config file")
	flag.Parse()
	if len(*flConfig) == 0 {
		log.Printf("hostbuilder: missing config file")
		os.Exit(1)
	}
	configData, err := ioutil.ReadFile(*flConfig)
	if err != nil {
		log.Printf("hostbuilder: failed to read config file")
		os.Exit(1)
	}
	// start the server
	server, err := hostbuilder.New(configData)
	if err != nil {
		log.Printf("hostbuilder: failed to create server %v", err)
		os.Exit(1)
	}
	http.Handle("/", server.Router)
	log.Printf("hostbuilder: starting server at %v", server.Config.Address)
	http.ListenAndServe(server.Config.Address, nil)
}
