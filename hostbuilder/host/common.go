package host

import (
	"errors"
	"fmt"
	"strings"
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
			err = fmt.Errorf("restart freighterd failed with %q, output: %s", err, out)
			finish <- mh.Error(err, label)
			return
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

// UpdateSoftware updates the package list and necessary packages that may be
// outdated since the last image builds.
func UpdateSoftware(l Logger, conn executor, version string, timeout <-chan struct{}) error {
	label := l.MetricsLabel("update_software")
	defer mh.UpdateSince(label, time.Now())

	// TODO move all of cmdParts into a shell script that is written and run
	// (similar to /tmp/already-installed.sh below)
	path := "/tmp/already-installed.sh"
	cmdParts := []string{
		fmt.Sprintf(`{ cat > %s <<- 'EOF'
			#!/bin/sh
			installed_version=$(/usr/sbin/freighterd -v)
			installed_version=${installed_version#'freighterd version '}
			test $installed_version = '%s'
		EOF
		} && chmod +x %s && %s`, path, version, path, path),
		`||`,
		// apt-get update with single source
		// https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=714877#12
		"DEBIAN_FRONTEND=noninteractive sudo apt-get update",
		"-o APT::List-Cleanup=0",
		`-o Dir::Etc::sourceparts="/dev/null"`,
		`-o Dir::Etc::sourcelist="sources.list.d/nitrousio.list"`,
		`&&`,
		"DEBIAN_FRONTEND=noninteractive sudo apt-get install -y --force-yes",
		`-o Dpkg::Options::="--force-confdef"`, // avoid prompt
		`-o Dpkg::Options::="--force-confold"`, // do not modify existing config file
	}
	pkgspec := "freighterd"
	if version != "" {
		pkgspec = fmt.Sprintf("freighterd=%s", version)
	}
	cmdParts = append(cmdParts, pkgspec)
	cmd := strings.Join(cmdParts, " ")

	finish := make(chan error, 1)

	go func() {
		l.Logf("Updating freighterd package")
		if out, err := conn.CombinedOutput(cmd); err != nil {
			err = fmt.Errorf("update failed with %q, output: %s", err, out)
			l.Logf("Failed to update freighterd %s", err)
			if version != "" {
				finish <- mh.Error(err, label)
				return
			}
		}
		// failure to update freighterd when a version is not specified is considered continuable
		l.Logf("Updated freighterd package")
		finish <- nil
	}()

	select {
	case <-timeout:
		return ErrTimeout
	case err := <-finish:
		return err
	}
}
