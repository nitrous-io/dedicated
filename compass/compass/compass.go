package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/compass"
)

// will be injected with the build process
var version string

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	flVersion := flag.Bool("v", false, "current version")
	flConfigPath := flag.String("c", "config.json", "config path")
	flag.Parse()
	if *flVersion {
		fmt.Printf("compass: revision %s\n", version)
		os.Exit(0)
		return
	}
	configData, err := ioutil.ReadFile(*flConfigPath)
	if err != nil {
		fatalf("Missing config file: %s", *flConfigPath)
	}
	config := new(compass.Config)
	err = json.Unmarshal(configData, &config)
	if err != nil {
		fatalf("Invalid config file at ./config.json: %s", err)
	}
	s, err := compass.NewServer(config)
	if err != nil {
		fatalf("Failed to start compass server: %s", err)
	}

	ch := make(chan os.Signal, 10)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

	logrus.Println("compass: Name serving since 1978")
	go s.ListenAndServe()

	sig := <-ch
	signal.Stop(ch)
	logrus.Infof("compass: signal received: %s", sig)
	s.Stop()
	logrus.Info("compass: exited")
}

func fatalf(a ...interface{}) {
	logrus.Fatalf(a...)
}
