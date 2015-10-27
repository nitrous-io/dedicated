package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

const (
	workers = 4 // controls number of concurrent http calls

	// user will supply remainder of query
	queryf = `
	SELECT hosts.slug, users.email
	FROM hosts
	JOIN users ON hosts.user_id = users.id
	WHERE hosts.state = 'running'
	%s
	`
)

var (
	usage = fmt.Sprintf(`Usage: monitor-freighter -q <aux query> [--live-run] [-d duration between mails]

	Where <aux query> is an addition to this query:
	
	%s

	Example usage:

	  monitor-freighter -q 'AND users.id = 3'
	`, queryf)

	liverun  = false
	auxQuery string

	dbConn *db.Database

	pollInterval, mailInterval time.Duration
	failsBeforeMail            int // number of failed attempts to wait between e-mails
	sgMailer                   mailer.Mailer

	mu          sync.Mutex
	consecFails map[string]int // workspace slug -> number of times it has failed to ping
)

type Workspace struct {
	Slug  string `db:"slug"`
	Email string `db:"email"`
}

func init() {
	consecFails = make(map[string]int)

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	flag.BoolVar(&liverun, "live-run", false, "enable live run")
	flag.StringVar(&auxQuery, "q", "", "custom SQL for workspaces to dispatch")
	flag.DurationVar(&mailInterval, "d", 24*time.Hour, "duration between mails. Units are: 's', 'm', 'h'")
	flag.DurationVar(&pollInterval, "i", 1*time.Minute, "polling interval. Units are: 's', 'm', 'h'")

	envvars.CheckDB()
	dbURL := envvars.NitrousPgUrl

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("db.NewDatabase() failed with '%s'", err)
	}

	envvars.CheckSendgrid()
	sgMailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

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
		"url":   proxyURL,
		"email": w.Email,
	}

	resp, err := http.Get(proxyURL)
	if err != nil {
		fields["err"] = err
		logrus.WithFields(fields).Error()
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if resp.StatusCode == http.StatusOK {
		logrus.WithFields(fields).Info("passed")
		consecFails[w.Slug] = 0
		return
	}
	consecFails[w.Slug] += 1
	numFails := consecFails[w.Slug]
	fields["code"] = resp.StatusCode
	fields["fails"] = numFails
	logrus.WithFields(fields).Warn("bad")

	// we don't want to repeatedly e-mail, so only mail once after 5 failed
	// attempts, and thereafter every N attempts (where N is calculated
	// from polling interval to be less frequent than once an hour)
	if (numFails-5)%failsBeforeMail != 0 {
		return
	}
	if !liverun {
		logrus.WithFields(fields).Info("not e-mailing user (dry run)")
		return
	}
	from := envvars.SupportEmail
	to := w.Email
	subject := fmt.Sprintf("Workspace %s unavailable for %s", w.Slug, time.Duration(float64(numFails)*pollInterval.Seconds())*time.Second)
	fields["from"] = from
	fields["to"] = to
	fields["subject"] = subject
	logrus.WithFields(fields).Info("sending e-mail")
	err = sgMailer.SendMail(from, []string{to}, []string{from}, nil, from,
		subject, "(no body)", "(no body)")
	if err != nil {
		fields["err"] = err
		logrus.WithFields(fields).Error("error sending email")
	}
}

func main() {
	flag.Parse()
	if auxQuery == "" {
		fmt.Fprintf(os.Stderr, usage)
		os.Exit(1)
	}
	if pollInterval.Seconds() >= mailInterval.Seconds() {
		logrus.WithFields(logrus.Fields{
			"polling_interval": pollInterval,
			"mail_interval":    mailInterval,
		}).Fatalf("Polling interval must be less than mail interval")
	}
	// e-mails are spaced out so they will not be more than specified mail interval
	failsBeforeMail = int(mailInterval.Seconds()) / int(pollInterval.Seconds())

	if !liverun {
		logrus.Warnf("Operating as a dry-run, no e-mails will be sent")
	} else {
		logrus.Warnf("--live-run specified, users will be e-mailed")
	}

	logrus.WithFields(logrus.Fields{
		"liverun":                       liverun,
		"aux_query":                     auxQuery,
		"num_workers":                   workers,
		"polling_interval":              pollInterval,
		"mail_interval":                 mailInterval,
		"failed_attempts_between_mails": failsBeforeMail,
	}).Info("running with this config")

	wch := make(chan Workspace)
	for i := 0; i < workers; i++ {
		go worker(wch)
	}
	dispatcher(wch)
}
