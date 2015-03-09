package finder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

/*
TODO:
- .gitignore support
*/

const (
	// user can provide multiple search terms by separating them by space
	// they act as AND (all must match for the query to match)
	// to allow us optimize the code, we limit number of terms we recognize
	// to more-than-generous value
	maxTerms = 8
)

var (
	mu sync.Mutex
	// maps full path to DirInfo
	cachedDirs       map[string]*DirInfo
	chPathsToRefresh chan string

	// how long we keep cached DirInfo in memory
	// exported so that we can shorten it in test
	CachedDirExpiry = time.Minute * 5
	// for debugging
	NRefreshed = 0
)

type DirInfo struct {
	// full path for the directory this DirInfo represents
	dir        string
	cachedTime time.Time
	nDirs      int
	nFiles     int
	// this is an optimization: we store name of files and sub-directories
	// in a single array. Total number of entries is nFiles + nDirs. Directories
	// are first
	dirsAndFiles []string
}

func (di *DirInfo) GetDir(n int) string {
	return di.dirsAndFiles[n]
}

func (di *DirInfo) GetFile(n int) string {
	return di.dirsAndFiles[di.nDirs+n]
}

func (di *DirInfo) Expired() bool {
	return time.Since(di.cachedTime) > CachedDirExpiry
}

type MatchPos struct {
	Pos int
	Len int
}

type matchPosByPos []MatchPos

func (s matchPosByPos) Len() int {
	return len(s)
}

func (s matchPosByPos) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s matchPosByPos) Less(i, j int) bool {
	return s[i].Pos < s[j].Pos
}

type FileNameMatch struct {
	FilePath string
	Matches  []MatchPos
	Score    float64
}

type fileNameMatchByScore []*FileNameMatch

func (s fileNameMatchByScore) Len() int {
	return len(s)
}

func (s fileNameMatchByScore) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s fileNameMatchByScore) Less(i, j int) bool {
	return s[i].Score > s[j].Score
}

func init() {
	cachedDirs = make(map[string]*DirInfo)
	chPathsToRefresh = make(chan string, 1000)
	go refreshDirInfoWorker()
}

func shouldIgnoreDir(fi os.FileInfo) bool {
	// we skip directories that are symlinks since that leads to matching
	// the same file twice
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name := fi.Name()
	if strings.EqualFold(name, ".git") {
		return true
	}
	if strings.EqualFold(name, ".svn") {
		return true
	}
	return false
}

func getDirInfo(path string) (*DirInfo, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", fi.Name())
	}
	var di = &DirInfo{
		dir:        path,
		cachedTime: time.Now(),
	}
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	nDirs := 0
	nFiles := 0
	for _, fi := range entries {
		if fi.IsDir() {
			if !shouldIgnoreDir(fi) {
				nDirs++
			}
		} else {
			nFiles++
		}
	}
	di.nDirs = nDirs
	di.nFiles = nFiles
	nTotal := nDirs + nFiles
	di.dirsAndFiles = make([]string, nTotal, nTotal)
	dirPos := 0
	filePos := nDirs
	for _, fi := range entries {
		name := fi.Name()
		if fi.IsDir() {
			if !shouldIgnoreDir(fi) {
				di.dirsAndFiles[dirPos] = name
				dirPos++
			}
		} else {
			di.dirsAndFiles[filePos] = name
			filePos++
		}
	}
	return di, nil
}

func refreshDirInfoWorker() {
	for path := range chPathsToRefresh {
		mu.Lock()
		di := cachedDirs[path]
		mu.Unlock()
		if di != nil && !di.Expired() {
			// two different goroutines could put the same dir on refresh queue,
			// in which case we want to avoid re-reading the directory
			continue
		}
		NRefreshed++
		di, err := getDirInfo(path)
		mu.Lock()
		if err != nil {
			// most likely because the dir was deleted. either way, remove from cache
			delete(cachedDirs, path)
		} else {
			cachedDirs[path] = di
		}
		mu.Unlock()
	}
}

func getCachedDir(path string) (*DirInfo, error) {
	mu.Lock()
	di := cachedDirs[path]
	mu.Unlock()
	if di != nil {
		if di.Expired() {
			chPathsToRefresh <- path
		}
		return di, nil
	}

	di, err := getDirInfo(path)
	if err != nil {
		return nil, err
	}

	mu.Lock()
	cachedDirs[path] = di
	mu.Unlock()
	return di, nil
}

