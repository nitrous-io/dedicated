package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
	"github.com/stripe/stripe-go"
)

const HostQueueName = "v1.host"

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

	envvars.CheckCA()
	if _, err := certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath); err != nil {
		panic(fmt.Errorf("Error with keypair (%s,%s): %s", envvars.CACertPath, envvars.CAKeyPath, err))
	}

	envvars.CheckSendgrid()
	envvars.CheckHostnames()
	envvars.CheckGitHubOAuth()
	envvars.CheckSecretToken()

	envvars.CheckStripe()
	stripe.Key = envvars.StripeSecretKey

	// setup database
	envvars.CheckDB()
	dbConn, err := db.NewDatabase(envvars.NitrousPgUrl)
	if err != nil {
		logr.FatalExit(err)
	}
	defer dbConn.DB.Close()
	if err := dbConn.MigrateAll(); err != nil {
		logr.Infof("nitrousd: migrations failed: %v", err)
	}

	var libratoInterval time.Duration
	li := envvars.LibratoInterval
	if li == "" {
		libratoInterval = 120 * time.Second
	} else {
		libratoInterval, err = time.ParseDuration(li)
		if err != nil {
			logr.FatalExit("nitrousd: LIBRATO_INTERVAL invalid:", err.Error())
		}
	}

	libratoUser := envvars.LibratoUser
	libratoToken := envvars.LibratoToken

	if libratoUser == "" ||
		libratoToken == "" ||
		strings.HasPrefix(libratoUser, "disable") ||
		strings.HasPrefix(libratoToken, "disable") {
		logr.Warnf("missing librato config. Metrics logging is disabled")
		// in dev, we discard metrics output because we are not interested in it
		go metrics.Log(metrics.DefaultRegistry, 60e9,
			log.New(ioutil.Discard, "metrics: ", log.Lmicroseconds))
	} else {

		go librato.Librato(metrics.DefaultRegistry,
			libratoInterval,  // interval
			libratoUser,      // account owner email address
			libratoToken,     // Librato API token
			"nitrousd",       // source
			[]float64{0.99},  // precentiles to send
			time.Millisecond, // time unit
		)
	}

	pu := envvars.ProvisionerUser
	if pu == "" {
		logr.FatalExit("nitrousd: PROVISIONER_USER is not defined")
	}
	pi, err := time.ParseDuration(envvars.ProvisionerInterval)
	if err != nil {
		logr.FatalExit("nitrousd: PROVISIONER_INTERVAL is not a valid duration: %v", err)
	}

	conf := &provisioner.Config{Username: pu, Interval: pi}

	svr := server.NewServer(envvars.RootDir, dbConn, logr, conf)

	st, err := time.ParseDuration(envvars.StopTimeout)
	if err != nil {
		log.Fatalf("nitrousd: STOP_TIMEOUT is not a valid duration: %v", err)
	}
	svr.StopTimeout = st

	kt, err := time.ParseDuration(envvars.KillTimeout)
	if err != nil {
		log.Fatalf("nitrousd: KILL_TIMEOUT is not a valid duration: %v", err)
	}
	svr.KillTimeout = kt

	// Setup queue consumer
	go queue.StartConsumer(
		envvars.RedisHost,
		envvars.RedisPort,
		envvars.RedisDB,
		HostQueueName,
		svr.HostQueueHandler(),
		svr.QueueStopCh,
		envvars.QueueHandlerTimeout,
	)

	pid := os.Getpid()
	host := "0.0.0.0"
	port := uint16(3333)

	logr.Infof("nitrousd %s (pid: %v) listening on %s:%d", version, pid, host, port)
	err = svr.Start(host, port)
	if err != nil {
		logr.Errorf("Error starting nitrousd: %s", err)
	}
}
