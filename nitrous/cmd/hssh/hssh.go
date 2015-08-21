package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostkeys"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
)

var (
	aesKey     string
	dbURL      string
	dbconn     *db.Database
	awsEnv     string
	sshKeysDir string
	sshTimeout = 15 * time.Second // Timeout for establishing an SSH connection.
)

func init() {
	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		logrus.Fatal("AES_KEY is not defined")
	}

	dbURL = os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		logrus.Fatal("NITROUS_PG_URL is not defined")
	}

	awsEnv = hostkeys.AWSEnv
	if awsEnv == "" {
		logrus.Fatalf("Invalid AWS_ENV %q. Suggested values: staging, production", awsEnv)
	}

	sshKeysDir = hostkeys.SSHKeysDir
	if sshKeysDir != "" {
		if fi, err := os.Stat(sshKeysDir); err != nil {
			if os.IsNotExist(err) {
				logrus.Fatalf("SSH_KEYS_DIR %q does not exist", sshKeysDir)
			}
			if !fi.IsDir() {
				logrus.Fatalf("SSH_KEYS_DIR %q should be a directory", sshKeysDir)
			}
		}
	}

	if d, err := time.ParseDuration(os.Getenv("SSH_TIMEOUT")); err == nil {
		sshTimeout = d
	}
}

func main() {
	var err error
	dbconn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("failed to open db connection %q", err)
	}

	c := cli.NewCLI("hssh", "0.1.0")
	c.Args = os.Args[1:]
	c.Commands = map[string]cli.CommandFactory{
		"info":   newInfoCommand(),
		"shell":  newShellCommand(),
		"upload": newUploadCommand(),
		"run":    newRunCommand(),
	}

	exitCode, _ := c.Run()
	os.Exit(exitCode)
}

func newConn(slug string) (*sshutil.Conn, error) {
	h, err := hostkeys.FromSlug(dbconn, slug)
	if err != nil {
		return nil, err
	}
	if h.State != host.StateRunning {
		logrus.Warnf("Host is not running.  Current state is %s", h.State)
	}

	privKey, err := h.PrivateKey()
	if err != nil {
		return nil, err
	}

	// create the ssh client
	addr := h.Address()
	connInterval := 5 * time.Second

	// set sshUser depending on the provider
	sshUser := "root"
	if h.Provider == "amazon" {
		sshUser = "ubuntu"
	}

	ctx, cancel := context.WithTimeout(context.Background(), sshTimeout)
	defer cancel()

	conn, err := sshutil.WaitForSSH(addr, sshUser, privKey,
		connInterval, ctx.Done(), nil)
	if err != nil {
		if err == sshutil.CancelError {
			err = fmt.Errorf("timed out establishing an SSH connection after %v, err: %v", sshTimeout, err)
		}
		return nil, err
	}

	return conn, nil
}

// pipes returns stdin, stdout, stderr of the current ssh session
func pipes(s *ssh.Session) (io.WriteCloser, io.Reader, io.Reader) {
	stdin, err := s.StdinPipe()
	if err != nil {
		logrus.Fatalf("failed to open stdin pipe %v", err)
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		logrus.Fatalf("failed to open stdout pipe %v", err)
	}
	stderr, err := s.StderrPipe()
	if err != nil {
		logrus.Fatalf("failed to open stderr pipe %v", err)
	}
	return stdin, stdout, stderr
}

// log and return error code 1
func failf(format string, args ...interface{}) int {
	logrus.Errorf(format, args...)
	return 1
}
