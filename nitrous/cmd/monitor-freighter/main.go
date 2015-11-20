package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

const (
	workers = 8 // controls number of concurrent http calls

	// user will supply remainder of query
	queryf = `
	SELECT hosts.slug
	FROM hosts
	JOIN users ON hosts.user_id = users.id
	WHERE hosts.state = 'running'
	%s
	`
)

var (
	usage = fmt.Sprintf(`Usage: monitor-freighter -q <aux query> [--live-run]

	Where <aux query> is an addition to this query:
	
	%s

	Example usage:

	  monitor-freighter -q 'AND users.id = 3'
	`, queryf)

	liverun  = false
	auxQuery string

	dbConn *db.Database

	pollInterval time.Duration
)

// can use a channel of slugs (strings) instead -- this is light futureproofing
type Workspace struct {
	Slug string `db:"slug"`
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	flag.BoolVar(&liverun, "live-run", false, "enable live run")
	flag.StringVar(&auxQuery, "q", "", "custom SQL for workspaces to dispatch")
	flag.DurationVar(&pollInterval, "i", 10*time.Minute, "polling interval. Units are: 's', 'm', 'h'")

	envvars.CheckDB()
	dbURL := envvars.NitrousPgUrl

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("db.NewDatabase() failed with '%s'", err)
	}
}

func dispatcher(ch chan Workspace) {
	query := fmt.Sprintf(queryf, auxQuery)
	logrus.Info("Determining workspaces to query with %s", query)
	for {
		ws := []Workspace{}
		// run Select in transaction in case cmd user accidentally
		// passes in a query that changes db
		errRollback := fmt.Errorf("force rollback")
		err := dbConn.WithTransaction(func(tx *sqlx.Tx) error {
			if _, err := tx.Exec("SET TRANSACTION READ ONLY"); err != nil {
				return err
			}
			if err := tx.Select(&ws, query); err != nil {
				return err
			}
			return errRollback
		})
		if err != nil && err != errRollback {
			logrus.Error("Error in dispatcher query: %v", err)
		}
		logrus.Debugf("dispatching %d workspaces for monitoring", len(ws))
		if len(ws) > 0 {
			for _, w := range ws {
				ch <- w
			}
		}
		time.Sleep(pollInterval)
	}
}

func worker(ch chan Workspace) {
	for {
		select {
		case w := <-ch:
			ping(w)
		}
	}
}

func ping(w Workspace) {
	proxyURL := fmt.Sprintf("https://%s.nitrousproxy.com:65333", w.Slug)
	fields := logrus.Fields{
		"slug": w.Slug,
		"url":  proxyURL,
	}

	resp, err := http.Get(proxyURL)
	if err != nil {
		fields["err"] = err
		logrus.WithFields(fields).Error()
		return
	}
	avail := resp.StatusCode == http.StatusOK
	if avail {
		logrus.WithFields(fields).Info("passed")
	} else {
		logrus.WithFields(fields).Warn("failed")
	}
	h, err := host.FindByIdentifier(dbConn, w.Slug)
	if err != nil {
		fields["err"] = err
		logrus.WithFields(fields).Error("error finding host")
		return
	}
	if err := h.UpdateAvailability(dbConn, avail); err != nil {
		fields["err"] = err
		logrus.WithFields(fields).Error("error updating host")
		return
	}
}

func main() {
	flag.Parse()
	if auxQuery == "" {
		fmt.Fprintf(os.Stderr, usage)
		os.Exit(1)
	}

	if !liverun {
		logrus.Warnf("Operating as a dry-run, no e-mails will be sent")
	} else {
		logrus.Warnf("--live-run specified, users will be e-mailed")
	}

	logrus.WithFields(logrus.Fields{
		"liverun":          liverun,
		"aux_query":        auxQuery,
		"num_workers":      workers,
		"polling_interval": pollInterval,
	}).Info("running with this config")

	wch := make(chan Workspace)
	for i := 0; i < workers; i++ {
		go worker(wch)
	}
	dispatcher(wch)
}
