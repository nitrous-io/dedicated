package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/kardianos/osext"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous-common/must"
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
)

var (
	version           string // current version of nitrousd
	freighterdVersion string // freighterd version that works with current nitrousd
	nitroVersion      string // nitro version that works with current nitrousd

	libratoUser    string
	libratoToken   string
	disableLibrato bool
)

func init() {
	libratoUser = os.Getenv("LIBRATO_USER")
	libratoToken = os.Getenv("LIBRATO_TOKEN")

	if libratoUser == "" ||
		libratoToken == "" ||
		strings.HasPrefix(libratoUser, "disable") ||
		strings.HasPrefix(libratoToken, "disable") {
		disableLibrato = true
	}
}

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

	// setup database
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

	// configure metrics output. if we do not have librato user/token i.e.
	// development environment, output the metrics to stderr
	// by default we flush metrics every 10s
	if disableLibrato {
		logr.Warnf("missing librato config. Metrics logging is disabled")
		// in dev, we flush metrics every 60s into devNull
		// because we are not interested in them
		go metrics.Log(metrics.DefaultRegistry, 60e9,
			log.New(ioutil.Discard, "metrics: ", log.Lmicroseconds))
	} else {
		go librato.Librato(metrics.DefaultRegistry,
			10e9,                        // interval
			libratoUser,                 // account owner email address
			libratoToken,                // Librato API token
			"nitrousd",                  // source
			[]float64{0.75, 0.95, 0.99}, // precentiles to send
			time.Millisecond,            // time unit
		)
	}

	// TODO: use some other mechanism for populating preprovisioner config
	pu := os.Getenv("PROVISIONER_USER")
	if pu == "" {
		logr.FatalExit("nitrousd: PROVISIONER_USER is not defined")
	}
	pw := os.Getenv("PROVISIONER_WORKERS")
	if pw == "" {
		logr.FatalExit("nitrousd: PROVISIONER_WORKERS is not defined")
	}
	workers, err := strconv.Atoi(pw)
	if err != nil {
		logr.FatalExit("nitrousd: PROVISIONER_WORKERS invalid:", err.Error())
	}
	conf := &provisioner.Config{
		Username:   pu,
		NumWorkers: workers,
	}

	svr := server.NewServer(must.String(osext.ExecutableFolder()),
		dbConn, logr, conf)
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
