package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider/docker"
	"github.com/nitrous-io/freighter/server"
	"github.com/nitrous-io/freighter/util"
)

var version string

func main() {
	flDataPath := flag.String("d", "/var/lib/freighter", "Specify the path to store freighter's data files")
	flPort := flag.Int("p", 3342, "Specify the port on which freighterd listens for connections")
	flVersion := flag.Bool("v", false, "Display the version and exit")
	flag.Parse()

	if *flVersion {
		fmt.Printf("freighterd: current version %v\n", version)
		os.Exit(0)
	}

	logger := &util.Logger{Level: util.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			logger.FatalExit(r)
		}
	}()

	_, err := os.Stat(*flDataPath)
	if err != nil && os.IsNotExist(err) {
		if err := os.Mkdir(*flDataPath, 0700); err != nil {
			logger.FatalExit(err)
		}
	} else if err != nil {
		logger.FatalExit(err)
	}

	file, err := os.Create(path.Join(*flDataPath, "freighter.pid"))
	if err != nil {
		logger.FatalExit(err)
	}

	pid := os.Getpid()
	_, err = file.WriteString(strconv.Itoa(pid))
	if err != nil {
		logger.FatalExit(err)
	}

	err = file.Close()
	if err != nil {
		logger.FatalExit(err)
	}

	store, err := datastore.NewSqliteDatastore(path.Join(*flDataPath, "freighter.sqlite3"))
	if err != nil {
		logger.FatalExit(err)
	}

	prov, err := docker.NewDockerProvider(store, *flDataPath)
	if err != nil {
		logger.FatalExit(err)
	}

	svr := server.NewServer(logger, prov, &server.HTTPCallback{})
	host := "0.0.0.0"
	port := uint16(*flPort)
	logger.Infof("freighterd (rev: %v, pid: %v) listening on %s:%d", version, pid, host, port)
	svr.Start(host, port)
}
