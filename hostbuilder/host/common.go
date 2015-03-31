package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/freighter/config"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
)

// WaitForSSH establishes a connection to the remote host. This is only
// available in the creation phase of the host.
func WaitForSSH(ctx *HostContext) error {
	l := ctx.Logger
	timeout := ctx.Done()
	config := ctx.Host.SSHConfig()

	label := l.MetricsLabel("ssh_wait")
	defer mh.UpdateSince(label, time.Now())

	l.Logf("Waiting for SSH to be ready")
	connInterval := 5 * time.Second
	sshConn, err := sshutil.WaitForSSH(config.Address(), config.Username,
		config.PrivateKey, connInterval, timeout, l)
	if err != nil {
		return mh.Error(err, label)
	}
	l.Logf("SSH ready")
	ctx.Executor = sshConn
	return nil
}

// create settings for the host with the provided dynamic properties.
func NewSettings(tls *TLSConfig, apt *AptConfig, home, notifyURL, secret, version string, publicKeys []string) *Settings {
	settings := &Settings{
		TLSConfig: tls,
		AptConfig: apt,
		Config: &config.Config{
			DataPath:  "/var/lib/freighter",
			TLSVerify: true,
			CA:        TLSCALocation,
			Cert:      TLSCertLocation,
			Key:       TLSKeyLocation,
			Secret:    SecretLocation,
			NotifyURL: notifyURL,
			Listen: []config.HostAddress{
				{Proto: "unix", Address: "/var/run/freighterd.sock"},
				{Proto: "tcp", Address: fmt.Sprintf("0.0.0.0:%d", DefaultPort)},
			},
		},
		Home:       home,
		Secret:     secret,
		Version:    version,
		PublicKeys: publicKeys,
	}
	return settings
}

// WriteSettings uploads necessary file system contents to the host, which
// include package configs, apt details, etc...
func WriteSettings(ctx *HostContext) error {
	l := ctx.Logger
	conn := ctx.Executor
	timeout := ctx.Done()
	settings := ctx.Host.Settings()

	label := l.MetricsLabel("write_settings")
	defer mh.UpdateSince(label, time.Now())
	if conn == nil {
		return mh.Error(errors.New("invalid SSH connection"), label)
	}

	pubKeysBuf := bytes.NewBuffer(nil)
	for _, k := range settings.PublicKeys {
		fmt.Fprintln(pubKeysBuf, k)
	}

	aptSourcesBuf := bytes.NewBuffer(nil)
	if err := SourceTemplate.Execute(aptSourcesBuf, settings.AptConfig); err != nil {
		return mh.Error(err, label)
	}

	cfgb, err := json.Marshal(settings.Config)
	if err != nil {
		err = fmt.Errorf("invalid config, err: %v", err)
		return mh.Error(err, label)
	}

	finish := make(chan error, 1)
	go func() {
		l.Logf("Writing settings")
		if err := conn.Execute("sudo mkdir -p /etc/freighterd"); err != nil {
			finish <- mh.Error(err, label)
			return
		}

		pubKeyLocation := filepath.Join(settings.Home, ".ssh", "authorized_keys")
		fl, err := uploadFile(l, conn, map[string]io.Reader{
			pubKeyLocation:  pubKeysBuf,
			SourceLocation:  aptSourcesBuf,
			TLSCALocation:   strings.NewReader(settings.TLSConfig.CA),
			TLSCertLocation: strings.NewReader(settings.TLSConfig.Cert),
			TLSKeyLocation:  strings.NewReader(settings.TLSConfig.Key),
			SecretLocation:  strings.NewReader(settings.Secret),
			ConfigLocation:  bytes.NewBuffer(cfgb),
		})
		l.Logf("Uploaded (%s) to %v", strings.Join(fl, ", "))
		if err != nil {
			finish <- mh.Error(err, label)
			return
		}
		l.Logf("Wrote settings")
		finish <- nil
	}()

	select {
	case <-timeout:
		return ErrTimeout
	case err := <-finish:
		return err
	}
}

func RestartServices(ctx *HostContext) error {
	l := ctx.Logger
	conn := ctx.Executor
	timeout := ctx.Done()
	label := l.MetricsLabel("restart_software")
	defer mh.UpdateSince(label, time.Now())
	if conn == nil {
		return mh.Error(errors.New("invalid SSH connection"), label)
	}

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
func UpdateSoftware(ctx *HostContext) error {
	l := ctx.Logger
	conn := ctx.Executor
	timeout := ctx.Done()
	version := ctx.Host.Settings().Version
	label := l.MetricsLabel("update_software")
	defer mh.UpdateSince(label, time.Now())
	if conn == nil {
		return mh.Error(errors.New("invalid SSH connection"), label)
	}

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

func uploadFile(l Logger, conn Executor, files map[string]io.Reader) ([]string, error) {
	flist := []string{}
	for dest, val := range files {
		if err := conn.Upload(dest, val); err != nil {
			return flist, err
		}
		flist = append(flist, dest)
	}
	return flist, nil
}
