package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/nitrous-io/tender"
)

var (
	addr        string
	databaseURL string
	baseDomain  string
	authedPort  int
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
	addr = "0.0.0.0:" + envStr("PORT")
	databaseURL = envStr("DATABASE_URL")
	authedPort = envInt("AUTHED_PORT")
	baseDomain = envStr("BASE_DOMAIN")
	if baseDomain[0] != '.' {
		panic("BASE_DOMAIN should start with .")
	}
}

func main() {
	s, err := tender.NewServer(baseDomain, databaseURL, authedPort)
	if err != nil {
		fmt.Printf("Failed to start tender server %v", err)
		os.Exit(1)
	}
	log.Printf("Tender starting at %s", addr)
	log.Fatal(http.ListenAndServe(addr, s))
}

func envStr(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing " + name)
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
