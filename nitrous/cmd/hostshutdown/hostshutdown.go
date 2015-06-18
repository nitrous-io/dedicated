package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
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

	if envvars.TrustKey == "" {
		logrus.Fatalf("TRUST_KEY is not defined")
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
		timeStart := time.Now()
		hosts, err := freetier.ExpiredHosts(dbConn)
		if err != nil {
			logrus.Fatalf("freetier.ExpiredHosts() failed with '%s'", err)
		}

		for _, hst := range hosts {
			wg.Add(1)
			hostsCh <- hst
		}
		wg.Wait()
		if n := len(hosts); n > 0 {
			logrus.Infof("Stopped %d hosts in %s", n, time.Since(timeStart))
		}
	}
}

func worker(dbConn *db.Database, wg *sync.WaitGroup, ch chan *host.Host) {
	for h := range ch {
		logrus.Infof("worker: received host %d", h.ID)
		controlHost(dbConn, h)
		wg.Done()
	}
}

func controlHost(dbConn *db.Database, hst *host.Host) {
	tx, err := dbConn.Beginx()
	if err != nil {
		logrus.Errorf("Unable to start transaction, err: %v", err)
		return
	}
	defer tx.Rollback()

	locked, err := db.TransactionLock(tx, db.HOST_SHUTDOWN_LOCK, int(hst.ID))
	if err != nil {
		logrus.Errorf("Unable to obtain lock on host %d, err: %v", hst.ID, err)
		return
	}
	if !locked {
		logrus.Warnf("Host %d is already locked by another process, skipping", hst.ID)
		return
	}

	logrus.Infof("Starting shut down on host %d for user %d", hst.ID, hst.UserID)
	timeStart := time.Now()

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, "hostcontrol")
	err = hostcontrol.Stop(dbConn, hst, headers, nil, initiatorID)
	if err != nil {
		logrus.Errorf("Unable to shutdown host %d, err: %v", hst.ID, err)
		return
	}
	logrus.Infof("Successfully shut down host %d for user %d in %s", hst.ID, hst.UserID, time.Since(timeStart))

	u := fmt.Sprintf("https://%s/api/v0/maintenance/user_refresh/%d", envvars.WebHostname, hst.UserID)
	data := url.Values{}
	data.Set("trust_key", envvars.TrustKey)
	client := http.DefaultClient
	resp, err := client.PostForm(u, data)
	if err != nil {
		logrus.Errorf("Error POSTing to %s for user %d, err: %v", u, hst.UserID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.Errorf("Unable to read HTTP response body, err: %s", err)
			return
		}
		logrus.Errorf("Unable to send user_refresh for user %d, response: %s", hst.UserID, b)
	}
}
