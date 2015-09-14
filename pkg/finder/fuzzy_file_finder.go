package finder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/* The fuzzy file finder searches a directory tree for files matching a query,
 * which can be a partial name. This is similar to how ⌘P in SublimeText and
 * Atom works.
 */

// Offset holds two integers denoting the offsets of a matched substring.
type Offset [2]int

type FileMatch struct {
	Path    string
	Offsets []Offset
	Score   Score
}

func FuzzyFindFile(dir, query string) ([]*FileMatch, error) {
	dir = filepath.Clean(dir)
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dir)
	}

	paths, err := filetree(dir)
	if err != nil {
		return nil, err
	}

	pattern := &Pattern{text: strings.ToLower(query)}
	matches := search(pattern, paths)

	sort.Sort(byRelevance(matches))

	return matches, nil
}

// search searches through a haystack for file paths that match a pattern.
func search(pattern *Pattern, haystack []string) []*FileMatch {
	matches := []*FileMatch{}

	// TODO Use cache.

	for _, path := range haystack {
		// TODO This can be optimized by running multiple matches in parallel.
		match := pattern.matchPath(path)
		if match != nil {
			matches = append(matches, match)
		}
	}

	return matches
}

// byRelevance implements sort.Interface so that we can sort a FileMatch slice.
// Matches are sorted, in decreasing priority, by:
// 1. Score
// 2. Length of offset
// 3. Tree depth e.g. sort "foo/host.go" before "foo/bar/host.go"
// 4. Path (alphabetically)
type byRelevance []*FileMatch

func (b byRelevance) Len() int {
	return len(b)
}

func (b byRelevance) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byRelevance) Less(i, j int) bool {
	iMatch, jMatch := b[i], b[j]

	if iMatch.Score > jMatch.Score {
		return true
	}
	if iMatch.Score < jMatch.Score {
		return false
	}

	// Use offsets as the 1st tiebreaker.
	// We just assume there is a single pair of offsets for now, since none of our
	// string search algorithms return > 1 offset pair.
	iMatchlen := iMatch.Offsets[0][1] - iMatch.Offsets[0][0]
	jMatchlen := jMatch.Offsets[0][1] - jMatch.Offsets[0][0]
	if iMatchlen < jMatchlen {
		return true
	}
	if iMatchlen > jMatchlen {
		return false
	}

	// Prefer offsets that are nearer the end of string, e.g. sort
	// "pkg/dockerclient/docker/pkg.a" before "pkg/dockerclient/doc.a".
	iDistFromEnd := len(iMatch.Path) - iMatch.Offsets[0][1]
	jDistFromEnd := len(jMatch.Path) - jMatch.Offsets[0][1]
	if iDistFromEnd < jDistFromEnd {
		return true
	}
	if iDistFromEnd > jDistFromEnd {
		return false
	}

	// Use tree depth of file as 3rd tiebreaker, e.g. sort "foo/host.go" before
	// "foo/bar/host.go".
	iDepth := len(strings.Split(iMatch.Path, string(filepath.Separator)))
	jDepth := len(strings.Split(jMatch.Path, string(filepath.Separator)))
	if iDepth < jDepth {
		return true
	}
	if iDepth > jDepth {
		return false
	}

	// Sort alphabetically if everything else is equal.
	return iMatch.Path < jMatch.Path
}
