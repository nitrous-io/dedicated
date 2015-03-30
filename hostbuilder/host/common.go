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
)

const (
	DefaultPort uint16 = 65333
)

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
	ConfigLocation  = "/etc/freighterd/config.json"

	ErrTimeout = errors.New("operation timedout")
)

// executor represents the connection to the remote host. We can use this to
// perform setup tasks such as uploading files, running commands on the remote
// host. The connection is only available during the host creation phase. The
// host's filesystem should be considered immutable otherwise.
type executor interface {
	// Upload transfers the content of the reader to the path on a remote server
	Upload(path string, input io.Reader) error
	// Execute runs the command on the remote server and returns its error
	Execute(cmd string) error
	// CombinedOutput executes the command on the remote server and returns its
	// combined standard stdout/stderr
	CombinedOutput(cmd string) ([]byte, error)
}

// for setting up repository on the host
type AptConfig struct {
	AptUsername string `json:"apt_username"` // host id on nitrousd
	AptPassword string `json:"apt_password"` // host secret
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

// for setting up TLS certificates on the host
type TLSConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// settings of the remote host. This includes contents that should be uploaded
// to the remote host
type Settings struct {
	*TLSConfig
	*AptConfig
	*config.Config
	Home       string // Home directory of the current user
	Secret     string
	PublicKeys []string
}

// create settings for the host with the provided dynatmic properties.
func NewSettings(tls *TLSConfig, apt *AptConfig, home, notifyURL, secret string, publicKeys []string) *Settings {
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
		PublicKeys: publicKeys,
	}
	return settings
}

// WriteSettings uploads necessary file system contents to the host, which
// include package configs, apt details, etc...
func WriteSettings(l Logger, conn executor, settings *Settings, timeout <-chan struct{}) error {
	label := l.MetricsLabel("write_settings")
	defer mh.UpdateSince(label, time.Now())

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

func uploadFile(l Logger, conn executor, files map[string]io.Reader) ([]string, error) {
	flist := []string{}
	for dest, val := range files {
		if err := conn.Upload(dest, val); err != nil {
			return flist, err
		}
		flist = append(flist, dest)
	}
	return flist, nil
}
