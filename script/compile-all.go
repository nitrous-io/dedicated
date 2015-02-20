package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

/*
This scripts compiles every go package in the repo.
With -all flag also compiles tests.
It helps to catch build breaks.

Compiling packages:   ~20 secs.
Compiling with tests: ~3 mins.

The code has an option to run multiple compilation in parallel but in
practice it doesn't improve anything (even though CPU usage is higher).

To run: go run scripts/compile-all.go

On Mac this will skip packages that cannot be built on Mac.
To build everything on mac, use scripts/compiel-all-in-docker.sh, which
compiles on Linux using a docker container.
*/

var (
	buildDir      string
	wgCount       int32
	flgCompileAll bool
)

func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func verifyRunningInRightDirMust() {
	if !PathExists("nitrous") || !PathExists("freighter") {
		log.Fatalf("must be run from top-level directory e.g.:\ngo run ./script/build-dev.go\n")
	}
}

func getCurrentDir() string {
	curDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("os.Getwd() failed with '%s'\n", err)
	}
	return curDir
}

func getCwdSha1ShortMust() string {
	cmd := exec.Command("git", "rev-list", "--max-count=1", "--abbrev-commit", "HEAD")
	d, err := cmd.Output()
	if err != nil {
		log.Fatalf("git failed with '%s'\n", err)
	}
	return strings.TrimSpace(string(d))
}

func getCmdFull(cmd *exec.Cmd) string {
	// should quote args when necessary
	return strings.Join(cmd.Args, " ")
}

func goBuild(dir string) {
	timeStart := time.Now()
	name := filepath.Base(dir)
	dst := filepath.Join(buildDir, name)

	cmd := exec.Command("godep", "go", "build", "-o", dst)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Command '%s' in dir '%s' failed with '%s', output:\n%s\n", getCmdFull(cmd), dir, err, string(out))
	}
	dur := time.Since(timeStart)
	fmt.Printf("Ran '%s' in dir '%s' in %s\n", getCmdFull(cmd), dir, dur)
}

func goInstall(dir string) {
	timeStart := time.Now()

	pkgName := "github.com/nitrous-io/dedicated/" + dir
	cmd := exec.Command("godep", "go", "install", pkgName)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Command '%s' in dir '%s' failed with '%s', output:\n%s\n", getCmdFull(cmd), dir, err, string(out))
	}
	dur := time.Since(timeStart)
	fmt.Printf("Ran '%s' in dir '%s' in %s\n", getCmdFull(cmd), dir, dur)
}

func rmDotTest(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatalf("rmDotTest: ioutil.ReadDir('%s') failed with '%s'", dir, err)
	}
	for _, fi := range files {
		if fi.Mode().IsRegular() {
			if strings.HasSuffix(fi.Name(), ".test") {
				path := filepath.Join(dir, fi.Name())
				err = os.Remove(path)
				if err != nil {
					log.Fatalf("rmDotTest: os.Remove('%s') failed with '%s'", path, err)
				}
			}
		}
	}
}

func compileTests(dir string) {
	if !flgCompileAll {
		return
	}
	timeStart := time.Now()
	cmd := exec.Command("godep", "go", "test", "-c", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Command '%s' in dir '%s' failed with '%s', output:\n%s\n", getCmdFull(cmd), dir, err, string(out))
	}
	dur := time.Since(timeStart)
	fmt.Printf("Ran '%s' in dir '%s' in %s\n", getCmdFull(cmd), dir, dur)
	rmDotTest(dir)
}

func createCleanBuildDir() {
	curDir := getCurrentDir()
	buildDir = filepath.Join(curDir, "build")
	if err := os.RemoveAll(buildDir); err != nil {
		log.Fatalf("createBuildDir: os.RemoveAll('%s') failed with '%s'", buildDir, err)
	}
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		log.Fatalf("os.MkdirAll('%s') failed with '%s'\n", buildDir, err)
	}
}

