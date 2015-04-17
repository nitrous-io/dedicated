package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
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

func simulateFreighterWorker() {
	port := 65333
	fmt.Printf("starting fake freighter server on port %d\n", port)
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
	cmdTender := startTenderMust()
	// TODO: heuristic to give tender time to boot up
	// should do it by watching tender's stdout for specific lines
	time.Sleep(time.Second * 5)
	s, err := httpGet("http://foolishbastard.n2odev.com")
	exitIfErr(err)
	fmt.Printf("s: '%s'\n", shortenString(s))
	os.Exit(1)
	cmdTender.Process.Kill()
}
