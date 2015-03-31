package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"crypto/rsa"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"github.com/nitrous-io/nitrous-common/cipher"
	"golang.org/x/crypto/ssh"
)

var (
	aesKey string
	dbURL  string
)

type resizeRequest struct {
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

func init() {
	// get AES_Key
	aesKey = envvars.AESKey
	if aesKey == "" {
		log.Fatal("AES_KEY is not defined")
	}

	// get db connection
	dbURL = envvars.NitrousPgUrl
	if dbURL == "" {
		log.Fatal(envvars.DBEnvVar, "is not defined")
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
	h, err := host.FindByIdentifier(dbconn, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to find host %q, error %q", slug, err)
	}

	// get keypair
	kp, err := keypairs.Get(dbconn, h.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find a keypair for host %v, error %q", h, err)
	}

	// get the private key from keypair
	priv, err := cipher.Decrypt(
		[]byte(aesKey),
		[]byte(kp.EncryptedPrivateKeyIv),
		[]byte(kp.EncryptedPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key for host %v, error %q", h, err)
	}

	pk, err := ssh.ParseRawPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key for host %v, error %q", h, err)
	}
	privKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid RSA key for host %v", h)
	}

	// create the ssh client
	address := fmt.Sprintf("%s:65100", h.IPAddress.String)
	connInterval := 5 * time.Second

	// set sshUser depends on the provider
	sshUser := "root"
	if h.Provider == "amazon" {
		sshUser = "ubuntu"
	}

	return sshutil.WaitForSSH(address, sshUser, privKey,
		connInterval, make(chan struct{}), nil)
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
	log.Printf(format, args...)
	return 1
}
