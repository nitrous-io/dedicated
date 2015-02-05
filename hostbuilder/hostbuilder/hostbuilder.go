package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/facebookgo/httpdown"
	"github.com/nitrous-io/dedicated/hostbuilder"
)

var (
	version string

	// StopTimeout is the duration before we begin force closing connections.
	// Defaults to 15 minutes.
	defaultStopTimeout = 15 * time.Minute

	// KillTimeout is the duration before which we completely give up and abort
	// even though we still have connected clients. This is useful when a large
	// number of client connections exist and closing them can take a long time.
	// Note, this is in addition to the StopTimeout. Defaults to 1 minute.
	defautlKillTimeout = 1 * time.Minute
)

type providerConfig map[string]string

type config struct {
	Providers []map[string]providerConfig `json:"providers"`
}

func main() {
	flConfig := flag.String("config", "", "path to the config file")
	flVersion := flag.Bool("v", false, "current version")
	flag.Parse()
	if *flVersion {
		fmt.Printf("hostbuilder version %v\n", version)
		os.Exit(0)
		return
	}
	if len(*flConfig) == 0 {
		log.Printf("hostbuilder: missing config file")
		os.Exit(1)
	}
	configData, err := ioutil.ReadFile(*flConfig)
	if err != nil {
		log.Printf("hostbuilder: failed to read config file")
		os.Exit(1)
	}

	// build the server routes
	server, err := hostbuilder.New(configData)
	if err != nil {
		log.Printf("hostbuilder: failed to create server %v", err)
		os.Exit(1)
	}
	http.Handle("/", server.Router)

	// create the http server
	httpServer := &http.Server{Addr: server.Config.Address, Handler: nil}

	// start the httpdown wrapper so we can graceful shutdown
	stopTimeout := time.Duration(server.Config.ShutdownTimeoutSecs) * time.Second
	if stopTimeout == 0 {
		stopTimeout = defaultStopTimeout
	}
	killTimeout := defautlKillTimeout
	hd := &httpdown.HTTP{
		StopTimeout: stopTimeout,
		KillTimeout: killTimeout,
	}

	// start listening to request
	// The server will be gracefully stopped when it receives SIGTERM or SIGINT
	// so another server can be started in its place.
	log.Printf("starting server at %v", server.Config.Address)
	err = httpdown.ListenAndServe(httpServer, hd)
	log.Printf("stopped server %v", err)
}
