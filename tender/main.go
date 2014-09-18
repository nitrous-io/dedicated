package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/nitrous-io/tender"
)

// current version of tender binary
var version string

var (
	databaseURL    string
	baseDomain     string
	freighterdPort int
	pubCACert      string
	pubCAKey       string
	privCACertPath string
)

// Configure require settings. Required env variables are:
// - ADDR: address that tender will listen to
// - DATABASE_URL: url of the postgresql main database
// - BASE_DOMAIN: base domain of the tender app i.e. .nitrousapp.com
// - FREIGHTERD_PORT: public freighted port. The freighterd server itself
// handles authentication so we dont need to handle it here
//
// TODO: eventually we may look into etcd ( and co ) so we dont have to touch
// the main database *because* latency
func main() {
	var (
		flVersion = flag.Bool("v", false, "Display the version and exit")
	)
	flag.Parse()

	if *flVersion {
		fmt.Printf("tender version: %s\n", version)
		os.Exit(0)
	}

	// setup env variables
	databaseURL = envStr("DATABASE_URL")
	freighterdPort = envInt("FREIGHTERD_PORT")
	baseDomain = envStr("BASE_DOMAIN")
	pubCACert = envStr("PUB_CRT")
	pubCAKey = envStr("PUB_KEY")
	privCACertPath = envStr("PRI_CRT")

	if baseDomain[0] != '.' {
		panic("BASE_DOMAIN should start with .")
	}

	_, err := tls.LoadX509KeyPair(pubCACert, pubCAKey)
	if err != nil {
		log.Fatalf("tender: Failed to load SSL certificates: %s", err)
	}
	privCACert, err := ioutil.ReadFile(privCACertPath)
	if err != nil {
		log.Fatalf("tender: Failed to find private certificate: %s", privCACertPath)
	}

	s, err := tender.NewServer(baseDomain, databaseURL, freighterdPort, privCACert)
	if err != nil {
		log.Fatalf("tender: Failed to start tender server, %v", err)
	}

	// proxy freighterd ports
	log.Printf("listen to port %d", freighterdPort)
	err = http.ListenAndServeTLS(fmt.Sprintf(":%d", freighterdPort), pubCACert, pubCAKey, s)
	if err != nil {
		log.Fatalf("tender: exit with error %s", err)
	}
}

func envStr(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing env var " + name)
	}
	return v
}

func envInt(name string) int {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing " + name)
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("Invalid %s. Required integer, got %s", name, v))
	}
	return i
}
