package finder

import (
	"path/filepath"
	"strings"
	"unicode"
)

type Score uint8

const (
	ScoreFuzzyPath Score = (1 << iota) // Lowest to highest score.
	ScoreFuzzyDir
	ScoreFuzzyFile
	ScoreExactFile
	ScoreExactPath
)

type Pattern struct {
	text string
}

// matchPath returns a FileMatch if the given path matches this Pattern.
func (p *Pattern) matchPath(path string) *FileMatch {
	pathLower := strings.ToLower(path)
	dirname, filename := filepath.Split(pathLower)

	caseSensitive := false
	query := strings.ToLower(p.text)

	// Compare path.
	if query == pathLower {
		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{0, len(pathLower)}},
			Score:   ScoreExactPath,
		}
	}

	// Compare filename.
	if query == filename {
		sidx := strings.Index(pathLower, filename)
		eidx := len(pathLower)

		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreExactFile,
		}
	}

	// Remove whitespace from query when doing fuzzy matching because it gets in
	// the way (e.g.. if we don't strip spaces from "model host", it will not
	// match "models/host.go").
	queryStripped := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, query)

	// Compare filename, using fuzzy matching.
	if sidx, eidx := fuzzyMatch(caseSensitive, []rune(filename), []rune(queryStripped)); sidx >= 0 {
		// Override start/end indices because we'd only used the filename.
		filenameIdx := strings.Index(pathLower, filename)
		sidx := sidx + filenameIdx
		eidx := eidx + filenameIdx

		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreFuzzyFile,
		}
	}

	// Compare directory name, using fuzzy matching.
	if sidx, eidx := fuzzyMatch(caseSensitive, []rune(dirname), []rune(queryStripped)); sidx >= 0 {
		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreFuzzyDir,
		}
	}

	// Compare path, using fuzzy matching.
	if sidx, eidx := fuzzyMatch(caseSensitive, []rune(pathLower), []rune(queryStripped)); sidx >= 0 {
		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreFuzzyPath,
		}
	}

	return nil
}

func fuzzyMatch(caseSensitive bool, runes []rune, pattern []rune) (int, int) {
	if len(pattern) == 0 {
		return 0, 0
	}

	pidx := 0
	sidx := -1
	eidx := -1

	for index, char := range runes {
		// This is considerably faster than blindly applying strings.ToLower to the
		// whole string
		if !caseSensitive {
			// Partially inlining `unicode.ToLower`. Ugly, but makes a noticeable
			// difference in CPU cost. (Measured on Go 1.4.1. Also note that the Go
			// compiler as of now does not inline non-leaf functions.)
			if char >= 'A' && char <= 'Z' {
				char += 32
			} else if char > unicode.MaxASCII {
				char = unicode.To(unicode.LowerCase, char)
			}
		}
		if char == pattern[pidx] {
			if sidx < 0 {
				sidx = index
			}
			if pidx++; pidx == len(pattern) {
				eidx = index + 1
				break
			}
		}
	}

	if sidx >= 0 && eidx >= 0 {
		pidx--
		for index := eidx - 1; index >= sidx; index-- {
			char := runes[index]
			if !caseSensitive {
				if char >= 'A' && char <= 'Z' {
					char += 32
				} else if char > unicode.MaxASCII {
					char = unicode.To(unicode.LowerCase, char)
				}
			}
			if char == pattern[pidx] {
				if pidx--; pidx < 0 {
					sidx = index
					break
				}
			}
		}
		return sidx, eidx
	}
	return -1, -1
}
