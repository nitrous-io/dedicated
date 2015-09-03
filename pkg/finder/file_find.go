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

func (p MatchPos) String() string {
	return fmt.Sprintf("[%d,%d]", p.Pos, p.Len)
}

// sort by Pos. if position is the same, match by inverse Len (i.e. bigger first)
type MatchPosByPosInvLen []MatchPos

func (s MatchPosByPosInvLen) Len() int {
	return len(s)
}

func (s MatchPosByPosInvLen) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s MatchPosByPosInvLen) Less(i, j int) bool {
	if s[i].Pos == s[j].Pos {
		return s[i].Len > s[j].Len
	}
	return s[i].Pos < s[j].Pos
}

type MatchTerm struct {
	term              string
	fuzzyPathMatching bool
}

type FileNameMatch struct {
	FilePath string
	Matches  []MatchPos
	Score    float64
}

type byScoreAndPath []*FileNameMatch

func (b byScoreAndPath) Len() int {
	return len(b)
}

func (b byScoreAndPath) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byScoreAndPath) Less(i, j int) bool {
	switch {
	case b[i].Score > b[j].Score:
		return true
	case b[i].Score < b[j].Score:
		return false
	default:
		return b[i].FilePath < b[j].FilePath
	}
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
	fi, err := os.Stat(path)
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

func subdirs(dir string) ([]*DirInfo, error) {
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

func matchTermInArray(term MatchTerm, a []MatchTerm) bool {
	for _, t := range a {
		if term.term == t.term {
			return true
		}
	}
	return false
}

func NewMatchTerm(term string) MatchTerm {
	return MatchTerm{
		// matching is always case insensitive
		term:              strings.ToLower(term),
		fuzzyPathMatching: strings.Contains(term, "/"),
	}
}

// split by space and lower case
func parseSearchTerm(term string) []MatchTerm {
	var terms []MatchTerm
	for _, termStr := range strings.Split(term, " ") {
		if len(termStr) == 0 {
			continue
		}
		term := NewMatchTerm(termStr)
		if matchTermInArray(term, terms) {
			continue
		}
		terms = append(terms, term)
		if len(terms) >= maxTerms {
			return terms
		}
	}
	return terms
}

func matchPathParts(pathParts, termParts []string) []MatchPos {
	var matchesBuf [8]MatchPos // perf: avoid allocation when no matches
	matches := matchesBuf[0:0]

	currTermPart := 0
	termLen := len(termParts)
	pathLen := len(pathParts)
	for currPathPart := 0; currPathPart < pathLen; currPathPart++ {
		// we match from the end
		term := termParts[termLen-currTermPart-1]
		pathEl := pathParts[pathLen-currPathPart-1]
		if idx := strings.Index(pathEl, term); idx != -1 {
			pos := idx
			for i := 0; i < pathLen-currPathPart-1; i++ {
				pos += len(pathParts[i]) + 1
			}
			currTermPart++
			matches = append(matches, MatchPos{Pos: pos, Len: len(term)})
			if currTermPart == termLen {
				// all term parts matched
				return matches
			}
		}
	}
	// we have unmatched term parts but ran out of path parts to try => no match
	return nil
}

// examples:
// "a/foo" matches "abba/foo.txt"
// "f/b/foo", "o/r/.txt", "a/oo.txt" matches "foo/bar/foo.txt"
func matchPathFuzzy(path string, term MatchTerm) []MatchPos {
	if !term.fuzzyPathMatching {
		return nil
	}
	termParts := strings.Split(term.term, "/")
	pathParts := strings.Split(path, "/")

	for len(termParts) <= len(pathParts) {
		if matches := matchPathParts(pathParts, termParts); matches != nil {
			return matches
		}
		pathParts = pathParts[:len(pathParts)-1]
	}
	return nil
}

// if regions p1 and p2 overlap, return true and a region representing merged
// p1 and p2
func mergeRegions(p1, p2 MatchPos) (bool, MatchPos) {
	if p1.Pos > p2.Pos {
		p1, p2 = p2, p1
	}
	if p1.Len+p1.Pos < p2.Pos {
		return false, MatchPos{}
	}
	pos := p1.Pos
	end := p1.Pos + p1.Len
	if p2.Pos+p2.Len > end {
		end = p2.Pos + p2.Len
	}
	return true, MatchPos{
		Pos: pos,
		Len: end - pos,
	}
}

// when match positions overlap, we want to merge them to make it easy to
// highlight matches
func mergeOverlappingRegions(a []MatchPos) []MatchPos {
	if len(a) < 2 {
		return a
	}
	// sort by position so that we only need to do a single pass
	sort.Sort(MatchPosByPosInvLen(a))
	i := 0
	for {
		didMerge, mergedPos := mergeRegions(a[i], a[i+1])
		if didMerge {
			// replace current element with merged value and delete the one after
			a[i] = mergedPos
			a = append(a[:i+1], a[i+2:]...)
		} else {
			i++
		}
		if i+1 >= len(a) {
			return a
		}
	}
}

// returns nil if no match
// our scoring is based on points:
// - file match scores higher than dir match
//   - 200 points for file match
//   - 100 points for dir match
//   - 50 points for fuzzy path match
// - we favor relatively larger file matches i.e. "foo" in "foo" scores higher
//   than "foo" in "foobar". We allocate 40 points * $matchStrength
//   where $matchStrength is 1..0, where 1 means "all letters match",
//   0.5 means "matched part is half of the matched string"
func fileNameMatch(dir, name string, terms []MatchTerm) *FileNameMatch {
	nameLower := strings.ToLower(name)
	pathLower := filepath.Join(strings.ToLower(dir), nameLower)
	var positionsBuf [maxTerms * 3]MatchPos // perf: minimize allocations for no-match cases
	positions := positionsBuf[0:0]

	posFileAdjust := 0
	if len(dir) > 0 {
		posFileAdjust = len(dir) + 1 // +1 for "/"
	}
	var score float64
	for _, term := range terms {
		termStr := term.term
		posFile := strings.Index(nameLower, termStr)
		posPath := strings.Index(pathLower, termStr)

		pathMatched := false
		// only try fuzzy match if we don't have exact match
		if posFile == -1 && posPath == -1 {
			pathMatches := matchPathFuzzy(pathLower, term)
			if pathMatches == nil {
				return nil
			}
			positions = append(positions, pathMatches...)
		}

		// if only file matches, it'll match in path as well, don't count it twice
		if posFile != -1 && posPath == posFile+posFileAdjust {
			posPath = -1
		}
		if posPath != -1 {
			score += 100
			positions = append(positions, MatchPos{Pos: posPath, Len: len(termStr)})
		}
		if posFile != -1 {
			score += 200
			scoreAdj := (40 * len(termStr)) / len(name)
			score += float64(scoreAdj)
			positions = append(positions, MatchPos{Pos: posFile + posFileAdjust, Len: len(termStr)})
		}
		if pathMatched {
			score += 50
			positions = append(positions, MatchPos{Pos: 0, Len: len(pathLower)})
		}
	}

	positions = mergeOverlappingRegions(positions)
	match := &FileNameMatch{
		FilePath: filepath.Join(dir, name),
		Score:    score,
		Matches:  positions,
	}

	return match
}

// a match can come from cache. to avoid false positives, we verify that
// matched file still exists on disk
func fileExists(path string) bool {
	_, err := os.Stat(path)
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
	dirs, err := subdirs(dir)
	if err != nil {
		return nil, err
	}

	var matches []*FileNameMatch
	for _, di := range dirs {
		for i := 0; i < di.nFiles; i++ {
			match := fileNameMatch(di.dir, di.GetFile(i), terms)
			// we might have used file names cached in memory so avoid false positives
			// by rejecting matches for files that no longer exist on disk
			if match != nil && fileExists(match.FilePath) {
				matches = append(matches, match)
			}
		}
	}
	sort.Sort(byScoreAndPath(matches))

	return matches, nil
}
