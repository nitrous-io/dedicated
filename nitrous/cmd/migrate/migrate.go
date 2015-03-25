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

var usage = `Usage: migrate <source host> <destination host>

Prerequisites:
- if this is a user host, they must have been notified prior to the migration
- create a new host to act as the destination
- AES_KEY and NITROUS_PG_URL environment variables need to be set

After successful migration:
- source host will still be around with terminated_at set
- source host *will* appear in host admin unaccounted hosts
- check dest host to see if data is preserved
- delete source host from provider using host admin page
`

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
		fmt.Println(usage)
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
	markSourceServer()

	defer func() {
		if err != nil {
			fmt.Println(err.Error())
			fmt.Println("Will now restore source server")
			if err := restoreSourceServer(); err != nil {
				fmt.Println(err.Error())
			}
			return
		}
		fmt.Println("Success!")
	}()

	err = stopServices()
	if err != nil {
		return
	}

	dirs := getTransferrableDirectories()
	err = backupDestDirectories(dirs)
	if err != nil {
		return
	}
	err = syncDirectories(dirs)
	if err != nil {
		return
	}

	err = sanityCheck()
	if err != nil {
		return
	}

	err = updateRecords()
	if err != nil {
		return
	}
	err = startServices()
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
