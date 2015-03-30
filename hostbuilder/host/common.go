package host

import (
	"errors"
	"fmt"
	"time"

	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

var (
	ErrTimeout = errors.New("operation timedout")
)

type executor interface {
	// CombinedOutput executes the command on the remote server and returns its
	// combined standard stdout/stderr
	CombinedOutput(cmd string) ([]byte, error)
}

func RestartServices(l Logger, conn executor, timeout <-chan struct{}) error {
	label := l.MetricsLabel("restart_software")
	defer mh.UpdateSince(label, time.Now())

	finish := make(chan error, 1)
	go func() {
		l.Logf("Restarting freighterd")
		if out, err := conn.CombinedOutput(`sudo service freighterd restart`); err != nil {
			l.Logf("Failed to restart freighterd")
			finish <- fmt.Errorf("restart freighterd failed with %q, output: %s", err, out)
		}
		l.Logf("Restarted freighterd")
		finish <- nil
	}()

	select {
	case <-timeout:
		return ErrTimeout
	case err := <-finish:
		return err
	}
}
