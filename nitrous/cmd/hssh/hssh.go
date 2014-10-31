package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"code.google.com/p/go.crypto/ssh"
	"github.com/mitchellh/cli"
	"github.com/nitrous-io/gorainbow/cipher"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
)

var (
	maxAttempts = 3
	wait        = 3 * time.Second
	termMode    = "xterm-256color"
	sshUser     = "root"

	aesKey string
	dbURL  string
)

// represent the ssh connection status. When the connection succeeds, then we
// have the client and err should be nil
type conn struct {
	client *ssh.Client
	err    error
}

type resizeRequest struct {
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

func init() {
	// get AES_Key
	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		log.Fatal("AES_KEY is not defined")
	}

	// get db connection
	dbURL = os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		log.Fatal("NITROUS_PG_URL is not defined")
	}
}

func main() {
	c := cli.NewCLI("hssh", "0.0.1-alpha")
	c.Args = os.Args[1:]
	c.Commands = map[string]cli.CommandFactory{
		"shell": newShellCommand(),
	}

	exitCode, _ := c.Run()
	os.Exit(exitCode)
}

func newSession(slug string) (*ssh.Session, error) {
	// get db conection
	dbconn, err := db.NewDatabase(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open db connection %q", err)
	}

	// get host
	h, err := host.FindBySlug(dbconn, slug)
	if err != nil || h == nil {
		return nil, fmt.Errorf("failed to find host %q, error %q", slug, err)
	}

	// get keypair
	kp, err := keypairs.Get(dbconn, h.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find a keypair for host %q, error %q", h, err)
	}

	// get the private key from keypair
	priv, err := cipher.Decrypt(
		[]byte(aesKey),
		[]byte(kp.EncryptedPrivateKeyIv),
		[]byte(kp.EncryptedPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key for host %q, error %q", h, err)
	}

	// create the ssh client
	signer, err := ssh.ParsePrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create a signer for the private key %q", err)
	}

	// generate the ssh config
	sshConfig := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
	}

	// starting to ssh into server
	ch := make(chan conn)
	address := fmt.Sprintf("%s:65100", h.IPAddress.String)
	go func() {
		// trying to connect to ssh here
		attempts := 0 // number of attemps to connect to ssh
		for {
			sshClient, err := ssh.Dial("tcp", address, sshConfig)
			if err == nil {
				ch <- conn{sshClient, nil}
				return
			}
			log.Printf("Waiting for ssh conn to be ready, got %v", err)
			// rejects failed authenticate and connection refused error
			if strings.Contains(err.Error(), "authenticate") ||
				strings.Contains(err.Error(), "connection refused") {
				log.Printf("Failed to authenticate with remote ssh: %v", err)
				attempts += 1
			}
			if attempts < maxAttempts {
				time.Sleep(wait)
				continue
			} else {
				ch <- conn{nil, err}
			}
		}
	}()

	sshConn := <-ch
	if sshConn.err != nil {
		return nil, fmt.Errorf("failed to ssh into the host %q", sshConn.err)
	}
	// open a session
	s, err := sshConn.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create a ssh session %q", err)
	}
	return s, nil
}

// pipes returns stdin, stdout, stderr of the current ssh session
func pipes(s *ssh.Session) (io.WriteCloser, io.Reader, io.Reader) {
	stdin, err := s.StdinPipe()
	if err != nil {
		log.Fatalf("failed to open stdin pipe %v", err)
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		log.Fatalf("failed to open stdout pipe %v", err)
	}
	stderr, err := s.StderrPipe()
	if err != nil {
		log.Fatalf("failed to open stderr pipe %v", err)
	}
	return stdin, stdout, stderr
}

// log and return error code 1
func failf(format string, args ...interface{}) int {
	log.Printf(format, args)
	return 1
}
