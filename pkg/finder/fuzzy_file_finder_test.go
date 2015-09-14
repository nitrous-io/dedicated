package finder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	testDir   string
	testFiles []string
)

// Perform setup and teardown.
func TestMain(m *testing.M) {
	var err error

	testFiles = []string{
		"README.md",
		"controllers/containers/containers.go",
		"controllers/containers/containers_test.go",
		"models/container.go",
		"models/workspace.go",
		"models/config.json",
		"dist/server",
		"pkg/docker/a.out",
		"pkg/dockerclient/base.a",
		"pkg/dockerclient/docker.a",
		".git/HEAD",
	}
	testDir, err = ioutil.TempDir("/tmp", "test-fuzzy-file-finder")
	if err != nil {
		panic(err)
	}

	for _, p := range testFiles {
		path := filepath.Join(testDir, p)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(err)
		}
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		f.Close()
	}

	exitCode := m.Run()
	os.RemoveAll(testDir)
	os.Exit(exitCode)
}

func assertExpectedMatches(t *testing.T, dir, query string, expectedMatches []FileMatch) {
	matches, err := FuzzyFindFile(dir, query)
	if err != nil {
		t.Fatalf("FuzzyFindFile(%q) failed with %v", query, err)
	}

	if len(matches) != len(expectedMatches) {
		t.Fatalf("FuzzyFindFile(%q): got %d results; expected %#v but got %#v",
			query, len(matches), expectedMatches, derefMatches(matches))
	}

	for i, exp := range expectedMatches {
		got := matches[i]

		if got.Path != exp.Path {
			t.Fatalf("FuzzyFindFile(%q): expected result %d to have Path %q, but got %q; expected matches: %#v, got %#v",
				query, i, exp.Path, got.Path, expectedMatches, derefMatches(matches))
		}

		if exp.Offsets != nil && !reflect.DeepEqual(exp.Offsets, got.Offsets) {
			t.Fatalf("FuzzyFindFile(%q): expected offsets of %#v to be %v but got %v; expected matches: %#v, got %#v",
				query, got, exp.Offsets, got.Offsets, expectedMatches, derefMatches(matches))
		}

		if exp.Score != 0 && got.Score != exp.Score {
			t.Fatalf("FuzzyFindFile(%q): expected score of %#v to be %d but got %d; expected matches: %#v, got %#v",
				query, got, exp.Score, got.Score, expectedMatches, derefMatches(matches))
		}
	}
}

