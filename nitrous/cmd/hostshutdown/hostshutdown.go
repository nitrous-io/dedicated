package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/nitrous-common/httpclient"
	"github.com/nitrous-io/nitrous-common/logger"
)

const (
	nWorkers = 4
)

var (
	logr          *logger.Logger
	checkInterval time.Duration = 5 * time.Second
)

func main() {
	var wg sync.WaitGroup

	logr = &logger.Logger{Level: logger.LogLevelDebug}
	logr.Infof("starting host shutdown service with %d workers", nWorkers)
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalfExit("db.NewDatabase() failed with '%s'", err)
	}

	if envvars.WebHostname == "" {
		logr.FatalfExit("WEB_HOSTNAME is not defined")
	}

	if envvars.TrustKey == "" {
		logr.FatalfExit("TRUST_KEY is not defined")
	}

	hostsCh := make(chan *host.Host)
	for i := 0; i < nWorkers; i++ {
		go worker(dbConn, &wg, hostsCh)
	}

	ticker := time.NewTicker(checkInterval)
	for _ = range ticker.C {
		timeStart := time.Now()
		hosts, err := freetier.ExpiredHosts(dbConn)
		if err != nil {
			logr.FatalfExit("freetier.ExpiredHosts() failed with '%s'", err)
		}

		for _, hst := range hosts {
			wg.Add(1)
			hostsCh <- hst
		}
		wg.Wait()
		if n := len(hosts); n > 0 {
			logr.Infof("Stopped %d hosts in %s", n, time.Since(timeStart))
		}
	}
}

func worker(dbConn *db.Database, wg *sync.WaitGroup, ch chan *host.Host) {
	for h := range ch {
		logr.Infof("worker: received host %d", h.ID)
		controlHost(dbConn, h)
		wg.Done()
	}
}

func controlHost(dbConn *db.Database, hst *host.Host) {
	tx, err := dbConn.Beginx()
	if err != nil {
		logr.Errorf("Unable to start transaction, err: %v", err)
		return
	}
	defer tx.Rollback()

	locked, err := db.TransactionLock(tx, db.HOST_SHUTDOWN_LOCK, int(hst.ID))
	if err != nil {
		logr.Errorf("Unable to obtain lock on host %d, err: %v", hst.ID, err)
		return
	}
	if !locked {
		logr.Warnf("Host %d is already locked by another process, skipping", hst.ID)
		return
	}

	logr.Infof("Starting shut down on host %d for user %d", hst.ID, hst.UserID)
	timeStart := time.Now()
	err = hostcontrol.Stop(dbConn, logr, hst, nil)
	if err != nil {
		logr.Errorf("Unable to shutdown host %d, err: %v", hst.ID, err)
		return
	}
	logr.Infof("Successfully shut down host %d for user %d in %s", hst.ID, hst.UserID, time.Since(timeStart))

	reqUrl := fmt.Sprintf("https://%s/api/v0/maintenance/user_refresh/%d", envvars.WebHostname, hst.UserID)
	c := httpclient.New("")
	resp, _ := c.PostForm(reqUrl, &url.Values{"trust_key": []string{envvars.TrustKey}})
	if err := httpclient.CheckResponse(resp, http.StatusOK); err != nil {
		logr.Errorf("Unable to send user_refresh for user %d, err: %+v", hst.ID, err)
	}
}
