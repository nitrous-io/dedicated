package main

import (
	"os"

	"github.com/nitrous-io/freighter/server"
	"github.com/nitrous-io/freighter/util"
)

func main() {
	logger := &util.Logger{Level: util.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			logger.Fatal(r)
			os.Exit(1)
		}
	}()

	s := &server.Server{Logger: logger}
	host := "0.0.0.0"
	port := uint16(3342)
	logger.Infof("freighterd listening on %s:%d", host, port)
	s.Start(host, port)
}
