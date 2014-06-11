package main

import (
	"os"

	"github.com/nitrous-io/nitrous/pkg/logger"
	"github.com/nitrous-io/nitrous/server"
)

func main() {
	l := &logger.Logger{Level: logger.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			l.FatalExit(r)
		}
	}()

	pid := os.Getpid()

	svr := server.NewServer(l)
	host := "0.0.0.0"
	port := uint16(3000)
	l.Infof("nitrousd (pid: %v) listening on %s:%d", pid, host, port)
	svr.Start(host, port)
}
