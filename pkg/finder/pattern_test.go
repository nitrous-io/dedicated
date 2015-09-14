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

func TestFuzzyMatch(t *testing.T) {
	testCases := []struct {
		pattern  string
		text     string
		forward  bool
		expected []int
	}{
		{
			pattern:  "dock",
			text:     "dockerclient/server/docker.go",
			forward:  true,
			expected: []int{0, 4},
		},
		{
			pattern:  "dock",
			text:     "dockerclient/server/docker.go",
			forward:  false,
			expected: []int{20, 24},
		},
		{
			pattern:  "apprb",
			text:     "application_controller.rb",
			forward:  true,
			expected: []int{0, len("application_controller.rb")},
		},
		{
			pattern:  "controller",
			text:     "application_controller.rb",
			forward:  true,
			expected: []int{12, 22},
		},
		{
			pattern:  "homecon",
			text:     "home_controller.rb",
			forward:  true,
			expected: []int{0, 8},
		},
	}

	for _, tc := range testCases {
		sidx, eidx := fuzzyMatch(tc.forward, []rune(tc.text), []rune(tc.pattern))

		if sidx != tc.expected[0] {
			t.Fatalf("fuzzyMatch(%v, %s, %s): expected start index of %d but got %d",
				tc.forward, tc.text, tc.pattern, tc.expected[0], sidx)
		}

		if eidx != tc.expected[1] {
			t.Fatalf("fuzzyMatch(%v, %s, %s): expected end index of %d but got %d",
				tc.forward, tc.text, tc.pattern, tc.expected[1], eidx)
		}
	}
}
