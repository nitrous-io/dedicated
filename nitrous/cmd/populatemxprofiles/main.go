package main

import (
	"os"
	"strings"
	"sync"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/nitrous-common/logger"
)

// run with --live-run to actually send to mixpanel, otherwise it will be a dry run

const maxConcurrent = 4

type job struct {
	mode string
	uid  int64
	p    map[string]interface{}
}

var (
	logr    *logger.Logger
	tracker analytics.Tracker
	dbConn  *db.Database
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

func (j job) Process() {
	j.p["$ignore_time"] = true
	j.p["$ip"] = "0"
	logr.Debugf("Processing job %d: %s %+v", j.uid, j.mode, j.p)
	finished := tracker.SetProfileProps(j.mode, j.uid, j.p, logr)
	if finished != nil {
		<-finished
	}
}

func makeJobs() <-chan job {
	jobs := make(chan job)

	users := []*user.User{}
	err := dbConn.Select(&users, "SELECT * FROM users WHERE service_account IS false ORDER BY id")
	if err != nil {
		logr.FatalExit(err)
	}

	type stepFn func(chan job, *user.User) error

	steps := map[string]stepFn{
		"set confirmed": SetConfirmed,
		"set templates": SetTemplates,
	}

	go func() {
		defer close(jobs)
		for _, u := range users {
			for name, fn := range steps {
				if err := fn(jobs, u); err != nil {
					logr.Errorf("User %s, step %s: %v", u.Email, name, err)
				}
			}
		}
	}()

	return jobs
}

func worker(jobs <-chan job) {
	for j := range jobs {
		j.Process()
	}
}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}

	mixpanelToken := envvars.MixpanelToken
	if mixpanelToken == "" || strings.HasPrefix(mixpanelToken, "disable") {
		logr.FatalExit(envvars.MixpanelEnvVar, "is not defined")
	}
	tracker = &DryTracker{}
	if len(os.Args) > 1 && os.Args[1] == "--live-run" {
		tracker = analytics.NewMixpanelTracker(envvars.MixpanelToken)
		logr.Warn("Live run!")
	}

	wg := sync.WaitGroup{}
	jobs := makeJobs()
	for i := 0; i < maxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(jobs)
		}()
	}
	logr.Infof("Done!")
	wg.Wait()
}
