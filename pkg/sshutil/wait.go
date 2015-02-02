package sshutil

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
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
	cancel <-chan struct{},
	ctx map[string]interface{}) (conn *Conn, err error) {
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

	baseString := ""
	for k, v := range ctx {
		baseString += fmt.Sprintf("%s=%v ", k, v)
	}

	// trying to connect to ssh here
	attempts := 0 // number of attempts to connect to ssh
	ticker := time.NewTicker(wait)
	defer ticker.Stop()
	for {
		select {
		case <-cancel:
			return nil, CancelError
		case <-ticker.C:
			conn, err = NewSSHConn(address, sshConfig)
			if err == nil {
				return conn, nil
			}

			log.Printf("%sWaiting for ssh conn to be ready, got %v", baseString, err)
			// Only count this as an attempt if we were able to attempt
			// to authenticate. Note this is very brittle since it depends
			// on the string of the error... but I don't see any other way.
			if strings.Contains(err.Error(), "authenticate") {
				log.Printf("%sFailed to authenticate with remote ssh: %v", baseString, err)
				attempts += 1
			}
			if attempts >= MaxAttempts {
				return nil, err
			}
		}
	}
}
