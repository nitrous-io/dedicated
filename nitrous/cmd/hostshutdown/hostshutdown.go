package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/pkg/uuid"
)

const (
	nWorkers = 4
)

var (
	checkInterval time.Duration = 5 * time.Second
	initiatorID   int64
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	var wg sync.WaitGroup

	logrus.Infof("starting host shutdown service with %d workers", nWorkers)
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("db.NewDatabase() failed with '%s'", err)
	}

	if envvars.WebHostname == "" {
		logrus.Fatalf("WEB_HOSTNAME is not defined")
	}

	if envvars.ProvisionerUser == "" {
		logrus.Fatal("PROVISIONER_USER is not defined")
	}

	err = dbConn.QueryRow(`SELECT id FROM users WHERE username = $1`, envvars.ProvisionerUser).Scan(&initiatorID)
	if err != nil {
		logrus.Fatalf("Failed to locate the ID of the PROVISIONER_USER. Err: '%s'", err)
	}

	hostsCh := make(chan *host.Host)
	for i := 0; i < nWorkers; i++ {
		go worker(dbConn, &wg, hostsCh)
	}

	ticker := time.NewTicker(checkInterval)
	for _ = range ticker.C {
		hosts, err := freetier.IdleFreeHosts(dbConn)
		if err != nil {
			logrus.Fatalf("freetier.IdleFreeHosts() failed with '%s'", err)
		}

		for _, hst := range hosts {
			wg.Add(1)
			hostsCh <- hst
		}
		wg.Wait()
	}
}

func worker(dbConn *db.Database, wg *sync.WaitGroup, ch chan *host.Host) {
	for h := range ch {
		logrus.Infof("worker: received host %d", h.ID)
		err := stopHost(dbConn, h)
		if err != nil {
			logrus.Errorf("worker: error stopping host: %v", err)
		} else {
			logrus.Infof("worker: stopped host %d", h.ID)
		}
		wg.Done()
	}
}

func stopHost(dbConn *db.Database, hst *host.Host) error {
	tx, err := dbConn.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	gotLock, err := db.TransactionLock(tx, db.HOST_SHUTDOWN_LOCK, int(hst.ID))
	if err != nil {
		return fmt.Errorf("unable to obtain lock on host %d, err: %v", hst.ID, err)
	}
	if !gotLock {
		return fmt.Errorf("host %d is already locked by another process", hst.ID)
	}

	// Confirm that host is still eligible for shutdown - user might have used
	// the host since the time the last query was ran, and we don't want to
	// incorrectly shutdown the host at this point.
	idle, err := freetier.IsIdle(dbConn, hst)
	if err != nil {
		return fmt.Errorf("error confirming that host %d is still idle, err: %v", hst.ID, err)
	}

	if !idle {
		return fmt.Errorf("host %d is no longer idle", hst.ID)
	}

	logrus.Infof("Starting shutdown on host %d for user %d", hst.ID, hst.UserID)
	start := time.Now()

	requestID := fmt.Sprintf("%s-hostcontrol", uuid.Make())
	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, requestID)

	if err := hostcontrol.StopWithQueue(dbConn, hst, headers, nil, initiatorID); err != nil {
		return fmt.Errorf("unable to shutdown host %d, err: %v", hst.ID, err)
	}
	logrus.Infof("Started shutdown on host %d for user %d after %s", hst.ID, hst.UserID, time.Since(start))

	return nil
}
