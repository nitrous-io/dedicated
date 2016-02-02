// The hostautoterminate command terminates hosts that were marked as
// "soft-terminated" by the idlefreehosts command (which "soft-terminates"
// inactive free hosts by cancelling their Stripe subscription).
package main

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
)

const nWorkers = 4

var (
	initiatorID int64

	dryrun = true
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	for _, arg := range os.Args {
		if arg == "--live-run" {
			dryrun = false
		}
	}

	if dryrun {
		logrus.Warnf("Operating as a dry-run, no destructive actions will occur")
	} else {
		logrus.Warnf("--live-run specified, will be performing destructive actions")
	}

	logrus.Infof("Starting host termination service with %d workers", nWorkers)
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatalf("%v is not defined", envvars.DBEnvVar)
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("failed to connect to database, err: %v", err)
	}

	if envvars.ProvisionerUser == "" {
		logrus.Fatal("PROVISIONER_USER is not defined")
	}

	err = dbConn.QueryRow(`SELECT id FROM users WHERE username = $1`, envvars.ProvisionerUser).Scan(&initiatorID)
	if err != nil {
		logrus.Fatalf("failed to locate the ID of the PROVISIONER_USER, err: %v", err)
	}

	var wg sync.WaitGroup
	hostsCh := make(chan *host.Host)
	for i := 0; i < nWorkers; i++ {
		go worker(dbConn, &wg, hostsCh)
	}

	start := time.Now()
	hosts, err := host.FindAllForTermination(dbConn)
	if err != nil {
		logrus.Fatalf("host.FindAllForTermination() failed with '%s'", err)
	}

	for _, hst := range hosts {
		wg.Add(1)
		logrus.Infof("Queueing host %s (user %d) for termination", hst.Slug, hst.UserID)
		hostsCh <- hst
	}
	wg.Wait()

	if n := len(hosts); n > 0 {
		logrus.Infof("Terminated %d hosts in %s", n, time.Since(start))
	}

	logrus.Info("Host termination service done. Exiting.")
}

func worker(dbConn *db.Database, wg *sync.WaitGroup, ch chan *host.Host) {
	for h := range ch {
		termHost(dbConn, h)
		wg.Done()
	}
}

func termHost(dbConn *db.Database, hst *host.Host) {
	logrus.Infof("Starting terminate on host %s (user %d)", hst.Slug, hst.UserID)
	start := time.Now()

	if dryrun {
		logrus.Infof("dry-run active, *would* terminate %s", hst.Slug)
		return
	}

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, "hostautoterminate")
	err := dbConn.WithTransaction(func(tx *sqlx.Tx) error {
		return hostcontrol.Terminate(tx, hst, headers, initiatorID, hst.State)
	})
	if err != nil {
		logrus.Errorf("Unable to terminate host %s, err: %v", hst.Slug, err)
		return
	}
	logrus.Infof("Successfully terminated host %s (user %d) in %s", hst.Slug, hst.UserID, time.Since(start))
}
