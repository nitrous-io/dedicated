package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	rpprof "runtime/pprof"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/facebookgo/httpdown"
	"github.com/nitrous-io/dedicated/hostbuilder"
)

var (
	version string

	// StopTimeout is the duration before we begin force closing connections.
	// Defaults to 15 minutes.
	defaultStopTimeout = 900 * time.Second // 15 * time.Minute

	// KillTimeout is the duration before which we completely give up and abort
	// even though we still have connected clients. This is useful when a large
	// number of client connections exist and closing them can take a long time.
	// Note, this is in addition to the StopTimeout. Defaults to 1 minute.
	defaultKillTimeout = 60 * time.Second // 1 * time.Minute
)

type providerConfig map[string]string

type config struct {
	Providers []map[string]providerConfig `json:"providers"`
}

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	flConfig := flag.String("config", "", "path to the config file")
	flVersion := flag.Bool("v", false, "current version")
	flag.Parse()
	if *flVersion {
		fmt.Printf("hostbuilder version %v\n", version)
		os.Exit(0)
		return
	}
	if len(*flConfig) == 0 {
		logrus.Fatal("hostbuilder: missing config file")
	}
	configData, err := ioutil.ReadFile(*flConfig)
	if err != nil {
		logrus.Fatal("hostbuilder: failed to read config file")
	}

	// build the server routes
	server, err := hostbuilder.New(configData)
	if err != nil {
		logrus.Fatalf("hostbuilder: failed to create server %v", err)
	}
	http.Handle("/", server.Router)

	// create the http server
	httpServer := &http.Server{Addr: server.Config.Address, Handler: nil}

	// setup httpdown wrapper so we can gracefully stop
	stopTimeout := time.Duration(server.Config.StopTimeout) * time.Second
	if stopTimeout == 0 {
		stopTimeout = defaultStopTimeout
	}
	killTimeout := defaultKillTimeout
	hd := &httpdown.HTTP{
		StopTimeout: stopTimeout,
		KillTimeout: killTimeout,
	}

	// start listening to request
	// The server will be gracefully stopped when it receives SIGTERM or SIGINT
	// so another server can be started in its place.
	logrus.Infof("starting server at %v", server.Config.Address)
	hs, err := hd.ListenAndServe(httpServer)
	if err != nil {
		logrus.Fatalf("hostbuilder: failed to start server %v", err)
	}

	waiterr := make(chan error, 1)
	go func() {
		defer close(waiterr)
		waiterr <- hs.Wait()
	}()

	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1, syscall.SIGUSR2)

	for {
		select {
		case err := <-waiterr:
			if err != nil {
				logrus.Fatalf("hostbuilder: failed to wait for the serving loop to finish %v", err)
				return
			}
		case s := <-signals:
			logrus.Infof("signal received: %s", s)
			switch s {
			case syscall.SIGUSR1:
				profile := rpprof.Lookup("goroutine")
				logrus.Infof("=== BEGIN goroutine stack dump ====")
				profile.WriteTo(logrus.StandardLogger().Out, 2)
				logrus.Infof("=== END goroutine stack dump ===")
			case syscall.SIGUSR2:
				if server.Config.ProfileDumpFolder != "" {
					dumpFileName := fmt.Sprintf("%s/hostbuilder-%d.prof", server.Config.ProfileDumpFolder, time.Now().Unix())
					f, err := os.Create(dumpFileName)
					if err == nil {
						logrus.Infof("Dumping CPU profile to %s....", dumpFileName)
						rpprof.StartCPUProfile(f)
						time.Sleep(10 * time.Second)
						rpprof.StopCPUProfile()
						f.Close()
						logrus.Infof("Dumped CPU profile to %s", dumpFileName)
					} else {
						logrus.Infof("Failed CPU profiling: %+v", err)
					}
				} else {
					logrus.Info("profile_dump_folder is not specified")
				}
			default:
				signal.Stop(signals)
				if err := hs.Stop(); err != nil {
					logrus.Fatalf("hostbuilder: failed to wait for httpdown to stop %v", err)
					return
				}

				if err := <-waiterr; err != nil {
					logrus.Fatalf("hostbuilder: failed to wait for the serving loop to finish %v", err)
					return
				}

				logrus.Infof("stopped server")
				return
			}
		}
	}
}
