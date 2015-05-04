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

	"github.com/nitrous-io/dedicated/freighter"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/server"
	"github.com/nitrous-io/dedicated/freighter/util"
	"github.com/nitrous-io/dedicated/pkg/airbrake"
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
}

func main() {
	flVersion := flag.Bool("v", false, "Display the version and exit")
	flConfigPath := flag.String("c", defaultConf, "Path to the configuration file. Default to "+defaultConf)
	flag.Parse()

	if *flVersion {
		fmt.Printf("freighterd version %v\n", version)
		os.Exit(0)
	}

	logger := &util.Logger{Level: util.LogLevelDebug}

	conf := &freighter.Config{}
	// parse config file
	confFile, err := os.Open(*flConfigPath)
	if err != nil {
		logger.Info("Missing config file. Use default config")
		conf = server.DefaultConfig
	} else {
		if err := json.NewDecoder(confFile).Decode(conf); err != nil {
			logger.FatalExit(err)
		}
	}

	airbrake.Init(109646, conf.AirbrakeKey, "production")

	// return if running in reexec mode, and the reexec func has already finished
	if reexec.Init() {
		return
	}

	// re-export the version so that it can be read elsewhere
	freighter.Version = version

	defer func() {
		if r := recover(); r != nil {
			logger.FatalExit(r)
		}
	}()

	// setup data path
	if _, err := os.Stat(conf.DataPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(conf.DataPath, 0755); err != nil {
				logger.FatalExit(err)
			}
		} else {
			logger.FatalExit(err)
		}
	}

	prov, err := docker.NewDockerProvider(logger, conf.DataPath)
	if err != nil {
		logger.FatalExit(err)
	}

	// save current pid
	pid := setPid(logger, conf.DataPath)
	defer removePid(logger, conf.DataPath)
	logger.Infof("freighterd (rev: %v, pid: %v) started", version, pid)

	// start the API server
	svr := server.NewServer(logger, prov, conf)
	svr.Start()

	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	s := <-ch
	logger.Infof("freighterd (rev: %v, pid: %v) got signal '%v'. Terminating.", version, pid, s)
	svr.Stop()
	logger.Infof("freighterd (rev: %v, pid: %v) terminated.", version, pid)
}

func setPid(logger *util.Logger, dataPath string) int {
	file, err := os.Create(path.Join(dataPath, "freighter.pid"))
	if err != nil {
		logger.FatalExit(err)
	}

	pid := os.Getpid()
	if _, err := file.WriteString(strconv.Itoa(pid)); err != nil {
		logger.FatalExit(err)
	}

	if err := file.Close(); err != nil {
		logger.FatalExit(err)
	}
	return pid
}

func removePid(logger *util.Logger, dataPath string) {
	if err := os.RemoveAll(path.Join(dataPath, "freighter.pid")); err != nil {
		logger.Warnf("freighterd: failed to remove pid file %q", err)
	}
}
