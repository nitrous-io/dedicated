package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"bitbucket.org/kardianos/osext"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous-common/must"
)

var (
	version           string // current version of nitrousd
	freighterdVersion string // freighterd version that works with current nitrousd
	nitroVersion      string // nitro version that works with current nitrousd
)

func main() {
	flVersion := flag.Bool("v", false, "Display the version and exit")
	flag.Parse()

	if *flVersion {
		fmt.Printf("nitrousd version %v\n", version)
		os.Exit(0)
	}

	logr := &logger.Logger{Level: logger.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			logr.FatalExit(r)
		}
	}()

	dbURL := os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		logr.FatalExit("nitrousd: NITROUS_PG_URL is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	defer dbConn.DB.Close()
	if err := dbConn.MigrateAll(); err != nil {
		logr.Infof("nitrousd: migrations failed: %v", err)
	}

	svr := server.NewServer(must.String(osext.ExecutableFolder()), dbConn, logr)
	pid := os.Getpid()
	host := "0.0.0.0"
	port := uint16(3333)
	logr.Infof("nitrousd %s (pid: %v) listening on %s:%d", version, pid, host, port)
	go svr.Start(host, port)

	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt, os.Kill)
	s := <-ch
	logr.Infof("nitrousd (pid: %v) got signal %v. Terminating.", pid, s)
	svr.Stop()
	logr.Infof("nitrousd (pid: %v) terminated.", pid)
}
