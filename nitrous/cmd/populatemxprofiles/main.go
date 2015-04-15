package main

import (
	"database/sql"
	"os"
	"strings"
	"sync"

	"github.com/lib/pq"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
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

type User struct {
	ID             int64          `db:"user_id"`
	Name           string         `db:"name"`
	Username       string         `db:"username"`
	Email          string         `db:"email"`
	CreatedAt      pq.NullTime    `db:"created_at"`
	ConfirmedAt    pq.NullTime    `db:"confirmed_at"`
	GithubUsername sql.NullString `db:"github_username"`
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

func NewJob(mode string, uid int64, p map[string]interface{}) job {
	return job{mode: mode, uid: uid, p: p}
}

func makeJobs() <-chan job {
	jobs := make(chan job)

	users := []*User{}
	err := dbConn.Select(&users, `
	SELECT users.id AS user_id, users.name, users.username, users.email,
		users.created_at, users.confirmed_at,
		user_profiles.github_user_name AS github_username
	FROM users
	JOIN user_profiles ON user_profiles.user_id = users.id
	WHERE service_account IS false ORDER BY users.id
	`)
	if err != nil {
		logr.FatalExit(err)
	}

	type stepFn func(chan job, *User) error

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
