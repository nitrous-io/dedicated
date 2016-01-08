package host

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/freighter"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
)

// WaitForSSH establishes a connection to the remote host. This is only
// available in the creation phase of the host.
func WaitForSSH(ctx *HostContext) error {
	l := ctx.Logger
	timeout := ctx.Done()
	config := ctx.Host.SSHConfig()

	if config.PrivateKey == nil {
		return errors.New("host's private key is missing")
	}

	label := l.MetricsLabel("ssh_wait")
	defer mh.UpdateSince(label, time.Now())

	l.Printf("Waiting for SSH to be ready")
	connInterval := 5 * time.Second
	sshConn, err := sshutil.WaitForSSH(config.Address(), config.Username,
		config.PrivateKey, connInterval, timeout, l)
	if err != nil {
		return mh.Error(err, label)
	}
	l.Printf("SSH ready")
	if ctx.Executor != nil {
		ctx.Executor.Close()
	}
	ctx.Executor = sshConn
	return nil
}

// create settings for the host with the provided dynamic properties.
func NewSettings(tls *TLSConfig, apt *AptConfig, home, notifyURL, secret string, publicKeys []string) *Settings {
	return &Settings{
		TLSConfig: tls,
		AptConfig: apt,
		Config: &freighter.Config{
			DataPath:  "/var/lib/freighter",
			TLSVerify: true,
			CA:        TLSCALocation,
			Cert:      TLSCertLocation,
			Key:       TLSKeyLocation,
			Secret:    SecretLocation,
			NotifyURL: notifyURL,
			Listen: []freighter.HostAddress{
				{Proto: "unix", Address: "/var/run/freighterd.sock"},
				{Proto: "tcp", Address: fmt.Sprintf("0.0.0.0:%d", DefaultPort)},
			},
		},
		Home:       home,
		Secret:     secret,
		PublicKeys: publicKeys,
	}
}

// WriteSettings uploads necessary file system contents to the host, which
// include package configs, apt details, etc...
func WriteSettings(ctx *HostContext) error {
	l := ctx.Logger
	conn := ctx.Executor
	settings := ctx.Host.Settings()

	label := l.MetricsLabel("write_settings")
	defer mh.UpdateSince(label, time.Now())
	if conn == nil {
		return mh.Error(errors.New("invalid SSH connection"), label)
	}

	cfgb, err := json.Marshal(settings.Config)
	if err != nil {
		err = fmt.Errorf("invalid config, err: %v", err)
		return mh.Error(err, label)
	}

	l.Printf("Writing settings")
	if err := conn.Execute("sudo mkdir -p /etc/freighterd"); err != nil {
		return mh.Error(err, label)
	}

	// Append to ~/.ssh/authorized_keys file so that any existing keys are
	// preserved.
	authorizedKeysPath := filepath.Join(settings.Home, ".ssh", "authorized_keys")
	cmd := fmt.Sprintf(`echo -e %q >> %s`, strings.Join(settings.PublicKeys, "\n"), authorizedKeysPath)
	if out, err := conn.CombinedOutput(cmd); err != nil {
		err := fmt.Errorf("failed to write to %s, output: %s, err: %v", authorizedKeysPath, out, err)
		return mh.Error(err, label)
	}

	// Write to apt sources file so that we preserve its permissions/ownership
	// and follow symlinks (if it is a symlink).
	buf := bytes.NewBuffer(nil)
	if err := SourceTemplate.Execute(buf, settings.AptConfig); err != nil {
		return mh.Error(err, label)
	}
	cmd = fmt.Sprintf(`echo -e %q | sudo tee %s > /dev/null`, buf.String(), SourceLocation)
	if out, err := conn.CombinedOutput(cmd); err != nil {
		err := fmt.Errorf("failed to write to %s, output: %s, err: %v", SourceLocation, out, err)
		return mh.Error(err, label)
	}

	files, err := uploadFiles(l, conn, map[string]io.Reader{
		TLSCALocation:   strings.NewReader(settings.TLSConfig.CA),
		TLSCertLocation: strings.NewReader(settings.TLSConfig.Cert),
		TLSKeyLocation:  strings.NewReader(settings.TLSConfig.Key),
		SecretLocation:  strings.NewReader(settings.Secret),
		ConfigLocation:  bytes.NewBuffer(cfgb),
	})
	l.Printf("Uploaded files to %s", strings.Join(files, ", "))
	if err != nil {
		return mh.Error(err, label)
	}
	l.Printf("Wrote settings")
	return nil
}

func RestartServices(ctx *HostContext) error {
	l := ctx.Logger
	conn := ctx.Executor
	label := l.MetricsLabel("restart_software")
	defer mh.UpdateSince(label, time.Now())
	if conn == nil {
		return mh.Error(errors.New("invalid SSH connection"), label)
	}

	l.Printf("Restarting freighterd")
	if out, err := conn.CombinedOutput(`sudo service freighterd restart`); err != nil {
		l.Printf("Failed to restart freighterd")
		err = fmt.Errorf("restart freighterd failed with %q, output: %s", err, out)
		return mh.Error(err, label)
	}
	l.Printf("Restarted freighterd")
	return nil
}

func uploadFiles(l Logger, conn Executor, files map[string]io.Reader) ([]string, error) {
	flist := []string{}
	// upload to tmp directory then mv to dest path to avoid permissions issues
	for dest, val := range files {
		tmpDest := filepath.Join("/tmp", hex.EncodeToString([]byte(dest)))
		if err := conn.Upload(tmpDest, val); err != nil {
			return flist, err
		}
		if err := conn.Execute(fmt.Sprintf(
			"sudo mv %s %s", tmpDest, fmt.Sprintf("%q", dest))); err != nil {
			return flist, err
		}
		flist = append(flist, dest)
	}
	return flist, nil
}

func PublishMessage(qp queue.Publisher, ev event.Event, uuid string, queueInfo *QueueInfo, message string, action queue.Event) {
	queueInfo.Message = message
	queueInfo.Host = nil

	payload, err := json.Marshal(queueInfo)
	if err != nil {
		logrus.Errorf("uuid=%s Failed to marshal queueInfo(%+v): %v", uuid, queueInfo, err)
		return
	}

	msg := &queue.Message{
		UUID:         uuid,
		Action:       action.String(),
		Event:        string(ev),
		ResourceName: EventResourceName,
		Payload:      payload,
	}
	err = qp.PublishRetry(msg, 10, 100*time.Millisecond)
	if err != nil {
		logrus.Errorf("uuid=%s Failed to send a message(%+v): %v", uuid, msg, err)
	}
}

func PublishHost(qp queue.Publisher, ev event.Event, uuid string, queueInfo *QueueInfo, info *Info, action queue.Event, message string) {
	queueInfo.Message = message
	queueInfo.Host = info

	payload, err := json.Marshal(queueInfo)
	if err != nil {
		logrus.Errorf("uuid=%s Failed to marshal queueInfo(%+v): %v", uuid, queueInfo, err)
		return
	}

	msg := &queue.Message{
		UUID:         uuid,
		Action:       action.String(),
		Event:        string(ev),
		ResourceName: EventResourceName,
		Payload:      payload,
	}
	err = qp.PublishRetry(msg, -1, 100*time.Millisecond)
	if err != nil {
		logrus.Errorf("uuid=%s Failed to send a message(%+v): %v", uuid, msg, err)
	}
}
