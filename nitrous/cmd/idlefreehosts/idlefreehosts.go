package main

import (
	"os"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/idlehosts"
)

const (
	nWorkers = 4
)

const (
	OP_FIRST_WARNING = "first warning"
	OP_FINAL_WARNING = "final warning"
	OP_TERMINATION   = "termination"
)

var (
	dryrun = true
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

type wrkMsg struct {
	Target *idlehosts.Target
	Op     string
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

	var wg sync.WaitGroup

	logrus.Infof("starting idle hosts service with %d workers", nWorkers)
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("db.NewDatabase() failed with '%s'", err)
	}

	msgsCh := make(chan *wrkMsg)
	for i := 0; i < nWorkers; i++ {
		go worker(dbConn, &wg, msgsCh)
	}

	targets, err := idlehosts.TerminationTargets(dbConn)
	if err != nil {
		logrus.Fatalf("idlehosts.TerminationTargets() failed with '%s'", err)
	}
	queueTargets(OP_TERMINATION, targets, &wg, msgsCh)

	targets, err = idlehosts.FinalWarningTargets(dbConn)
	if err != nil {
		logrus.Fatalf("idlehosts.FinalWarningTargets() failed with '%s'", err)
	}
	queueTargets(OP_FINAL_WARNING, targets, &wg, msgsCh)

	targets, err = idlehosts.FirstWarningTargets(dbConn)
	if err != nil {
		logrus.Fatalf("idlehosts.FirstWarningTargets() failed with '%s'", err)
	}
	queueTargets(OP_FIRST_WARNING, targets, &wg, msgsCh)
	wg.Wait()
}

func queueTargets(op string, targets []*idlehosts.Target, wg *sync.WaitGroup, ch chan *wrkMsg) {
	for _, trgt := range targets {
		wg.Add(1)
		msg := &wrkMsg{
			Target: trgt,
			Op:     op,
		}
		logrus.Infof("Queueing host %s (user %d) for %s", trgt.HostSlug, trgt.UserID, op)
		ch <- msg
	}
}

func worker(dbConn *db.Database, wg *sync.WaitGroup, ch chan *wrkMsg) {
	for msg := range ch {
		doWork(dbConn, msg)
		wg.Done()
	}
}

func doWork(dbConn *db.Database, msg *wrkMsg) {
	var err error
	op := msg.Op
	trgt := msg.Target
	logrus.Infof("Starting %s for host %s (user %d)", op, trgt.HostSlug, trgt.UserID)
	switch op {
	case OP_TERMINATION:
		err = doTermination(dbConn, msg.Target)
	case OP_FINAL_WARNING:
		err = doFinalWarning(dbConn, msg.Target)
	case OP_FIRST_WARNING:
		err = doFirstWarning(dbConn, msg.Target)
	}

	if err != nil {
		logrus.Errorf("%s for host %s (user %d), error: %v", op, trgt.HostSlug, trgt.UserID, err)
	}
	logrus.Infof("Finished %s for host %s (user %d)", op, trgt.HostSlug, trgt.UserID)
}

func doTermination(dbConn *db.Database, trgt *idlehosts.Target) error {
	if dryrun {
		logrus.Infof("dry-run active, *would* cancel %s", trgt.HostSlug)
		return nil
	}

	return trgt.Terminate(dbConn)
}

func doFinalWarning(dbConn *db.Database, trgt *idlehosts.Target) error {
	if dryrun {
		logrus.Infof("dry-run active, *would* send final warning to %s", trgt.HostSlug)
		return nil
	}

	return trgt.SendFinalWarning(dbConn)
}

func doFirstWarning(dbConn *db.Database, trgt *idlehosts.Target) error {
	if dryrun {
		logrus.Infof("dry-run active, *would* send first warning to %s", trgt.HostSlug)
		return nil
	}

	return trgt.SendFirstWarning(dbConn)
}
