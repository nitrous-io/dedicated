/*
finder handle quick-finding a file within a path
*/
package finder

import (
	"path/filepath"
	"strings"

	pt "github.com/monochromegane/the_platinum_searcher"
)

var (
	chanSize = 10
	maxDepth = 10
)

type Finder struct {
	root string
}

// Line represents a matched line in search response
type Line struct {
	Row                int    `json:"row"`
	Content            string `json:"content"`
	HighlightedContent string `json:"highlighted_content"`
	Count              int    `json:"count"` // Number of matches on this line
}

// SearchResults represent the full results for a single file
type SearchResult struct {
	File    string  `json:"file"`
	Matches []*Line `json:"matches"`
}

// NewFinder initializes a new finder inside root directory
func NewFinder(root string) *Finder {
	return &Finder{root}
}

// FindContent finds files which content has the pattern
//
// chan *SearchResult has to be fully read even if we dont consume it
// completely
func (f *Finder) FindContent(term string, ignoreCase, useRegexp bool) (chan *SearchResult, error) {
	smartCase := true
	if ignoreCase {
		smartCase = false
	}

	pattern, err := pt.NewPattern(term, "", smartCase, ignoreCase, useRegexp)
	if err != nil {
		return nil, err
	}

	out := make(chan *pt.PrintParams, chanSize)
	results := make(chan *SearchResult, chanSize)
	go func() {
		for pp := range out {
			if len(pp.Matches) == 0 {
				continue
			}
			rel, err := filepath.Rel(f.root, pp.Path)
			if err != nil {
				continue
			}
			r := &SearchResult{File: rel}
			for _, m := range pp.Matches {
				line := &Line{
					Row:                m.Num,
					Content:            m.Str,
					HighlightedContent: highlightMatches(pattern, m.Str),
					Count:              countMatches(pattern, m.Str),
				}
				r.Matches = append(r.Matches, line)
			}
			results <- r
		}
		close(results)
	}()

	fch := make(chan *pt.GrepParams, chanSize)
	option := defaultOption()
	go pt.Find(f.root, pattern, fch, option)
	go pt.Grep(fch, out, option)

	return results, nil
}

const (
	delimStart = "\x1b[30;43m"
	delimEnd   = "\x1b[0m\x1b[K"
)

// We should probably return the delimiters used or let the caller specify them.
func highlightMatches(pattern *pt.Pattern, str string) string {
	if pattern.UseRegexp || pattern.IgnoreCase {
		return pattern.Regexp.ReplaceAllString(str, delimStart+"${1}"+delimEnd)
	}
	return strings.Replace(str, pattern.Pattern, delimStart+pattern.Pattern+delimEnd, -1)
}

func countMatches(pattern *pt.Pattern, str string) int {
	if pattern.UseRegexp || pattern.IgnoreCase {
		return len(pattern.Regexp.FindAllString(str, -1))
	}
	return strings.Count(str, pattern.Pattern)
}

// FindFiles finds files which name has the pattern
//
// the response channel has to be fully read even if we dont consume it
// completely
func (f *Finder) FindFiles(term string) (chan string, error) {
	term = regexTerm(term)
	pattern, err := pt.NewPattern(term, term, true, false, false)
	if err != nil {
		return nil, err
	}

	out := make(chan *pt.GrepParams, chanSize)
	results := make(chan string, chanSize)
	go func() {
		for val := range out {
			if val == nil {
				continue
			}
			rel, err := filepath.Rel(f.root, val.Path)
			if err != nil {
				continue
			}
			results <- rel
		}
		close(results)
	}()

	option := defaultOption()
	go pt.Find(f.root, pattern, out, option)

	return results, nil
}

// regexTerm splits the term characters and join them with .*? so it behaves
// like a quick-open/smart regex for searching filenames
func regexTerm(term string) string {
	chars := strings.Split(term, "")
	return ".*?" + strings.Join(chars, ".*?") + ".*?"
}

func defaultOption() *pt.Option {
	return &pt.Option{
		VcsIgnore: []string{".gitignore", ".hgignore", ".ptignore"},
		Depth:     maxDepth,
		NoGroup:   true,
		Regexp:    true,
		Follow:    true, // follow symlink
		Proc:      chanSize,
		// do not use global git ignore as this's not running inside user context
		NoPtIgnore:        true,
		NoGlobalGitIgnore: true,
	}
}
