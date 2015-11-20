package main

import (
	"flag"
	"math"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
)

/*
* Simple pool manager that respects configured minimums
* Rough overview:
* 1. get latest AMI from hostbuilder for each region
* 2. subnet and security group defined in config
* 3. call hostbuilder GET /pools endpoint to get pool counts
* 4. call hostbuilder POST /pools/grow endpoint if counts are less than minimum

* Known limitations:
* - if polling interval is too short, can potentially create more than the
* minimum (since the count will not be updated while growing is in progress)
* - no way to reload config means that if AMI tags are updated in ec2, need to
* restart poolman to retrieve the latest AMI
 */

var (
	appConfig *Config

	ec2conns = map[string]*ec2.EC2{}
	hbClient *HBClient

	// flags
	flConfPath     string
	flPollInterval time.Duration
	// this number is the maximum number of grows sent in a single polling
	// period, across all instance types and regions
	flMaxGrowsPerPoll int
)

func init() {
	flag.StringVar(&flConfPath, "c", "", "path to config file")
	flag.DurationVar(&flPollInterval, "i", 5*time.Minute, "polling interval. Units are: 's', 'm', 'h'")
	flag.IntVar(&flMaxGrowsPerPoll, "g", 8, "number of times we can ask hostbuilder to grow pool per poll interval")

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func poll() {
	for {
		grown := 0 // number of hostbuilder grows in this polling interval

		s, err := hbClient.List()
		if err != nil {
			logrus.Fatalf("error getting /pools: %v", err)
		}
		counts, err := parseList(s)
		if err != nil {
			logrus.Fatalf("error parsing /pools: %v", err)
		}
		for _, r := range appConfig.Regions {
			f := logrus.Fields{
				"region":   r.ID,
				"config":   r.InstancePool,
				"existing": counts[r.ID],
			}
			logrus.WithFields(f).Debugf("evaluating region")
			// FIXME if pollInterval is below the time it takes to grow
			// (create) an instance, this will cause more calls to the grow
			// endpoint than intended (since the count won't update)
			for _, poolConf := range r.InstancePool {
				existing, ok := counts[r.ID][poolConf.Type]
				if !ok {
					existing = 0
				}
				if existing >= poolConf.Min {
					logrus.WithFields(f).Debugf("available instances above minimum for type %s", poolConf.Type)
					continue
				}
				if grown >= flMaxGrowsPerPoll {
					logrus.WithFields(f).Infof("max grows for this period reached: %d", flMaxGrowsPerPoll)
					continue
				}
				numDesired := poolConf.Min - existing
				numToGrow := int(math.Min(float64(flMaxGrowsPerPoll-grown), float64(numDesired)))
				for i := 0; i < numToGrow; i++ {
					grown++
					logrus.WithFields(f).Infof("growing type %s (%d/%d)", poolConf.Type, grown, flMaxGrowsPerPoll)
					go growPool(r.ID, poolConf.Type)
				}
			}
		}

		time.Sleep(flPollInterval)
	}
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

	awsConnect()
	for i, reg := range appConfig.Regions {
		latest, err := latestAMI(reg.ID)
		if err != nil {
			logrus.Fatalf("error listing AMIs for %s: %v", reg.ID, err)
		}
		appConfig.Regions[i].SourceAMI = latest
	}

	logrus.WithFields(logrus.Fields{
		"config":             appConfig,
		"polling_interval":   flPollInterval,
		"max_grows_per_poll": flMaxGrowsPerPoll,
	}).Info("using config")

	hbClient = NewHBClient(appConfig.HostbuilderURL)
	poll()
}
