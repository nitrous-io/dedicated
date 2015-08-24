package main

import (
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"crypto/rsa"
	"crypto/x509"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"github.com/nitrous-io/nitrous-common/cipher"
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

type SshHost struct {
	ID                    int64          `db:"id"`
	State                 string         `db:"state"`
	TemplateName          string         `db:"pt_name"`
	RegionIdentifier      sql.NullString `db:"region_identifier"`
	IPAddress             sql.NullString `db:"ip_address"`
	EncryptedPrivateKey   string         `db:"encrypted_private_key"`
	EncryptedPrivateKeyIv string         `db:"encrypted_private_key_iv"`
	Provider              string         `db:"provider"`
}

func init() {
	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		logrus.Fatal("AES_KEY is not defined")
	}

	dbURL = os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		logrus.Fatal("NITROUS_PG_URL is not defined")
	}

	awsEnv = os.Getenv("AWS_ENV")
	if awsEnv == "" {
		logrus.Fatalf("Invalid AWS_ENV %q. Suggested values: staging, production", awsEnv)
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
	if h.State != host.StateRunning {
		logrus.Warnf("Host is not running.  Current state is %s", h.State)
	}

	privKey, err := h.privateKey()
	if err != nil {
		return nil, err
	}

	// create the ssh client
	addr := h.address()
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

func (h *SshHost) privateKey() (*rsa.PrivateKey, error) {
	if h.useLocalKey() {
		privKey, err := h.amazonPrivateKey()
		if err == nil {
			return privKey, nil
		}

		logrus.Infof("failed to use local private key, using key from database, err: %v", err)
	}

	// get the private key from keypair
	priv, err := cipher.Decrypt(
		[]byte(aesKey),
		[]byte(h.EncryptedPrivateKeyIv),
		[]byte(h.EncryptedPrivateKey))
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

func (h *SshHost) amazonPrivateKey() (*rsa.PrivateKey, error) {
	keyPath := path.Join(sshKeysDir, fmt.Sprintf(
		"hostbuilder.%s.%s.pem", h.RegionIdentifier.String, awsEnv))
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

func (h *SshHost) address() string {
	return fmt.Sprintf("%s:65100", h.IPAddress.String)
}

func hostFromSlug(slug string) (*SshHost, error) {
	host := &SshHost{}
	err := dbconn.Get(host,
		`SELECT hosts.id, hosts.region_identifier, hosts.ip_address, hosts.state,
			pt.provider, pt.name AS pt_name,
			key_pairs.encrypted_private_key, key_pairs.encrypted_private_key_iv
		FROM hosts
		JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
		JOIN key_pairs ON key_pairs.host_id = hosts.id
		WHERE hosts.slug = $1
		AND hosts.terminated_at IS NULL;`, slug)
	if err != nil {
		return nil, err
	}
	return host, nil
}

func (h *SshHost) useLocalKey() bool {
	// For the "amazon" provider, the private key can be either:
	// 1. that of an EC2 keypair, or
	// 2. the private key stored in the database.
	// We check if the provider template has 'hotswap' in the name and if
	// we have a private key for it in SSH_KEYS_DIR, we return that instead.
	return h.Provider == "amazon" &&
		strings.Contains(h.TemplateName, "hotswap") &&
		sshKeysDir != ""
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
