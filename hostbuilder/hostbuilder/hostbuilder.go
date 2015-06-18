package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/facebookgo/httpdown"
	"github.com/nitrous-io/dedicated/hostbuilder"
)

var (
	version string

	// StopTimeout is the duration before we begin force closing connections.
	// Defaults to 15 minutes.
	defaultStopTimeout = 900 * time.Second // 15 * time.Minute

	// KillTimeout is the duration before which we completely give up and abort
	// even though we still have connected clients. This is useful when a large
	// number of client connections exist and closing them can take a long time.
	// Note, this is in addition to the StopTimeout. Defaults to 1 minute.
	defaultKillTimeout = 60 * time.Second // 1 * time.Minute
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
		logrus.Fatal("hostbuilder: missing config file")
	}
	configData, err := ioutil.ReadFile(*flConfig)
	if err != nil {
		logrus.Fatal("hostbuilder: failed to read config file")
	}

	// build the server routes
	server, err := hostbuilder.New(configData)
	if err != nil {
		logrus.Fatalf("hostbuilder: failed to create server %v", err)
	}
	http.Handle("/", server.Router)

	// create the http server
	httpServer := &http.Server{Addr: server.Config.Address, Handler: nil}

	// setup httpdown wrapper so we can gracefully stop
	stopTimeout := time.Duration(server.Config.StopTimeout) * time.Second
	if stopTimeout == 0 {
		stopTimeout = defaultStopTimeout
	}
	killTimeout := defaultKillTimeout
	hd := &httpdown.HTTP{
		StopTimeout: stopTimeout,
		KillTimeout: killTimeout,
	}

	// start listening to request
	// The server will be gracefully stopped when it receives SIGTERM or SIGINT
	// so another server can be started in its place.
	logrus.Infof("starting server at %v", server.Config.Address)
	err = httpdown.ListenAndServe(httpServer, hd)
	logrus.Infof("stopped server %v", err)
}
