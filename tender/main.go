package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/nitrous-io/tender"
)

var (
	databaseURL string
	baseDomain  string
	authedPort  int
	pubCACert   string
	pubCAKey    string
)

// Configure require settings. Required env variables are:
// - ADDR: address that tender will listen to
// - DATABASE_URL: url of the postgresql main database
// - BASE_DOMAIN: base domain of the tender app i.e. .nitrousapp.com
// - AUTHED_PORT: port that requires authentication to proxy i.e public freighterd port
//
// TODO: eventually we may look into etcd ( and co ) so we dont have to touch
// the main database / calling HTTP API *because* latency
func init() {
	databaseURL = envStr("DATABASE_URL")
	authedPort = envInt("AUTHED_PORT")
	baseDomain = envStr("BASE_DOMAIN")
	pubCACert = envStr("PUB_CRT")
	pubCAKey = envStr("PUB_KEY")

	if baseDomain[0] != '.' {
		panic("BASE_DOMAIN should start with .")
	}
}

func main() {
	_, err := tls.LoadX509KeyPair(pubCACert, pubCAKey)
	if err != nil {
		log.Fatalf("tender: Failed to load SSL certificates- %s & %s: %s", pubCACert, pubCAKey, err)
	}

	s, err := tender.NewServer(baseDomain, databaseURL, authedPort)
	if err != nil {
		log.Fatalf("tender: Failed to start tender server, %v", err)
	}

	// proxy freighterd ports
	log.Println("listen to port 65333")
	err = http.ListenAndServeTLS(":65333", pubCACert, pubCAKey, s)
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
