package main

import (
	"crypto/rsa"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"github.com/nitrous-io/nitrous-common/cipher"
	"github.com/nitrous-io/nitrous-common/logger"
	"golang.org/x/crypto/ssh"
)

var usage = `Usage: migrate <source host> [<destination host>]

Prerequisites:
- if this is a user host, they must have been notified prior to the migration
- AES_KEY and NITROUS_PG_URL environment variables need to be set

- if no destination host is provided, a new one will be created for the user

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
	srcSSHHost   *sshutil.Conn
	destSSHHost  *sshutil.Conn

	srcDBHost  *host.Host
	destDBHost *host.Host

	uToken *token.Token

	logr *logger.Logger

	dbConn *db.Database
)

var (
	planMap = map[string]string{
		"1gb": "ebs-small",
		"2gb": "ebs-medium",
		"4gb": "ebs-large",
		// support ec2->ec2
		"ebs-small":  "ebs-small",
		"ebs-medium": "ebs-medium",
		"ebs-large":  "ebs-large",
	}
	regionMap = map[string]string{
		"sfo": "usw2",
		"nyc": "use1",
		"ams": "euw1",
		"sgp": "apse1",
		// support ec2->ec2
		"usw2":  "usw2",
		"use1":  "use1",
		"euw1":  "euw1",
		"apse1": "apse1",
	}
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}

	// get AES_Key
	aesKey = appconfig.AESKey
	if aesKey == "" {
		logr.FatalExit("AES_KEY is not defined")
	}

	// get db connection
	dbURL = appconfig.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(appconfig.DBEnvVar, "is not defined")
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		return
	}

	var err error

	srcHostSlug = os.Args[1]
	var destHostSlug string

	logr.Infof("Connecting to %s", srcHostSlug)
	srcSSHHost, srcDBHost, err = newConn(srcHostSlug)
	if err != nil {
		logr.FatalfExit("Error connecting to %s: %+v\n", srcHostSlug, err)
	}
	logr.Infof("Connected to %s", srcHostSlug)

	var refreshFn func()

	if len(os.Args) >= 3 {
		destHostSlug = os.Args[2]
	} else {
		destHostSlug, refreshFn = provisionDestHost()
	}

	logr.Infof("Connecting to %s", destHostSlug)
	destSSHHost, destDBHost, err = newConn(destHostSlug)
	if err != nil {
		logr.FatalfExit("Error connecting to %s: %+v\n", destHostSlug, err)
	}
	logr.Infof("Connected to %s", destHostSlug)

	fmt.Println("---------")
	fmt.Println("PLAN:")
	fmt.Printf("%s will be renamed as %s (slug & name)\n", destHostSlug, srcHostSlug)
	fmt.Println("Containers, images and volumes will be updated to the host_id")
	fmt.Println("---------")

	verifyCommunication()
	setSSHKeys()
	markServers()
	if refreshFn != nil {
		refreshFn()
	}

	restorable := true

	defer func() {
		if err != nil {
			fmt.Println(err.Error())
			if !restorable {
				fmt.Println("Past restore point, please restore manually")
				fmt.Printf("Please remove %s after verification to avoid charges to Nitrous\n", srcDBHost.ProviderIdentifier.String)
				return
			}

			fmt.Println("Will now restore source server")
			if err := restoreServers(); err != nil {
				fmt.Println(err.Error())
			}
			if refreshFn != nil {
				refreshFn()
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

	restorable = false

	err = removeBackupDestDirectories(dirs)
	if err != nil {
		return
	}

	err = startServices()
	if err != nil {
		return
	}

	err = stopDestination()
	if err != nil {
		return
	}

	if refreshFn != nil {
		refreshFn()
	}

	fmt.Println("Process complete!")
	fmt.Println("NOTICE: The destination host has been swapped with the source slug.")
	fmt.Printf("Please remove %s after verification to avoid charges to Nitrous\n", srcDBHost.ProviderIdentifier.String)
}

func newConn(slug string) (*sshutil.Conn, *host.Host, error) {
	// get db conection
	if dbConn == nil {
		var err error
		dbConn, err = db.NewDatabase(dbURL)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open db connection %q", err)
		}
	}

	// get host
	h, err := host.FindByIdentifier(dbConn, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find host %q, error %q", slug, err)
	}

	// get keypair
	kp, err := keypairs.Get(dbConn, h.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find a keypair for host %v, error %q", h, err)
	}

	// get the private key from keypair
	priv, err := cipher.Decrypt(
		[]byte(aesKey),
		[]byte(kp.EncryptedPrivateKeyIv),
		[]byte(kp.EncryptedPrivateKey))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt private key for host %v, error %q", h, err)
	}

	pk, err := ssh.ParseRawPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key for host %v, error %q", h, err)
	}
	privKey, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("invalid RSA key for host %v (got type %T)", h, pk)
	}

	// create the ssh client
	address := fmt.Sprintf("%s:65100", h.IPAddress.String)
	connInterval := 5 * time.Second

	// set sshUser depends on the provider
	sshUser := "root"
	if h.Provider == "amazon" {
		sshUser = "ubuntu"
	}

	wait, err := sshutil.WaitForSSH(address, sshUser, privKey, connInterval, make(chan struct{}), map[string]interface{}{})

	return wait, h, err
}

func provisionDestHost() (string, func()) {
	uID := srcDBHost.UserID

	u, err := user.FindByID(dbConn, uID)
	if err != nil {
		logr.FatalfExit("Error fetching user %d, err: %v", uID, err)
	}

	uToken = &token.Token{
		UserID: u.ID,
		Type:   token.TypeLogin,
	}

	if err := uToken.Insert(dbConn); err != nil {
		logr.FatalfExit("Error generating token for %d, err: %v", uID, err)
	}

	var currentSlugs struct {
		PlanSlug   string `db:"plan_slug"`
		RegionSlug string `db:"region_slug"`
	}

	err = dbConn.Get(&currentSlugs, `SELECT plans.slug plan_slug, regions.slug region_slug
		FROM hosts
		INNER JOIN plans ON plans.id = hosts.plan_id
		INNER JOIN regions ON regions.id = hosts.region_id
		WHERE hosts.id = $1`, srcDBHost.ID)

	if err != nil {
		logr.FatalfExit("Error fetching slugs from host err: %v", err)
	}

	region, ok := regionMap[currentSlugs.RegionSlug]
	if !ok {
		logr.FatalfExit("Error matching region %s", currentSlugs.RegionSlug)
	}

	plan, ok := planMap[currentSlugs.PlanSlug]
	if !ok {
		logr.FatalfExit("Error matching plan %s", currentSlugs.PlanSlug)
	}

	logr.Infof("Using region '%s'", region)
	logr.Infof("Using plan '%s'", plan)

	name := fmt.Sprintf("destination-%d", srcDBHost.ID)
	logr.Debugf("Creating your host %s", name)
	bearer := fmt.Sprintf(`Authorization: Bearer %s`, uToken.Token)
	params := fmt.Sprintf(`name=%s&region=%s&size=%s`, name, region, plan)
	cmd := exec.Command("curl", "-H", bearer, "--data", params, "https://pro.nitrous.io/api/v0/hosts")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
	cmd.Wait()

	fmt.Println()

	var slug string
	err = dbConn.Get(&slug, `SELECT slug FROM hosts WHERE name = $1 AND state = 'running';`, name)
	if err != nil {
		logr.FatalfExit("Error fetching final host, err: %v", err)
	}

	refreshFn := func() {
		fmt.Println("Refreshing hosts")
		cmd = exec.Command("curl", "-X", "POST", "-H", bearer, fmt.Sprintf("https://pro.nitrous.io/api/v0/hosts/refresh"))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		cmd.Wait()
	}

	return slug, refreshFn
}
