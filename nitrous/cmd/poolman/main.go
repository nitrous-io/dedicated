package main

import (
	"flag"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
)

var (
	appConfig *Config

	ec2conns = map[string]*ec2.EC2{}

	// flags
	flConfPath     string
	flPollInterval time.Duration
)

func init() {
	flag.DurationVar(&flPollInterval, "i", 5*time.Minute, "polling interval. Units are: 's', 'm', 'h'")
	flag.StringVar(&flConfPath, "c", "", "path to config file")
}

func main() {
	flag.Parse()
	if len(flConfPath) == 0 {
		logrus.Fatal("required argument: -c <path to config json>")
	}
	cf, err := os.Open(flConfPath)
	if err != nil {
		logrus.Fatalf("failed to open config file: %v", err)
	}
	defer cf.Close()

	appConfig, err = NewConfig(cf)
	if err != nil {
		logrus.Fatalf("error parsing config: %v", err)
	}
	logrus.Infof("Running with config: %+v\n", appConfig)

	awsConnect()
}
