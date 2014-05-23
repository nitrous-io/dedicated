package main

import (
	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider/docker"
	"github.com/nitrous-io/freighter/server"
	"github.com/nitrous-io/freighter/util"
)

func main() {
	logger := &util.Logger{Level: util.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			logger.FatalExit(r)
		}
	}()

	store, err := datastore.NewSqliteDatastore("freighter.sqlite3")
	if err != nil {
		logger.FatalExit(err)
	}

	prov, err := docker.NewDockerProvider(store)
	if err != nil {
		logger.FatalExit(err)
	}

	svr := server.NewServer(logger, prov, &server.HTTPCallback{})
	host := "0.0.0.0"
	port := uint16(3342)
	logger.Infof("freighterd listening on %s:%d", host, port)
	svr.Start(host, port)
}
