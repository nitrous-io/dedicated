package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/kardianos/osext"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous-common/must"
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
	"github.com/rcrowley/goagain"
	"github.com/stripe/stripe-go"
)

var (
	version           string // current version of nitrousd
	freighterdVersion string // freighterd version that works with current nitrousd
	nitroVersion      string // nitro version that works with current nitrousd

	libratoUser    string
	libratoToken   string
	disableLibrato bool

	// StopTimeout is the duration before we stop waiting for existing
	// connections to complete.
	stopTimeout = 1 * time.Minute
)

func init() {
	// The Double strategy is Upstart-compatible. The parent forks a child, the
	// child execs, the child signals the parent, the parent execs, and finally
	// the parent kills the child.
	goagain.Strategy = goagain.Double

	libratoUser = envvars.LibratoUser
	libratoToken = envvars.LibratoToken

	if libratoUser == "" ||
		libratoToken == "" ||
		strings.HasPrefix(libratoUser, "disable") ||
		strings.HasPrefix(libratoToken, "disable") {
		disableLibrato = true
	}

	// check CA
	envvars.CheckCAVariables()
	if _, err := certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath); err != nil {
		panic(fmt.Errorf("Error with keypair (%s,%s): %s", envvars.CACertPath, envvars.CAKeyPath, err))
	}

	envvars.CheckSendgridVariables()
	envvars.CheckHostnames()
	envvars.CheckGitHubOAuth()

	stripe.Key = envvars.StripeSecretKey
	if stripe.Key == "" {
		panic("STRIPE_SECRET_KEY environment variable not set")
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
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalfExit("nitrousd: %s is not defined", envvars.DBEnvVar)
	}
	dbConn, err := db.NewDatabase(dbURL)
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

	if disableLibrato {
		logr.Warnf("missing librato config. Metrics logging is disabled")
		// in dev, we flush metrics every 60s into devNull
		// because we are not interested in them
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
	pi := envvars.ProvisionerInterval
	interval, err := time.ParseDuration(pi)
	if err != nil {
		logr.FatalExit("nitrousd: PROVISIONER_INTERVAL invalid:", err.Error())
	}
	conf := &provisioner.Config{Username: pu, Interval: interval}
	pid := os.Getpid()
	svr := server.NewServer(must.String(osext.ExecutableFolder()), dbConn, logr, conf)

	done := make(chan struct{})
	serve := func(ln net.Listener) {
		svr.Start(ln)
		close(done)
	}

	listener, err := goagain.Listener()
	if err != nil {
		// If there's an error, it means that there isn't an existing listener,
		// so we create one.
		addr := "0.0.0.0:3333"
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			logr.FatalfExit("nitrousd: failed to listen on %q, error: %q", addr, err)
		}

		logr.Infof("nitrousd %s (pid: %v) listening on %v", version, pid, listener.Addr())
		go serve(listener)

	} else {
		logr.Infof("nitrousd: resuming listening on %v", listener.Addr())

		logr.Infof("nitrousd %s (pid: %v) listening on %v", version, pid, listener.Addr())
		go serve(listener)

		// If this is the child, send the parent SIGUSR2. If this is the parent,
		// send the child SIGQUIT.
		if err := goagain.Kill(); err != nil {
			logr.FatalfExit("nitrousd: failed to signal parent/child, error: %q", err)
		}
	}

	// Wait for signals.
	sig, err := goagain.Wait(listener)
	if err != nil {
		logr.FatalfExit("nitrousd: error waiting for signals in main goroutine, error: %q", err)
	}

	logr.Infof("nitrousd (pid: %v) got signal %q. Stopping.", pid, sig)
	svr.Stop()
	logr.Infof("nitrousd (pid: %v) stopped.", pid)

	// Stop gracefully on SIGQUIT or SIGUSR2.
	if sig == goagain.SIGQUIT || sig == goagain.SIGUSR2 {
		logr.Infof("nitrousd: received %q, will stop gracefully", goagain.SIGUSR2)
		// Wait for existing connections to complete.
		select {
		case <-done:
			logr.Info("nitrousd: all existing connections have completed")
		case <-time.After(stopTimeout):
			logr.Info("nitrousd: timed out waiting for all existing connections to complete")
		}
	}

	// If we received SIGUSR2, re-exec the parent process.
	if sig == goagain.SIGUSR2 {
		logr.Infof("nitrousd: received SIGUSR2, will restart gracefully")
		if err := goagain.Exec(listener); err != nil {
			logr.FatalfExit("nitrousd: failed to re-exec with the same listener, error: %q", err)
		}
	}
}
