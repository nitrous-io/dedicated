package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	rpprof "runtime/pprof"
	"strconv"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/tender"
)

// current version of tender binary
var version string

const (
	freighterdForwardPort = 443 // We forward this port to FREIGHTERD_PORT.
	cacheRefreshPeriod    = time.Minute
)

var (
	// TODO: eventually we may look into etcd ( and co ) so we dont have to touch
	// the main database *because* latency
	databaseURL    string
	freighterdPort int64

	hostBaseDomain string
	hostCACert     string
	hostCAKey      string

	containerBaseDomain string
	containerCACert     string
	containerCAKey      string
	previewStartPort    string
	previewEndPort      string

	tlsDomain      string
	privCACertPath string

	flVersion     = flag.Bool("v", false, "Display the version and exit")
	flVerboseLogs = flag.Bool("verbose", false, "show verbose logging")

	profileDumpFolder string
)

func init() {
	databaseURL = envStr("DATABASE_URL")
	freighterdPort = envInt("FREIGHTERD_PORT")

	// credentials to serve HTTPS for host-slug.nitrousproxy.com
	// which is the proxy to freighter remote api on each host
	hostBaseDomain = envStr("HOST_BASE_DOMAIN")
	if hostBaseDomain[0] != '.' {
		logrus.Fatalf("HOST_BASE_DOMAIN should start with .")
	}
	hostCACert = envStr("HOST_CA_CRT")
	hostCAKey = envStr("HOST_CA_KEY")

	// this is the host domain that we should forward request to
	// this is used to talk to freighterd on the host
	tlsDomain = envStr("TLS_DOMAIN")
	// this is the ca for TLS_DOMAIN
	privCACertPath = envStr("PRI_CRT")

	containerBaseDomain = envStr("CONTAINER_BASE_DOMAIN")
	if containerBaseDomain[0] != '.' {
		logrus.Fatalf("CONTAINER_BASE_DOMAIN should start with .")
	}
	containerCACert = envStr("CONTAINER_CA_CRT")
	containerCAKey = envStr("CONTAINER_CA_KEY")
	previewStartPort = os.Getenv("PREVIEW_START_PORT")
	previewEndPort = os.Getenv("PREVIEW_END_PORT")

	profileDumpFolder = os.Getenv("PROFILE_DUMP_FOLDER")
}

func refreshCachesWorker(db *sql.DB) {
	var err error
	for {
		// Compromise between accuracy and performance (i.e. hitting db all the time)
		// Periodically refresh the cache with the latest state of running containers,
		// so the maximum amount of time a host can be out of date is cacheRefreshPeriod.
		// When cache hit is a miss (host that transitioned to `running` state
		// between refreshes), we fetch from db
		time.Sleep(cacheRefreshPeriod)
		if err = tender.RefreshContainersCache(db); err != nil {
			logrus.Errorf("RefreshContainersCache() failed with %s", err)
		}
		if err = tender.RefreshHostsCache(db); err != nil {
			logrus.Errorf("RefreshHostsCache() failed with %s", err)
		}
	}
}

func main() {
	flag.Parse()
	if *flVersion {
		fmt.Printf("tender version: %s\n", version)
		os.Exit(0)
	}

	if *flVerboseLogs {
		tender.VerboseLog = 1
	}
	tender.InitDNS()

	db, err := connectDB(databaseURL)
	if err != nil {
		logrus.Fatalf("connectDB: %v", err)
	}

	privCACert, err := ioutil.ReadFile(privCACertPath)
	if err != nil {
		logrus.Fatalf("tender: failed to find private certificate at %s: %v", privCACertPath, err)
	}

	timeStart := time.Now()
	tender.RefreshContainersCache(db)
	logrus.Infof("initial refreshContainersCache() took %s", time.Since(timeStart))

	timeStart = time.Now()
	tender.RefreshHostsCache(db)
	logrus.Infof("initial refreshHostsCache() took %s", time.Since(timeStart))

	go refreshCachesWorker(db)

	freighterProxy := tender.NewFreighterProxyServer(hostBaseDomain, tlsDomain, db, privCACert)
	previewProxy := tender.NewPreviewProxyServer(containerBaseDomain, tlsDomain, int(freighterdPort), db, privCACert)

	hostCert, err := tls.LoadX509KeyPair(hostCACert, hostCAKey)
	if err != nil {
		logrus.Fatalf("tender: failed to load host proxy's SSL certificates: %v", err)
	}
	containerCert, err := tls.LoadX509KeyPair(containerCACert, containerCAKey)
	if err != nil {
		logrus.Fatalf("tender: failed to load container proxy's SSL certificates: %v", err)
	}

	go handleSignals()

	go serve443(hostCert, containerCert, freighterProxy, previewProxy)
	servePreviewProxy(previewProxy)
	serveFreighterdProxy(hostCert, freighterProxy)
}

