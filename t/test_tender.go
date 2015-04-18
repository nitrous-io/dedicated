package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

/*
This orchestrates testing of tender by starting up all processes
that need to be started and running test logic.

On failure does os.Exit(1).
*/

const (
	defaultFreighterPort = 65333
	tlsDomain            = ".n2odev.com"

	myCACertPath = "script/certs/n2odevpro-ca.crt"
	myCertPath   = "script/certs/n2odev.crt"
	myKeyPath    = "script/certs/n2odev.key"
)

var (
	privCACert []byte
)

func exitIfErr(err error) {
	if err != nil {
		fmt.Printf("got error: %s\n", err)
		os.Exit(1)
	}
}

func readLinesMust(path string) []string {
	var lines []string
	f, err := os.Open(path)
	exitIfErr(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := scanner.Text()
		s = strings.TrimSpace(s)
		lines = append(lines, s)
	}
	if err := scanner.Err(); err != nil {
		exitIfErr(err)
	}
	return lines
}

func isEmptyOrCommentLine(s string) bool {
	if s == "" {
		return true
	}
	return s[0] == '#'
}

// return env strings
func readEnvFileMust(path string, withOsEnviron bool) []string {
	var env []string
	if withOsEnviron {
		env = os.Environ()
	}
	lines := readLinesMust(path)
	for _, l := range lines {
		if isEmptyOrCommentLine(l) {
			continue
		}
		parts := strings.SplitN(l, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid env line: '%s'\n", l)
		}
		env = append(env, l)
	}
	return env
}

func dumpEnv(env []string) {
	for _, e := range env {
		fmt.Printf("%s\n", e)
	}
}

type FakeFreighterServer struct {
}

func (s *FakeFreighterServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("url: %s\n", r.URL)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("hello"))
}

// TcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
//
// This is copied from net/http package
type TcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln TcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func simulateFreighterWorker() {
	port := 61000
	cert, err := tls.LoadX509KeyPair(myCertPath, myKeyPath)
	exitIfErr(err)

	d, err := ioutil.ReadFile(myCACertPath)
	exitIfErr(err)
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(d); !ok {
		exitIfErr(fmt.Errorf("Couldn't append CA certificate %s", myCACertPath))
	}
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
		ClientCAs:    certPool,
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS10,
	}
	tlsConfig.BuildNameToCertificate()

	fmt.Printf("starting fake freighter server on port %d\n", port)
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:      addr,
		Handler:   &FakeFreighterServer{},
		TLSConfig: tlsConfig,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("simulateFreighterWorker(): net.Listen() failed with %s", err)
	}
	ln = tls.NewListener(TcpKeepAliveListener{ln.(*net.TCPListener)}, tlsConfig)
	err = srv.Serve(ln)
	exitIfErr(err)
}

func startTenderMust() *exec.Cmd {
	cmd := exec.Command("build/tender")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = readEnvFileMust("t/.env-tender", true)
	//dumpEnv(cmd.Env)
	err := cmd.Start()
	exitIfErr(err)
	return cmd
}

func httpGet(uri string) (string, error) {
	res, err := http.Get(uri)
	if err != nil {
		return "", err
	}
	d, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return "", err
	}
	if res.StatusCode != 200 {
		return "", errors.New("Not 200")
	}
	return string(d), nil
}

func shortenString(s string) string {
	maxLen := 78
	if len(s) >= maxLen {
		return s[:maxLen]
	}
	return s
}

func main() {
	fmt.Printf("starting tender tests\n")
	go simulateFreighterWorker()
	cmdTender := startTenderMust()
	// TODO: heuristic to give tender time to boot up
	// should do it by watching tender's stdout for specific lines
	time.Sleep(time.Second * 5)
	uri := fmt.Sprintf("https://local.n2odev.com:%d", defaultFreighterPort)
	s, err := httpGet(uri)
	exitIfErr(err)
	fmt.Printf("s: '%s'\n", shortenString(s))
	os.Exit(1)
	cmdTender.Process.Kill()
}
