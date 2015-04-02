package sshutil

import (
	"crypto/rsa"
	"errors"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	MaxAttempts     = 10
	CancelError     = errors.New("SSH wait cancelled")
	InvalidKeyError = errors.New("Invalid private key")
)

type res struct {
	conn *Conn
	err  error
}

type Logger interface {
	Printf(format string, v ...interface{})
}

type logger struct{}

func (l *logger) Printf(format string, v ...interface{}) { log.Printf(format, v...) }

// wait for the ssh to be ready on `address`
func WaitForSSH(
	address string,
	username string,
	privateKey *rsa.PrivateKey,
	connInterval time.Duration,
	cancel <-chan struct{},
	l Logger) (conn *Conn, err error) {

	// if we dont have a logger, delegates to normal log.Printf
	if l == nil {
		l = &logger{}
	}

	// generate the signer from private key
	signer, err := ssh.ParsePrivateKey(GetPem(privateKey))
	if err != nil {
		return nil, InvalidKeyError
	}

	// generate the ssh config
	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
	}

	attempts := 0 // number of attempts to connect to ssh
	ticker := time.NewTicker(connInterval)
	defer ticker.Stop()
	for {
		select {
		case <-cancel:
			return nil, CancelError
		case <-ticker.C:
			conn, err := NewSSHConn(address, sshConfig)
			if err == nil {
				return conn, nil
			}

			l.Printf("Waiting for ssh conn to be ready, got %v", err)
			// Only count this as an attempt if we were able to attempt
			// to authenticate. Note this is very brittle since it depends
			// on the string of the error... but I don't see any other way.
			if strings.Contains(err.Error(), "authenticate") {
				l.Printf("%sFailed to authenticate with remote ssh: %v", err)
				attempts += 1
			}
			if attempts >= MaxAttempts {
				return nil, err
			}
		}
	}
}
