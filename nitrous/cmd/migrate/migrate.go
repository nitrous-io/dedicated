package main

import (
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"github.com/nitrous-io/nitrous-common/cipher"
	"github.com/nitrous-io/nitrous-common/logger"
	"golang.org/x/crypto/ssh"
)

var (
	aesKey string
	dbURL  string

	srcHostSlug  string
	destHostSlug string
	srcHost      *sshutil.Conn
	destHost     *sshutil.Conn

	logr *logger.Logger

	dbConn *db.Database
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}

	// get AES_Key
	aesKey = envvars.AESKey
	if aesKey == "" {
		logr.FatalExit("AES_KEY is not defined")
	}

	// get db connection
	dbURL = envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
}

func main() {
	if len(os.Args) < 3 {
		logr.Fatalf("%s SRC_HOST DEST_HOST", os.Args[0])
		return
	}

	var err error

	srcHostSlug = os.Args[1]
	destHostSlug = os.Args[2]

	logr.Infof("Connecting to %s", srcHostSlug)
	srcHost, err = newConn(srcHostSlug)
	if err != nil {
		logr.FatalfExit("Error connecting to %s: %+v\n", srcHostSlug, err)
	}
	logr.Infof("Connected to %s", srcHostSlug)

	logr.Infof("Connecting to %s", destHostSlug)
	destHost, err = newConn(destHostSlug)
	if err != nil {
		logr.FatalfExit("Error connecting to %s: %+v\n", destHostSlug, err)
	}
	logr.Infof("Connected to %s", destHostSlug)

	verifyCommunication()
	setSSHKeys()
	stopServices()

	dirs := getTransferrableDirectories()
	backupDestDirectories(dirs)
	syncDirectories(dirs)

	if !sanityCheck() {
		logr.FatalExit("sanityCheck() failed, exiting.....")
	}

	updateRecords()
	startServices()
}

func newConn(slug string) (*sshutil.Conn, error) {
	// get db conection
	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open db connection %q", err)
	}

	// get host
	h, err := host.FindByIdentifier(dbConn, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to find host %q, error %q", slug, err)
	}

	// get keypair
	kp, err := keypairs.Get(dbConn, h.ID)
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
		return nil, fmt.Errorf("invalid RSA key for host %v (got type %T)", h, pk)
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
		connInterval, make(chan struct{}), map[string]interface{}{})
}
