package finder

import (
	"reflect"
	"testing"
)

// 	ScoreFuzzyPath Score = (1 << iota) // Lowest to highest score.
// 	ScoreFuzzyDir
// 	ScoreFuzzyFile
// 	ScoreExactFile
// 	ScoreExactPath

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
			pattern: Pattern{text: "mdls"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{0, 6}},
				Score:   ScoreFuzzyDir,
			},
		},
		{
			pattern: Pattern{text: "mdcont"},
			path:    "models/containers.rb",
			expected: FileMatch{
				Path:    "models/containers.rb",
				Offsets: []Offset{{0, 11}},
				Score:   ScoreFuzzyPath,
			},
		},
	}

	for _, tc := range testCases {
		got := tc.pattern.matchPath(tc.path)

		if !reflect.DeepEqual(tc.expected, *got) {
			t.Fatalf("matchPath(): expected %#v but got %#v", tc.expected, *got)
		}
	}

	p := Pattern{text: "wont-match"}
	match := p.matchPath("models/containers.rb")
	if match != nil {
		t.Fatalf("matchPath(): expected nil got %#v", match)
	}
}
