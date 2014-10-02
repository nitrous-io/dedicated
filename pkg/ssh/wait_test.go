package ssh

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	"crypto/rsa"

	"code.google.com/p/go.crypto/ssh"
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

func TestWaitForSSH(t *testing.T) {
	priv, _ := GenerateSSHKey()
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: dummyPubKeyCallback(priv),
	}
	signer, _ := ssh.ParsePrivateKey(GetPem(priv))
	serverConfig.AddHostKey(signer)

	// start the server
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			conn, _ := listener.Accept()
			_, _, reqs, err := ssh.NewServerConn(conn, serverConfig)
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(reqs)
		}
	}()
	_, err := WaitForSSH(listener.Addr().String(), "root", priv,
		10*time.Millisecond, make(chan struct{}))
	if err != nil {
		t.Fatal("failed to wait for ssh connection")
	}
}