func collectDirInfos(dir string) ([]*DirInfo, error) {
	var res []*DirInfo
	dirsToVisit := []string{dir}

	for len(dirsToVisit) > 0 {
		dir = dirsToVisit[0]
		dirsToVisit = dirsToVisit[1:]

		di, err := getCachedDir(dir)
		if err != nil {
			return res, err
		}
		res = append(res, di)

		for i := 0; i < di.nDirs; i++ {
			path := filepath.Join(dir, di.GetDir(i))
			dirsToVisit = append(dirsToVisit, path)
		}
	}
	return res, nil
}

func strInArray(s string, a []string) bool {
	for _, str := range a {
		if s == str {
			return true
		}
	}
	return false
}

// split by space and lower case
func parseSearchTerm(term string) []string {
	var terms []string
	parts := strings.Split(term, " ")
	for _, term := range parts {
		if len(term) == 0 {
			continue
		}
		// matching is always case insensitive
		term = strings.ToLower(term)
		if strInArray(term, terms) {
			continue
		}
		terms = append(terms, term)
		if len(terms) >= maxTerms {
			return terms
		}
	}
	return terms
}

// returns nil if no match
// our scoring:
// - file match scores higher than dir match
//   - we give 200 points for file match
//   - 100 points for dir match
// - we favor relatively larger file matches i.e. "foo" in "foo" scores higher
//   than "foo" in "foobar". We allocate 40 points * $matchStrength
//   where $matchStrength is 1..0, where 1 means "all letters match",
//   0.5 means "matched part is half of the matched string"
func exactMatch(dir, name string, terms []string) *FileNameMatch {
	nameLower := strings.ToLower(name)
	pathLower := filepath.Join(strings.ToLower(dir), nameLower)
	var positions [maxTerms * 2]MatchPos // *2 because both file and dir might match

	posFileAdjust := 0
	if len(dir) > 0 {
		posFileAdjust = len(dir) + 1 // +1 for "/"
	}
	var score float64
	nPos := 0
	for _, term := range terms {
		posFile := strings.Index(nameLower, term)
		posPath := strings.Index(pathLower, term)
		if posFile == -1 && posPath == -1 {
			return nil
		}

		// if only file matches, it'll match in path as well, don't count it twice
		if posFile != -1 && posPath == posFile+posFileAdjust {
			posPath = -1
		}
		if posPath != -1 {
			score += 100
			positions[nPos].Pos = posPath
			positions[nPos].Len = len(term)
			nPos++
		}
		if posFile != -1 {
			score += 200
			scoreAdj := (40 * len(term)) / len(name)
			score += float64(scoreAdj)
			positions[nPos].Pos = posFile + posFileAdjust
			positions[nPos].Len = len(term)
			nPos++
		}
	}

	matchPositions := make([]MatchPos, nPos, nPos)
	for i := 0; i < nPos; i++ {
		matchPositions[i] = positions[i]
	}
	sort.Sort(matchPosByPos(matchPositions))

	match := &FileNameMatch{
		FilePath: filepath.Join(dir, name),
		Score:    score,
		Matches:  matchPositions,
	}
	return match
}

// a match can come from cache. to avoid false positives, we verify that
// matched file still exists on disk
func fileStillExists(match *FileNameMatch) bool {
	_, err := os.Stat(match.FilePath)
	return err == nil
}

// FindFiles recursively finds all files matching a search term, which is a
// space-separated list of strings that all must match.
// Matching is case-insensitive.
// Returned matches are sorted by match score.
func FindFiles(dir string, term string) ([]*FileNameMatch, error) {
	dir = filepath.Clean(dir)
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", dir)
	}

	terms := parseSearchTerm(term)
	dirs, err := collectDirInfos(dir)
	if err != nil {
		return nil, err
	}
	var matches []*FileNameMatch
	nTotalFiles := 0
	for _, di := range dirs {
		nTotalFiles += di.nFiles
		for i := 0; i < di.nFiles; i++ {
			match := exactMatch(di.dir, di.GetFile(i), terms)
			if match != nil && fileStillExists(match) {
				matches = append(matches, match)
			}
		}
	}
	sort.Sort(fileNameMatchByScore(matches))
	return matches, nil
}