func buildWorker(exeDirs, pkgDirs, testDirs chan string, wg *sync.WaitGroup) {
	for {
		select {
		case dir := <-exeDirs:
			goBuild(dir)
			wg.Done()
		case <-pkgDirs:
			// we assume that every non-main package is used by some main package
			// so save time by not building them
			//build(dir)
			wg.Done()
		case dir := <-testDirs:
			compileTests(dir)
			wg.Done()
		}
	}
}

func installWorker(exeDirs, pkgDirs, testDirs chan string, wg *sync.WaitGroup) {
	for {
		select {
		case dir := <-exeDirs:
			goInstall(dir)
			wg.Done()
		case <-pkgDirs:
			// we assume that every non-main package is used by some main package
			// so save time by not building them
			//build(dir)
			wg.Done()
		case dir := <-testDirs:
			compileTests(dir)
			wg.Done()
		}
	}
}

func isMainPackage(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("isMainPackage: os.Open('%s') failed with %s", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := scanner.Text()
		if strings.Contains(s, "package main") {
			return true
		}
		return false
	}
	return false
}

func isMac() bool {
	return runtime.GOOS == "darwin"
}

func findToCompile(exeDirs chan string, pkgDirs chan string, testDirs chan string, wg *sync.WaitGroup) {
	dirsToSkip := map[string]bool{
		"Godeps":                       true,
		"script":                       true,
		"nitrous/cmd/createmxprofiles": true, // TODO: doesn't compile atm
		"nitro": true, // old code
	}
	// some packages don't compile on Mac due to dependencies on C library
	macDirsToSkip := map[string]bool{
		"freighter/collab":                    true,
		"freighter/freighterd":                true,
		"freighter/cops":                      true,
		"freighter/provider":                  true,
		"freighter/util":                      true,
		"freighter/server":                    true,
		"nitrous/cmd/populatehosttransitions": true,
		"freighter/pkg/stats":                 true,
	}
	if !isMac() {
		macDirsToSkip = map[string]bool{}
	}
	dirsToVisit := []string{"."}
	for len(dirsToVisit) > 0 {
		dir := dirsToVisit[0]
		dirsToVisit = dirsToVisit[1:]
		files, err := ioutil.ReadDir(dir)
		var hasGo, hasGoMain, hasGoTest bool
		if err != nil {
			log.Fatalf("findToCompile: ioutil.ReadDir('%s') failed with %s", dir, err)
		}
		for _, fi := range files {
			name := fi.Name()
			if fi.IsDir() {
				path := name
				if dir != "." {
					path = filepath.Join(dir, name)
				}
				if !dirsToSkip[path] && !macDirsToSkip[path] {
					dirsToVisit = append(dirsToVisit, path)
				} else {
					fmt.Printf("skipped dir %s\n", path)
				}
				continue
			}

			if !strings.HasSuffix(name, ".go") {
				continue
			}
			// since all .go files in a directory should be in the same package,
			// only read the first .go file to determine package name
			if !hasGo {
				path := filepath.Join(dir, name)
				hasGoMain = isMainPackage(path)
			}
			hasGo = true
			if strings.HasSuffix(name, "_test.go") {
				hasGoTest = true
			}
		}

		if hasGoTest {
			wg.Add(1)
			testDirs <- dir
		}

		if hasGo {
			wg.Add(1)
			if hasGoMain {
				exeDirs <- dir
			} else {
				pkgDirs <- dir
			}
		}
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.BoolVar(&flgCompileAll, "all", false, "should compile tests?")
	flag.Parse()
	nWorkers := 1
	fmt.Printf("flgCompileAll: %v, nWorkers: %d\n", flgCompileAll, nWorkers)
	timeStart := time.Now()
	verifyRunningInRightDirMust()
	createCleanBuildDir()

	var wg sync.WaitGroup
	exeDirs := make(chan string, 128)
	pkgDirs := make(chan string, 128)
	testDirs := make(chan string, 128)
	for i := 0; i < nWorkers; i++ {
		go installWorker(exeDirs, pkgDirs, testDirs, &wg)
	}
	findToCompile(exeDirs, pkgDirs, testDirs, &wg)
	wg.Wait()
	fmt.Printf("Total time: %s\n", time.Since(timeStart))
}
