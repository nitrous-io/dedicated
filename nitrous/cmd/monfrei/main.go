package main

import (
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
	workers  = 4 // controls number of concurrent http calls
	interval = 1 * time.Minute

	usage = `monfrei [--live-run]`

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
	dryrun = true

	dbConn *db.Database

	failsBeforeMail int // number of failed attempts to wait between e-mails
	sgMailer        mailer.Mailer

	mu          sync.Mutex
	consecFails map[string]int // workspace slug -> number of times it has failed to ping
)

type Workspace struct {
	Slug  string `db:"slug"`
	Email string `db:"email"`
}

func init() {
	consecFails = make(map[string]int)

	// e-mails are spaced out so they will not be more than once an hour
	if interval.Seconds() < 3600 {
		failsBeforeMail = 3600 / int(interval.Seconds())
	}

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	for _, arg := range os.Args[1:] {
		if arg == "--live-run" {
			dryrun = false
		}
	}

	if dryrun {
		logrus.Warnf("Operating as a dry-run, no e-mails will be sent")
	} else {
		logrus.Warnf("--live-run specified, users will be e-mailed")
	}

	envvars.CheckDB()
	dbURL := envvars.NitrousPgUrl

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("db.NewDatabase() failed with '%s'", err)
	}

	envvars.CheckSendgrid()
	sgMailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

	logrus.WithFields(logrus.Fields{
		"num workers":                   workers,
		"polling interval":              interval,
		"failed attempts between mails": failsBeforeMail,
	}).Info("running with this config")
}

func dispatcher(ch chan Workspace) {
	for {
		ws := []Workspace{}
		// run Select in transaction in case cmd user accidentally
		// passes in a query that changes db
		_ = dbConn.WithTransaction(func(tx *sqlx.Tx) error {
			serr := tx.Select(&ws, fmt.Sprintf(queryf, "AND hosts.id < 0"))
			if serr != nil {
				logrus.Error("Error in dispatcher query: %v", serr)
			}
			return fmt.Errorf("force rollback")
		})
		logrus.Debugf("dispatching %d workspaces for monitoring", len(ws))
		if len(ws) > 0 {
			for _, w := range ws {
				ch <- w
			}
		}
		time.Sleep(interval)
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
	} else {
		consecFails[w.Slug] += 1
		numFails := consecFails[w.Slug]
		fields["code"] = resp.StatusCode
		fields["fails"] = numFails
		logrus.WithFields(fields).Warn("bad")

		// we don't want to repeatedly e-mail, so only mail once after 5 failed
		// attempts, and thereafter every N attempts (where N is calculated
		// from polling interval to be less frequent than once an hour)
		if (numFails-5)%failsBeforeMail == 0 {
			if dryrun {
				logrus.WithFields(fields).Info("not e-mailing user (dryrun)")
				return
			}
			from := envvars.SupportEmail
			to := w.Email
			subject := fmt.Sprintf("Workspace %s unavailable for %s", w.Slug, time.Duration(float64(numFails)*interval.Seconds())*time.Second)
			fields["from"] = from
			fields["to"] = to
			fields["subject"] = subject
			logrus.WithFields(fields).Info("sending e-mail")
			err := sgMailer.SendMail(from, []string{to}, []string{from}, nil, from,
				subject, "(no body)", "(no body)")
			if err != nil {
				fields["err"] = err
				logrus.WithFields(fields).Error("error sending email")
			}
		}
	}
}

func main() {
	wch := make(chan Workspace)
	for i := 0; i < workers; i++ {
		go worker(wch)
	}
	dispatcher(wch)
}
