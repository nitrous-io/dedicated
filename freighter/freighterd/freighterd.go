package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/freighter"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/server"
	"github.com/nitrous-io/dedicated/pkg/reexec"

	// load required init for reexec commands
	_ "github.com/nitrous-io/dedicated/freighter/cops"
)

var (
	version     string
	defaultConf string
)

func init() {
	defaultConf = path.Join("etc", "freighterd", "config.json")

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	// return if running in reexec mode, and the reexec func has already finished
	if reexec.Init() {
		return
	}

	// re-export the version so that it can be read elsewhere
	freighter.Version = version

	var (
		flVersion    = flag.Bool("v", false, "Display the version and exit")
		flConfigPath = flag.String("c", defaultConf, "Path to the configuration file. Default to "+defaultConf)
	)
	flag.Parse()

	if *flVersion {
		fmt.Printf("freighterd version %v\n", version)
		os.Exit(0)
	}

	defer func() {
		if r := recover(); r != nil {
			logrus.Fatal(r)
		}
	}()

	conf := &freighter.Config{}
	// parse config file
	confFile, err := os.Open(*flConfigPath)
	if err != nil {
		logrus.Info("Missing config file. Use default config")
		conf = server.DefaultConfig
	} else {
		if err := json.NewDecoder(confFile).Decode(conf); err != nil {
			logrus.Fatal(err)
		}
	}

	// setup data path
	if _, err := os.Stat(conf.DataPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(conf.DataPath, 0755); err != nil {
				logrus.Fatal(err)
			}
		} else {
			logrus.Fatal(err)
		}
	}

	prov, err := docker.NewDockerProvider(conf.DataPath)
	if err != nil {
		logrus.Fatal(err)
	}

	// save current pid
	pid := setPid(conf.DataPath)
	defer removePid(conf.DataPath)
	logrus.Infof("freighterd (rev: %v, pid: %v) started", version, pid)

	// start the API server
	svr := server.NewServer(prov, conf)
	svr.Start()

	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	s := <-ch
	logrus.Infof("freighterd (rev: %v, pid: %v) got signal '%v'. Terminating.", version, pid, s)
	svr.Stop()
	logrus.Infof("freighterd (rev: %v, pid: %v) terminated.", version, pid)
}

func setPid(dataPath string) int {
	file, err := os.Create(path.Join(dataPath, "freighter.pid"))
	if err != nil {
		logrus.Fatal(err)
	}

	pid := os.Getpid()
	if _, err := file.WriteString(strconv.Itoa(pid)); err != nil {
		logrus.Fatal(err)
	}

	if err := file.Close(); err != nil {
		logrus.Fatal(err)
	}
	return pid
}

func removePid(dataPath string) {
	if err := os.RemoveAll(path.Join(dataPath, "freighter.pid")); err != nil {
		logrus.Warnf("freighterd: failed to remove pid file %q", err)
	}
}
