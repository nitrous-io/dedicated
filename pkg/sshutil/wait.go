package sshutil

import (
	"crypto/rsa"
	"errors"
	"log"
	"strings"
	"time"

	"code.google.com/p/go.crypto/ssh"
)

var (
	TimeWait        = 5 * time.Second
	MaxAttempts     = 10
	CancelError     = errors.New("SSH wait cancelled")
	InvalidKeyError = errors.New("Invalid private key")
)

// wait for the ssh to be ready on `address`
func WaitForSSH(
	address string,
	username string,
	privateKey *rsa.PrivateKey,
	wait time.Duration,
	cancel <-chan struct{}) (conn *Conn, err error) {
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

	// trying to connect to ssh here
	attempts := 0 // number of attemps to connect to ssh
	for {
		select {
		case <-cancel:
			return nil, CancelError
		case <-time.After(wait):
		}
		conn, err = NewSSHConn(address, sshConfig)
		if err != nil {
			log.Printf("Waiting for ssh conn to be ready, got %v", err)
			// Only count this as an attempt if we were able to attempt
			// to authenticate. Note this is very brittle since it depends
			// on the string of the error... but I don't see any other way.
			if strings.Contains(err.Error(), "authenticate") {
				log.Printf("Failed to authenticate with remote ssh: %v", err)
				attempts += 1
			}
			if attempts < MaxAttempts {
				continue
			}
			return nil, err
		}
		break
	}
	return conn, nil
}
