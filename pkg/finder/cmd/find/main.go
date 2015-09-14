// A convenient command to test the file finder (i.e. finder.FindFiles()).
package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/nitrous-io/dedicated/pkg/finder"
)

const (
	colorReset = "\x1b[0m\x1b[K"
	colorMatch = "\x1b[30;43m" // black text with yellow background
)

var (
	flgPositions = false // print positions of matches
	flgScore     = false // show match score
	flgStress    = false // run a stress test with several goroutines
	flgDouble    = false // run the same query twice. allows to compare cached vs. non-cached times
)

func panicIfErr(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func decorate(match *finder.FileMatch) string {
	offsets := match.Offsets
	if len(offsets) == 0 {
		return match.Path
	}

	positions := ""
	if flgPositions {
		for _, offset := range offsets {
			positions += fmt.Sprintf("[%d, %d] ", offset[0], offset[1])
		}
		fmt.Printf("s: %s\n", match.Path)
		fmt.Printf("pos: %s\n", positions)
	}

	res := ""
	prevEnd := 0
	for _, offset := range offsets {
		res += match.Path[prevEnd:offset[0]]
		res += colorMatch
		prevEnd = offset[1]
		res += match.Path[offset[0]:prevEnd]
		res += colorReset
	}
	res += match.Path[prevEnd:len(match.Path)]
	if flgScore {
		res += fmt.Sprintf(" | score: %d", match.Score)
	}
	return res
}

func doFind(dir, term string) {
	timeStart := time.Now()
	matches, err := finder.FuzzyFindFile(dir, term)
	panicIfErr(err)
	dur := time.Since(timeStart)
	for _, match := range matches {
		fmt.Printf("match: '%+v'\n", decorate(match))
	}
	fmt.Printf("new: found %d matches for '%s' in %s\n", len(matches), term, dur)
}

func parseFlags() {
	flag.BoolVar(&flgPositions, "show-positions", false, "show match positions")
	flag.BoolVar(&flgScore, "show-score", false, "show match score")
	flag.BoolVar(&flgStress, "stress", false, "run a stress test")
	flag.BoolVar(&flgDouble, "double", false, "run the same search twice")
	flag.Parse()
}

func stressSearches() {
	terms := []string{"euc", ".go", "google.com", "/finder/"}
	dir := "."
	var wg sync.WaitGroup
	for _, term := range terms {
		wg.Add(1)
		go func(term string) {
			matches, err := finder.FuzzyFindFile(dir, term)
			if err != nil {
				fmt.Printf("finder.FuzzyFindFile('%s', '%s') failed with '%s'\n", dir, term, err)
			} else {
				fmt.Printf("%d matches for '%s'\n", len(matches), term)
			}
			wg.Done()
		}(term)
	}
	wg.Wait()
}

func runStress() {
	finder.CachedDirExpiry = time.Second
	stressSearches()
	fmt.Printf("Refreshed %d cached entries\n", finder.NRefreshed)
	time.Sleep(time.Second * 2) // wait for cache to expire
	fmt.Print("\nSecond search\n")
	stressSearches()
	fmt.Printf("Refreshed %d cached entries\n", finder.NRefreshed)
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	parseFlags()
	if flgStress {
		runStress()
		return
	}
	dir := "."
	term := "euc"
	args := flag.Args()
	if len(args) > 0 {
		term = args[0]
	}
	if len(args) > 1 {
		dir = args[1]
	}
	doFind(dir, term)
	if flgDouble {
		doFind(dir, term)
	}
}
