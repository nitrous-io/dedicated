package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"crypto/rsa"

	"code.google.com/p/go.crypto/ssh"
	"github.com/mitchellh/cli"
	sshutil "github.com/nitrous-io/dedicated/hostbuilder/pkg/ssh"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous-common/cipher"
)

var (
	sshUser = "root"
	aesKey  string
	dbURL   string
)

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
		"shell":  newShellCommand(),
		"upload": newUploadCommand(),
		"run":    newRunCommand(),
	}

	exitCode, _ := c.Run()
	os.Exit(exitCode)
}

func newConn(slug string) (*sshutil.Conn, error) {
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

	pk, err := ssh.ParseRawPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key for host %q, error %q", h, err)
	}
	privKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid RSA key for host %q", h)
	}

	// create the ssh client
	address := fmt.Sprintf("%s:65100", h.IPAddress.String)
	return sshutil.WaitForSSH(address, sshUser, privKey, 10*time.Second, make(chan struct{}))
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
