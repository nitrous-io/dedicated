package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
)

func init() {
	databaseURL = envStr("DATABASE_URL")
	freighterdPort = envInt("FREIGHTERD_PORT")

	// credentials to serve HTTPS for host-slug.nitrousproxy.com
	// which is the proxy to freighter remote api on each host
	hostBaseDomain = envStr("HOST_BASE_DOMAIN")
	if hostBaseDomain[0] != '.' {
		log.Fatalf("HOST_BASE_DOMAIN should start with .")
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
		log.Fatalf("CONTAINER_BASE_DOMAIN should start with .")
	}
	containerCACert = envStr("CONTAINER_CA_CRT")
	containerCAKey = envStr("CONTAINER_CA_KEY")
	previewStartPort = os.Getenv("PREVIEW_START_PORT")
	previewEndPort = os.Getenv("PREVIEW_END_PORT")
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
			log.Printf("RefreshContainersCache() failed with %s", err)
		}
		if err = tender.RefreshHostsCache(db); err != nil {
			log.Printf("RefreshHostsCache() failed with %s", err)
		}
	}
}

func installSignalHandlers() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			if s == syscall.SIGUSR1 {
				tender.ToggleVerboseLog()
			} else if s == syscall.SIGUSR2 {
				tender.DumpHostsCache()
			} else {
				log.Printf("Unexpected signal %s", s)
			}
		}
	}()
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
	installSignalHandlers()

	db, err := connectDB(databaseURL)
	if err != nil {
		log.Fatalf("connectDB: %v", err)
	}

	privCACert, err := ioutil.ReadFile(privCACertPath)
	if err != nil {
		log.Fatalf("tender: failed to find private certificate at %s: %v", privCACertPath, err)
	}

	timeStart := time.Now()
	tender.RefreshContainersCache(db)
	log.Printf("[INFO] initial refreshContainersCache() took %s", time.Since(timeStart))

	timeStart = time.Now()
	tender.RefreshHostsCache(db)
	log.Printf("[INFO] initial refreshHostsCache() took %s", time.Since(timeStart))

	go refreshCachesWorker(db)

	freighterProxy := tender.NewFreighterProxyServer(hostBaseDomain, tlsDomain, db, privCACert)
	previewProxy := tender.NewPreviewProxyServer(containerBaseDomain, tlsDomain, int(freighterdPort), db, privCACert)

	hostCert, err := tls.LoadX509KeyPair(hostCACert, hostCAKey)
	if err != nil {
		log.Fatalf("tender: failed to load host proxy's SSL certificates: %v", err)
	}
	containerCert, err := tls.LoadX509KeyPair(containerCACert, containerCAKey)
	if err != nil {
		log.Fatalf("tender: failed to load container proxy's SSL certificates: %v", err)
	}

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

	log.Printf("listening to port %d", freighterdForwardPort)
	log.Fatal(tender.ServeTLS(freighterdForwardPort, tlsConfig, proxyServer))
}

func serveFreighterdProxy(cert tls.Certificate, handler http.Handler) {
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
	}

	log.Printf("listening to port %d", freighterdPort)
	log.Fatal(tender.ServeTLS(freighterdPort, tlsConfig, handler))
}

func servePreviewProxy(handler http.Handler) {
	startPort, endPort, err := tender.PortsRange(previewStartPort, previewEndPort)
	if err != nil {
		log.Fatalf("tender: invalid ports, %q", err)
	}
	log.Printf("listen to port from %d to %d", startPort, endPort)

	// proxy port range for preview
	for p := startPort; p <= endPort; p++ {
		go func(port int64) {
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), handler))
		}(p)
	}

	// Listen to port 80.
	go func() {
		log.Println("listening to port 80")
		log.Fatal(http.ListenAndServe(":80", handler))
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