var fuzzyFindTestCases = []struct {
	query    string
	expected []FileMatch
}{

	// Exact path matches.
	{
		query: "README.md",
		expected: []FileMatch{
			{
				Path:    "README.md",
				Offsets: []Offset{{0, len("README.md")}},
				Score:   ScoreExactPath,
			},
		},
	},
	{
		query: "models/container.go",
		expected: []FileMatch{
			{
				Path:    "models/container.go",
				Offsets: []Offset{{0, len("models/container.go")}},
				Score:   ScoreExactPath,
			},
		},
	},

	// Exact filename matches.
	{
		query: "containers.go",
		expected: []FileMatch{
			{
				Path:    "controllers/containers/containers.go",
				Offsets: []Offset{{23, len("controllers/containers/containers.go")}},
				Score:   ScoreExactFile,
			},
			{
				Path:    "controllers/containers/containers_test.go",
				Offsets: []Offset{{23, len("controllers/containers/containers_test.go")}},
				Score:   ScoreFuzzyFile,
			},
		},
	},

	// Fuzzy matching of filename.
	{
		query: "containersgo",
		expected: []FileMatch{
			{
				Path:  "controllers/containers/containers.go",
				Score: ScoreFuzzyFile,
			},
			{
				Path:  "controllers/containers/containers_test.go",
				Score: ScoreFuzzyFile,
			},
		},
	},

	// Fuzzy matching of directory.
	{
		query: "cntrl",
		expected: []FileMatch{
			{
				Path:  "controllers/containers/containers.go",
				Score: ScoreFuzzyPath,
			},
			{
				Path:  "controllers/containers/containers_test.go",
				Score: ScoreFuzzyPath,
			},
		},
	},

	// Matching directory.
	{
		query: "models",
		expected: []FileMatch{
			{
				Path:  "models/workspace.go",
				Score: ScoreFuzzyPartialFile,
			},
			{
				Path:  "models/config.json",
				Score: ScoreFuzzyPartialFile,
			},
			{
				Path:  "models/container.go",
				Score: ScoreFuzzyPath,
			},
		},
	},

	{
		query: "models/", // Trailing slash.
		expected: []FileMatch{
			{
				Path:  "models/config.json",
				Score: ScoreFuzzyPath,
			},
			{
				Path:  "models/container.go",
				Score: ScoreFuzzyPath,
			},
			{
				Path:  "models/workspace.go",
				Score: ScoreFuzzyPath,
			},
		},
	},

	// Fuzzy matching of full path.
	{
		query: "cccontest",
		expected: []FileMatch{
			{
				Path:  "controllers/containers/containers_test.go",
				Score: ScoreFuzzyPartialFile,
			},
		},
	},

	// Fuzzy matching of full path, with backward scanning.
	{
		query: "pkgdocker",
		expected: []FileMatch{
			{
				Path:    "pkg/dockerclient/docker.a",
				Offsets: []Offset{{0, 23}},
				Score:   ScoreFuzzyPartialFile,
			},
			{
				Path:    "pkg/docker/a.out",
				Offsets: []Offset{{0, 10}},
				Score:   ScoreFuzzyPath,
			},
			{
				Path:    "pkg/dockerclient/base.a",
				Offsets: []Offset{{0, 10}},
				Score:   ScoreFuzzyPath,
			},
		},
	},

	// Fuzzy matching of full path with separating slashes.
	{
		query: "c/c/containtest",
		expected: []FileMatch{
			{
				Path:  "controllers/containers/containers_test.go",
				Score: ScoreFuzzyPartialFile,
			},
		},
	},

	// Matching of file extensions.
	{
		query: ".go",
		expected: []FileMatch{
			{
				// Files in models/ sort above files in controllers/containers because
				// the filetree depth is shallower.
				Path:    "models/container.go",
				Score:   ScoreFuzzyFile,
				Offsets: []Offset{{16, len("models/container.go")}},
			},
			{
				Path:    "models/workspace.go",
				Score:   ScoreFuzzyFile,
				Offsets: []Offset{{16, len("models/workspace.go")}},
			},
			{
				Path:    "controllers/containers/containers.go",
				Score:   ScoreFuzzyFile,
				Offsets: []Offset{{33, len("controllers/containers/containers.go")}},
			},
			{
				Path:    "controllers/containers/containers_test.go",
				Score:   ScoreFuzzyFile,
				Offsets: []Offset{{38, len("controllers/containers/containers_test.go")}},
			},
		},
	},

	// Spaces should be ignored.
	{
		query: "model workspace",
		expected: []FileMatch{
			{
				Path:  "models/workspace.go",
				Score: ScoreFuzzyPartialFile,
			},
		},
	},

	// Nothing matches.
	{
		query:    "does/not/exist",
		expected: []FileMatch{},
	},
}

func TestFuzzyFindFile(t *testing.T) {
	for _, tc := range fuzzyFindTestCases {
		assertExpectedMatches(t, testDir, tc.query, tc.expected)
	}
}

// Test that FuzzyFindFile() works the same when given a symlink to a directory.
func TestFuzzyFindFileInSymlinkDir(t *testing.T) {
	symlinkPath := filepath.Join("/tmp", "a-symlink")
	if err := os.Symlink(testDir, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink to %s at %s, err: %v", testDir, symlinkPath, err)
	}

	for _, tc := range fuzzyFindTestCases {
		assertExpectedMatches(t, symlinkPath, tc.query, tc.expected)
	}

	if err := os.Remove(symlinkPath); err != nil {
		t.Errorf("failed to cleanup symlink at %s, err: %v", symlinkPath, err)
	}
}

// derefMatches dereferences pointers in a []*FileMatch slice so that we get
// helpful error messages when printing.
func derefMatches(matches []*FileMatch) []FileMatch {
	ret := []FileMatch{}
	for _, m := range matches {
		ret = append(ret, *m)
	}
	return ret
}
