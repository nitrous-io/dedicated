package finder

import (
	"reflect"
	"testing"
)

func TestMatchPath(t *testing.T) {
	testCases := []struct {
		pattern  Pattern
		path     string
		expected FileMatch
	}{
		{
			pattern: Pattern{text: "models/containers.rb"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{0, len("models/containers.rb")}},
				Score:   ScoreExactPath,
			},
		},
		{
			pattern: Pattern{text: "containers.rb"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{7, len("models/containers.rb")}},
				Score:   ScoreExactFile,
			},
		},
		{
			pattern: Pattern{text: "containers"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{7, 17}},
				Score:   ScoreFuzzyFile,
			},
		},
		{
			pattern: Pattern{text: "mdcont"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{0, 11}},
				Score:   ScoreFuzzyPartialFile,
			},
		},
		{
			pattern: Pattern{text: "libdocker"},
			path:    "lib/docker/docker.go",
			expected: FileMatch{
				Path:    "lib/docker/docker.go",
				Offsets: []Offset{{0, 17}},
				Score:   ScoreFuzzyPartialFile,
			},
		},
		{
			pattern: Pattern{text: "mdls/"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{0, 7}},
				Score:   ScoreFuzzyPath,
			},
		},
	}

	for _, tc := range testCases {
		got := tc.pattern.matchPath(tc.path)

		if !reflect.DeepEqual(tc.expected, *got) {
			t.Fatalf("%#v.matchPath(%q): expected %#v but got %#v",
				tc.pattern, tc.path, tc.expected, *got)
		}
	}

	p := Pattern{text: "wont-match"}
	match := p.matchPath("models/containers.rb")
	if match != nil {
		t.Fatalf("matchPath(): expected nil got %#v", match)
	}
}

func TestFuzzyMatch(t *testing.T) {
	text := "dockerclient/server/docker.go"
	pattern := "dock"
	forward, backward := true, false
	greedy := true

	sidx, eidx := fuzzyMatch(forward, !greedy, []rune(text), []rune(pattern))
	if sidx != 0 {
		t.Fatalf("fuzzyMatch(%v, %s, %s): expected start index of %d but got %d", forward, text, pattern, 0, sidx)
	}
	if eidx != 4 {
		t.Fatalf("fuzzyMatch(%v, %s, %s): expected end index of %d but got %d", forward, text, pattern, 4, eidx)
	}

	sidx, eidx = fuzzyMatch(backward, !greedy, []rune(text), []rune(pattern))
	if sidx != 20 {
		t.Fatalf("fuzzyMatch(%v, %s, %s): expected start index of %d but got %d", forward, text, pattern, 20, sidx)
	}
	if eidx != 24 {
		t.Fatalf("fuzzyMatch(%v, %s, %s): expected end index of %d but got %d", forward, text, pattern, 24, eidx)
	}
}
