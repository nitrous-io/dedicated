package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

const Usage = `Usage: hostrunner [workingdir]
  [workingdir] is the directory that stores artifacts (private key files) and defaults to a directory in /tmp
`

var (
	dbConn *db.Database

	aesKey  string
	workDir string
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		logrus.Fatal("AES_KEY is not defined")
	}

	envvars.CheckDB()
	dbURL := envvars.NitrousPgUrl

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatal(err)
	}
}

func writeInventory(hosts []*AnsibleHost) error {
	fpath := path.Join(workDir, "inventory")
	logrus.Debug("Writing inventory file to", fpath)
	f, err := os.Create(fpath)
	if err != nil {
		return fmt.Errorf("Error creating inventory file: %v", err)
	}
	defer f.Close()
	if err := render(f, hosts); err != nil {
		return fmt.Errorf("Error rendering inventory file: %v", err)
	}
	return nil
}

func main() {
	switch len(os.Args) {
	case 1:
		var err error
		workDir, err = ioutil.TempDir("", "hostrunner")
		if err != nil {
			logrus.Fatalf("Error creating work directory: %v", err)
		}
		// no-op
	case 2:
		workDir = os.Args[1]
		if err := os.MkdirAll(workDir, 0700); err != nil {
			logrus.Fatalf("Could not use directory %s: %v", workDir, err)
		}
	default:
		fmt.Println(Usage)
		logrus.Fatal("Wrong number of arguments")
	}

	logrus.Debugf("Using work directory %s", workDir)

	hosts, err := inventoryHosts()
	if err != nil {
		logrus.Fatalf("Error retrieving hosts: %v", err)
	}
	for _, h := range hosts {
		if err := h.writePrivateKey(); err != nil {
			logrus.Fatalf("Error writing private key for host %d %s: %v", h.ID, h.Slug, err)
		}
	}

	if err := writeInventory(hosts); err != nil {
		logrus.Fatal(err)
	}
}
