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
	if sidx, eidx := fuzzyMatch(forward, []rune(filename), []rune(queryStripped)); sidx >= 0 {
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
	if sidx, eidx := fuzzyMatch(backward, []rune(dirname), []rune(queryStripped)); sidx >= 0 {
		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreFuzzyDir,
		}
	}

	// Compare path, using fuzzy matching.
	if sidx, eidx := fuzzyMatch(backward, []rune(pathLower), []rune(queryStripped)); sidx >= 0 {
		return &FileMatch{
			Path:    path,
			Offsets: []Offset{{sidx, eidx}},
			Score:   ScoreFuzzyPath,
		}
	}

	return nil
}

func fuzzyMatch(forward bool, runes []rune, pattern []rune) (int, int) {
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

		if forward {
			return sidx, eidx
		}
		return lenRunes - eidx, lenRunes - sidx
	}
	return -1, -1
}

func runeAt(runes []rune, index int, max int, forward bool) rune {
	if forward {
		return runes[index]
	}
	return runes[max-index-1]
}
