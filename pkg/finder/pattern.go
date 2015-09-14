package finder

import (
	"path/filepath"
	"strings"
	"unicode"
)

type Score uint8

const (
	ScoreFuzzyPath Score = (1 << iota) // Lowest to highest score.
	ScoreFuzzyPartialFile
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
	filename := filepath.Base(pathLower)

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
	forward, backward := true, false
	greedy := true
	if sidx, eidx := fuzzyMatch(forward, !greedy, []rune(filename), []rune(queryStripped)); sidx >= 0 {
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

	// Compare path, using fuzzy matching, greedy.
	// E.g. "libdocker" will match "lib/docker/docker.go" at "lib/docker/docker".
	if sidx, eidx := fuzzyMatch(backward, greedy, []rune(pathLower), []rune(queryStripped)); sidx >= 0 {
		// If match part of filename.
		filenameIdx := strings.Index(pathLower, filename)
		if eidx > filenameIdx {
			return &FileMatch{
				Path:    path,
				Offsets: []Offset{{sidx, eidx}},
				Score:   ScoreFuzzyPartialFile,
			}
		}
	}

	// Compare path, using fuzzy match, non-greedy.
	// E.g. "libdocker" will match "lib/docker/docker.go" at "lib/docker".
	if sidx, eidx := fuzzyMatch(backward, !greedy, []rune(pathLower), []rune(queryStripped)); sidx >= 0 {
		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreFuzzyPath,
		}
	}

	return nil
}

func runeAt(runes []rune, index int, max int, forward bool) rune {
	if forward {
		return runes[index]
	}
	return runes[max-index-1]
}

func fuzzyMatch(forward, greedy bool, runes []rune, pattern []rune) (int, int) {
	if len(pattern) == 0 {
		return 0, 0
	}

	pidx := 0
	sidx := -1
	eidx := -1

	lenRunes := len(runes)
	lenPattern := len(pattern)

	for index := range runes {
		char := runeAt(runes, index, lenRunes, forward)
		pchar := runeAt(pattern, pidx, lenPattern, forward)

		if char == pchar {
			if sidx < 0 {
				sidx = index
			}
			if pidx++; pidx == lenPattern {
				eidx = index + 1
				break
			}
		}
	}

	if sidx >= 0 && eidx >= 0 {
		// Try to shorten match.
		if !greedy {
			pidx--
			for index := eidx - 1; index >= sidx; index-- {
				char := runeAt(runes, index, lenRunes, forward)
				pchar := runeAt(pattern, pidx, lenPattern, forward)

				if char == pchar {
					if pidx--; pidx < 0 {
						sidx = index
						break
					}
				}
			}
		}

		if forward {
			return sidx, eidx
		}
		return lenRunes - eidx, lenRunes - sidx
	}
	return -1, -1
}
