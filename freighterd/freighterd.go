package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider/docker"
	"github.com/nitrous-io/freighter/server"
	"github.com/nitrous-io/freighter/util"
)

var version string

func main() {
	flData := flag.String("data", "freighter.sqlite3", "location of data store")
	flPort := flag.Int("port", 3342, "listening port")
	flVersion := flag.Bool("v", false, "current version")
	flag.Parse()

	if *flVersion {
		fmt.Printf("Freighterd: current version %v\n", version)
		os.Exit(0)
	}

	logger := &util.Logger{Level: util.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			logger.FatalExit(r)
		}
	}()

	store, err := datastore.NewSqliteDatastore(*flData)
	if err != nil {
		logger.FatalExit(err)
	}

	prov, err := docker.NewDockerProvider(store)
	if err != nil {
		logger.FatalExit(err)
	}

	svr := server.NewServer(logger, prov, &server.HTTPCallback{})
	host := "0.0.0.0"
	port := uint16(*flPort)
	logger.Infof("freighterd listening on %s:%d", host, port)
	svr.Start(host, port)
}
