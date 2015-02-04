package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder"
)

var version string

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
	// start the server
	server, err := hostbuilder.New(configData)
	if err != nil {
		log.Printf("hostbuilder: failed to create server %v", err)
		os.Exit(1)
	}
	http.Handle("/", server.Router)
	log.Printf("hostbuilder: starting server at %v", server.Config.Address)
	httpServer := &http.Server{Addr: server.Config.Address, Handler: nil}
	go func() {
		log.Fatal(httpServer.ListenAndServe())
	}()

	ch := make(chan os.Signal)
	// upstart uses term by default
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)
	log.Printf("Starting to shut down hostbuilder, %d request(s) in progress, will wait max %d seconds", hostbuilder.GetHttpReqCount(), server.Config.ShutdownTimeoutSecs)
	timeStart := time.Now()
	httpServer.SetKeepAlivesEnabled(false) // recommended when shutting down
	hostbuilder.ShutdownAndWaitHttpRequestsDone(time.Duration(server.Config.ShutdownTimeoutSecs) * time.Second)
	log.Printf("hostbuilder did shutdown in %s", time.Since(timeStart))
}
