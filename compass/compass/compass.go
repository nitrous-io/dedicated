package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/compass"
)

// will be injected with the build process
var version string

func main() {
	flVersion := flag.Bool("v", false, "current version")
	flConfigPath := flag.String("c", "config.json", "config path")
	flag.Parse()
	if *flVersion {
		fmt.Printf("Compass: revision %s\n", version)
		os.Exit(0)
		return
	}
	configData, err := ioutil.ReadFile(*flConfigPath)
	if err != nil {
		fatal("Missing config file", *flConfigPath)
	}
	config := new(compass.Config)
	err = json.Unmarshal(configData, &config)
	if err != nil {
		fatal("Invalid config file at ./config.json", err)
	}
	s, err := compass.NewServer(config)
	if err != nil {
		fatal("Failed to start compass server:", err)
	}
	logrus.Println("compass: Name serving since 1978")
	if err := s.ListenAndServe(); err != nil {
		fatal("compass exited with error:", err)
	}
}

func fatal(a ...interface{}) {
	logrus.Fatal(a...)
}
