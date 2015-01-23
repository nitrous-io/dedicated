package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/tender"
)

// current version of tender binary
var version string

const freighterdForwardPort = 443 // We forward this port to FREIGHTERD_PORT.

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

	flVersion = flag.Bool("v", false, "Display the version and exit")
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

func main() {
	flag.Parse()
	if *flVersion {
		fmt.Printf("tender version: %s\n", version)
		os.Exit(0)
	}

	db, err := connectDB(databaseURL)
	if err != nil {
		log.Fatalf("connectDB: %v", err)
	}

	privCACert, err := ioutil.ReadFile(privCACertPath)
	if err != nil {
		log.Fatalf("tender: failed to find private certificate at %s: %v", privCACertPath, err)
	}

	freighterPortMap := map[int]int{freighterdForwardPort: int(freighterdPort)}
	freighterProxy := tender.NewFreighterProxyServer(hostBaseDomain, tlsDomain, freighterPortMap, db, privCACert)
	previewProxy := tender.NewPreviewProxyServer(containerBaseDomain, tlsDomain, db, privCACert)

	hostCert, err := tls.LoadX509KeyPair(hostCACert, hostCAKey)
	if err != nil {
		log.Fatalf("tender: failed to load host proxy's SSL certificates: %v", err)
	}
	containerCert, err := tls.LoadX509KeyPair(containerCACert, containerCAKey)
	if err != nil {
		log.Fatalf("tender: failed to load container proxy's SSL certificates: %v", err)
	}

	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{hostCert, containerCert},
	}
	tlsConfig.BuildNameToCertificate()
	addr := fmt.Sprintf(":%d", freighterdForwardPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("tender: failed to bind to %s, err: %v", addr, err)
	}
	ln = tls.NewListener(tender.TcpKeepAliveListener{ln.(*net.TCPListener)}, tlsConfig)
	proxyServer := &vhostServer{
		freighterProxyServer: freighterProxy,
		previewProxyServer:   previewProxy,
	}
	srv := &http.Server{
		Addr:      addr,
		Handler:   proxyServer,
		TLSConfig: tlsConfig,
	}
	go func() {
		log.Fatal(srv.Serve(ln))
	}()

	servePreviewProxy(previewProxy)
	serveFreighterdProxy(freighterProxy)
}

type vhostServer struct {
	freighterProxyServer http.Handler
	previewProxyServer   http.Handler
}

func (s *vhostServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.Host, hostBaseDomain) {
		s.freighterProxyServer.ServeHTTP(w, r)
	} else if strings.HasSuffix(r.Host, containerBaseDomain) {
		s.previewProxyServer.ServeHTTP(w, r)
	}
}

// this runs in foreground and should be called after servePreviewProxy
func serveFreighterdProxy(s http.Handler) {
	crt, err := tls.LoadX509KeyPair(hostCACert, hostCAKey)
	if err != nil {
		log.Fatalf("tender: Failed to load freighter's SSL certificates: %q", err)
	}
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{crt},
	}

	log.Printf("listening to port %d", freighterdPort)
	log.Fatal(tender.ServeTLS(freighterdPort, tlsConfig, s))
}

// this runs in background, and should be called before serveFreighterdProxy
func servePreviewProxy(s http.Handler) {
	startPort, endPort, err := tender.PortsRange(previewStartPort, previewEndPort)
	if err != nil {
		log.Fatalf("tender: invalid ports, %q", err)
	}
	log.Printf("listen to port from %d to %d", startPort, endPort)
	if startPort != config.MinPreviewPort || endPort != config.MaxPreviewPort {
		log.Printf(
			"tender: [warn] mismatched ports with freighter. Expect %d-%d, got %d-%d",
			config.MinPreviewPort, config.MaxPreviewPort, startPort, endPort,
		)
	}

	crt, err := tls.LoadX509KeyPair(containerCACert, containerCAKey)
	if err != nil {
		log.Fatalf("failed to load container's SSL certificates: %q", err)
	}
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{crt},
	}

	// proxy port range for preview
	for p := startPort; p <= endPort; p++ {
		go func(p int64) {
			log.Fatal(tender.ServeTLS(p, tlsConfig, s))
		}(p)
	}
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