func serve443(hostCert, containerCert tls.Certificate, freighterProxy, previewProxy http.Handler) {
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{hostCert, containerCert},
	}
	tlsConfig.BuildNameToCertificate()

	proxyServer := &tender.VhostServer{
		FreighterBaseDomain:  hostBaseDomain,
		FreighterProxyServer: freighterProxy,
		PreviewBaseDomain:    containerBaseDomain,
		PreviewProxyServer:   previewProxy,
	}

	logrus.Printf("listening to port %d", freighterdForwardPort)
	logrus.Fatal(tender.ServeTLS(freighterdForwardPort, tlsConfig, proxyServer))
}

func serveFreighterdProxy(cert tls.Certificate, handler http.Handler) {
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
	}

	logrus.Printf("listening to port %d", freighterdPort)
	logrus.Fatal(tender.ServeTLS(freighterdPort, tlsConfig, handler))
}

func servePreviewProxy(handler http.Handler) {
	startPort, endPort, err := tender.PortsRange(previewStartPort, previewEndPort)
	if err != nil {
		logrus.Fatalf("tender: invalid ports, %q", err)
	}
	logrus.Printf("listen to port from %d to %d", startPort, endPort)

	// proxy port range for preview
	for p := startPort; p <= endPort; p++ {
		go func(port int64) {
			logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), handler))
		}(p)
	}

	// Listen to port 80.
	go func() {
		logrus.Println("listening to port 80")
		logrus.Fatal(http.ListenAndServe(":80", handler))
	}()
}

func envStr(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing env var " + name)
	}
	return v
}

func envInt(name string) int64 {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing " + name)
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("Invalid %s. Required integer, got %s", name, v))
	}
	return i
}

// connectDB establish a valid connection with our postgresql database
func connectDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open a db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to get a valid db connection %v", err)
	}
	return db, nil
}

func handleSignals() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGUSR1, syscall.SIGUSR2)
	for s := range ch {
		logrus.Infof("tender: signal received: %s", s)
		switch s {
		case syscall.SIGUSR1:
			profile := rpprof.Lookup("goroutine")
			logrus.Infof("=== BEGIN goroutine stack dump ====")
			profile.WriteTo(logrus.StandardLogger().Out, 2)
			logrus.Infof("=== END goroutine stack dump ===")
		case syscall.SIGUSR2:
			if profileDumpFolder != "" {
				dumpFileName := fmt.Sprintf("%s/tender-%d.prof", profileDumpFolder, time.Now().Unix())
				f, err := os.Create(dumpFileName)
				if err == nil {
					logrus.Infof("tender: Dumping CPU profile to %s for 10s", dumpFileName)
					rpprof.StartCPUProfile(f)
					time.Sleep(10 * time.Second)
					rpprof.StopCPUProfile()
					f.Close()
					logrus.Infof("tender: Dumped CPU profile to %s", dumpFileName)
				} else {
					logrus.Infof("tender: Failed CPU profiling %+v", err)
				}
			} else {
				logrus.Info("tender: PROFILE_DUMP_FOLDER is not specified")
			}
		}
	}
}
