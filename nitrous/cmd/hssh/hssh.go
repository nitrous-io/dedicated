package main

import (
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"crypto/rsa"
	"crypto/x509"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"github.com/nitrous-io/nitrous-common/cipher"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
)

var (
	aesKey       string
	dbURL        string
	dbconn       *db.Database
	awsAccessKey string
	awsSecretKey string
	sshKeysDir   string
	sshTimeout   = 15 * time.Second // Timeout for establishing an SSH connection.
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

	awsAccessKey = os.Getenv("AWS_ACCESS_KEY")
	if awsAccessKey == "" {
		logrus.Fatal("AWS_ACCESS_KEY is not defined")
	}
	awsSecretKey = os.Getenv("AWS_SECRET_KEY")
	if awsSecretKey == "" {
		logrus.Fatal("AWS_SECRET_KEY is not defined")
	}

	sshKeysDir = os.Getenv("SSH_KEYS_DIR")
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
	h, err := hostFromSlug(slug)
	if err != nil {
		return nil, err
	}

	privKey, err := privateKey(h)
	if err != nil {
		return nil, err
	}

	// create the ssh client
	addr := address(h)
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

func privateKey(h *host.Host) (*rsa.PrivateKey, error) {
	// For the "amazon" provider, the private key can be either:
	// 1. that of an EC2 keypair, or
	// 2. the private key stored in the database.
	// We first check the keypair name of the underlying EC2 instance and if
	// we have a private key for it in SSH_KEYS_DIR, we return that instead.
	if h.Provider == "amazon" && sshKeysDir != "" {
		privKey, err := amazonPrivateKey(h)
		if err == nil {
			return privKey, nil
		}

		logrus.Infof("failed to use local private key, using key from database, err: %v", err)
	}

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

	return privKey, nil
}

func amazonPrivateKey(h *host.Host) (*rsa.PrivateKey, error) {
	auth, err := aws.GetAuth(awsAccessKey, awsSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to auth with AWS API, err: %v", err)
	}

	region, ok := aws.Regions[h.RegionIdentifier.String]
	if !ok {
		return nil, fmt.Errorf("invalid AWS region %q for host", h.RegionIdentifier.String)
	}

	client := ec2.New(auth, region)

	instanceID := h.ProviderIdentifier.String
	resp, err := client.Instances([]string{instanceID}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance info for %q, err: %v", instanceID, err)
	}

	if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}

	instance := resp.Reservations[0].Instances[0]

	logrus.Infof("Underlying EC2 instance: %+v", instance)

	keyPath := path.Join(sshKeysDir, fmt.Sprintf("%s.pem", instance.KeyName))
	if _, err := os.Stat(keyPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("private key does not exist locally at %q", keyPath)
		}
		return nil, fmt.Errorf("error stat-ing key at %q, err: %v", keyPath, err)
	}

	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading private key at %q, err: %v", keyPath, err)
	}
	block, _ := pem.Decode(data)
	pk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse a valid RSA private key from %q, err: %v", keyPath, err)
	}

	logrus.Infof("using private key at %v", keyPath)

	return pk, nil
}

func address(h *host.Host) string {
	return fmt.Sprintf("%s:65100", h.IPAddress.String)
}

func hostFromSlug(slug string) (*host.Host, error) {
	return host.FindByIdentifier(dbconn, slug)
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
