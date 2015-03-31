package sshutil

import (
	"bytes"
	"errors"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"crypto/rsa"

	"golang.org/x/crypto/ssh"
)

type pubKeyCallback func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error)

func dummyPubKeyCallback(priv *rsa.PrivateKey) pubKeyCallback {
	pub, _ := GetPubKey(priv)
	return func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(pub, ssh.MarshalAuthorizedKey(key)) {
			return nil, nil
		}
		return nil, errors.New("invalid public key")
	}
}

func dummyServerConfig(key *rsa.PrivateKey) *ssh.ServerConfig {
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: dummyPubKeyCallback(key),
	}
	signer, _ := ssh.ParsePrivateKey(GetPem(key))
	serverConfig.AddHostKey(signer)
	return serverConfig
}

func TestWaitForSSH(t *testing.T) {
	priv, _ := GenerateSSHKey()
	serverConfig := dummyServerConfig(priv)
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			conn, _ := listener.Accept()
			sconn, _, reqs, err := ssh.NewServerConn(conn, serverConfig)
			if err != nil {
				continue
			}
			defer sconn.Close()
			go ssh.DiscardRequests(reqs)
		}
	}()
	_, err := WaitForSSH(listener.Addr().String(), "root", priv,
		10*time.Millisecond, make(chan struct{}), nil)
	if err != nil {
		t.Fatal("failed to wait for SSH connection")
	}
}

func TestWaitForSSHCancel(t *testing.T) {
	priv, _ := GenerateSSHKey()
	serverConfig := dummyServerConfig(priv)
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	var wg sync.WaitGroup
	go func() {
		for {
			wg.Add(1)
			conn, _ := listener.Accept()
			wg.Done()
			sconn, _, reqs, err := ssh.NewServerConn(conn, serverConfig)
			if err != nil {
				continue
			}
			defer sconn.Close()
			go ssh.DiscardRequests(reqs)
		}
	}()

	cancel := make(chan struct{}, 1)
	errCh := make(chan error)
	clientKey, _ := GenerateSSHKey() // Use a different key so that conn fails.
	connInterval := 10 * time.Millisecond
	orig := MaxAttempts
	MaxAttempts = int(math.MaxInt64) // Set MaxAttempts to a huge number so we never stop because of it.
	defer func(n int) {
		MaxAttempts = n
	}(orig)
	go func() {
		_, err := WaitForSSH(listener.Addr().String(), "root", clientKey,
			connInterval, cancel, nil)
		errCh <- err
	}()

	wg.Wait()                      // Wait for at least 1 connection.
	<-time.After(3 * connInterval) // Allow enough time to pass so that we actually attempt to connect.

	close(cancel)
	select {
	case err := <-errCh:
		if err != CancelError {
			t.Fatalf("failed to cancel WaitForSSH, got err: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WaitForSSH to return")
	}
}
