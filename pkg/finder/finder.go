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
	Row     int    `json:"row"`
	Content string `json:"content"`
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

// FindPattern finds files which content has the pattern
//
// chan *ContentResults has to be fully read even if we dont consume it
// completely
func (f *Finder) FindContent(term string) (chan *SearchResult, error) {
	pattern, err := pt.NewPattern(term, "", true, false, true)
	if err != nil {
		return nil, err
	}
	option := defaultOption()

	fch := make(chan *pt.GrepParams, chanSize)
	out := make(chan *pt.PrintParams, chanSize)
	results := make(chan *SearchResult, chanSize)
	go func() {
		for val := range out {
			rel, err := filepath.Rel(f.root, val.Path)
			if err != nil {
				continue
			}
			r := &SearchResult{File: rel}
			for _, m := range val.Matches {
				r.Matches = append(r.Matches, &Line{Row: m.Num, Content: m.Str})
			}
			results <- r
		}
		close(results)
	}()

	go pt.Find(f.root, pattern, fch, option)
	go pt.Grep(fch, out, option)

	return results, nil
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
	option := defaultOption()

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
