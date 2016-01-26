// The hostshutdown command stops free hosts that have been inactive for X
// minutes.
//
// This is mostly to control our hosting costs by reducing the amount of EC2
// compute time when users are not actually using their hosts.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/pkg/uuid"
)

const nWorkers = 4

var (
	checkInterval time.Duration = 5 * time.Second
	initiatorID   int64
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	logrus.Infof("Starting host shutdown service with %d workers", nWorkers)
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatalf("%v is not defined", envvars.DBEnvVar)
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("failed to connect to database, err: %v", err)
	}

	if envvars.WebHostname == "" {
		logrus.Fatalf("WEB_HOSTNAME is not defined")
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

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				users, err := user.FindAllWithFreeHost(dbConn)
				if err != nil {
					logrus.Fatalf("failed to lookup users with free hosts, err: %v", err)
				}

				for _, u := range users {
					if u.HasUsageRemaining(dbConn) {
						continue
					}

					hosts, err := host.FindAllByUser(dbConn, u.ID)
					if err != nil {
						logrus.Fatalf("failed to lookup user's hosts, err: %v", err)
					}

					for _, h := range hosts {
						// Ignore hosts that are not running or paid.
						if h.State != host.StateRunning || !h.Free {
							continue
						}

						logrus.Infof("Shutting down host %d whose free usage has been depleted", h.ID)
						wg.Add(1)
						hostsCh <- h
					}
				}
				// Wait for all of the free hosts whose free usage have been
				// depleted to shutdown.
				wg.Wait()

				hosts, err := host.FindAllIdleForAutoShutdown(dbConn)
				if err != nil {
					logrus.Fatalf("failed to lookup idle and free hosts, err: %v", err)
				}

				for _, h := range hosts {
					logrus.Infof("Shutting down idle host %d", h.ID)
					wg.Add(1)
					hostsCh <- h
				}
				wg.Wait()
			case <-stop:
				close(stopped)
				return
			}
		}

		close(stopped)
	}()

	sig := <-ch
	signal.Stop(ch)
	logrus.Infof("signal received: %v", sig)
	close(stop)

	select {
	case <-stopped: // Wait for goroutine to exit cleanly.
		logrus.Info("Host shutdown service exiting.")
	case <-time.After(time.Minute):
		logrus.Error("Timed out waiting for host shutdown service to clean up, exiting anyway.")
	}
}

func worker(dbConn *db.Database, wg *sync.WaitGroup, ch chan *host.Host) {
	for h := range ch {
		logrus.Infof("worker: received host %d", h.ID)
		if err := stopHost(dbConn, h); err != nil {
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

	// Confirm that host is still eligible for shutdown:
	// 1. user might have used the host since the time the query was ran, or
	// 2. user might have had their free usage quota replenished.
	// We don't want to incorrectly shutdown the host if either of these occur.
	if err := canShutdown(dbConn, hst); err != nil {
		return fmt.Errorf("host %d is no longer eligible for shutdown, err: %v", hst.ID, err)
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

func canShutdown(dbConn *db.Database, hst *host.Host) error {
	idle, err := hst.IsIdle(dbConn)
	if err != nil {
		return err
	}
	if idle {
		return nil
	}

	if !hst.Free {
		return errors.New("host is neither free nor idle")
	}

	u, err := user.FindByID(dbConn, hst.UserID)
	if err != nil {
		return err
	}
	if !u.HasUsageRemaining(dbConn) {
		return nil
	}

	return errors.New("non-idle host's user still has free usage remaining")
}
