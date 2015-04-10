package finder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type File struct {
	name string
	d    string
}

func createFiles(files []File) (string, error) {
	root, err := ioutil.TempDir("/tmp", "test-find-content")
	if err != nil {
		return "", err
	}
	for _, f := range files {
		path := filepath.Join(root, f.name)
		if err := ioutil.WriteFile(path, []byte(f.d), 0755); err != nil {
			os.RemoveAll(root)
			return "", err
		}
	}
	return root, nil
}

func getAllResults(t *testing.T, results chan *SearchResult) []*SearchResult {
	var res []*SearchResult
	for {
		select {
		case r, ok := <-results:
			if !ok {
				return res
			}
			res = append(res, r)
		case <-time.After(1 * time.Second):
			t.Errorf("timed out")
		}
	}
}

func TestFindContent(t *testing.T) {
	var files = []File{
		{name: "container.go", d: "hello world\nhehe"},
	}
	root, err := createFiles(files)
	if err != nil {
		t.Errorf("failed to create files %s", err)
	}
	defer os.RemoveAll(root)

	f := NewFinder(root)
	results, err := f.FindContent("he", false, false, "")
	if err != nil {
		t.Fatalf("failed to find content %s", err)
	}

	select {
	case r := <-results:
		if r.File != "container.go" {
			t.Errorf("expected .File to be container.go but got %v", r.File)
		}

		match1 := r.Matches[0]
		expected := Line{
			Row:                1,
			Count:              1,
			Content:            "hello world",
			HighlightedContent: fmt.Sprintf("%she%sllo world", delimStart, delimEnd),
		}
		if *match1 != expected {
			t.Errorf("expected line to equal %#v but got %#v", expected, *match1)
		}

		match2 := r.Matches[1]
		expected = Line{
			Row:                2,
			Count:              2,
			Content:            "hehe",
			HighlightedContent: fmt.Sprintf("%she%s%she%s", delimStart, delimEnd, delimStart, delimEnd),
		}
		if *match2 != expected {
			t.Errorf("expected line to equal %#v but got %#v", expected, *match2)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timed out")
	}
}

func TestFindContentIgnoreCase(t *testing.T) {
	var files = []File{
		{name: "container.go", d: "Hello WORLD"},
	}
	root, err := createFiles(files)
	if err != nil {
		t.Errorf("failed to create files %s", err)
	}

	defer os.RemoveAll(root)

	f := NewFinder(root)
	ignoreCase := true
	results, err := f.FindContent("hello world", ignoreCase, false, "")
	if err != nil {
		t.Fatalf("failed to find content %s", err)
	}

	select {
	case r := <-results:
		if r.File != "container.go" {
			t.Errorf("expected .File to be container.go but got %v", r.File)
		}

		match1 := r.Matches[0]
		expected := Line{
			Row:                1,
			Count:              1,
			Content:            "Hello WORLD",
			HighlightedContent: fmt.Sprintf("%sHello WORLD%s", delimStart, delimEnd),
		}
		if *match1 != expected {
			t.Errorf("expected line to equal %#v but got %#v", expected, *match1)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timed out")
	}
}

func TestFindContentUseRegexp(t *testing.T) {
	var files = []File{
		{name: "container.go", d: "hello x 2"},
	}
	root, err := createFiles(files)
	if err != nil {
		t.Errorf("failed to create files %s", err)
	}
	defer os.RemoveAll(root)

	f := NewFinder(root)
	useRegexp := true
	results, err := f.FindContent(`x\s\d+`, false, useRegexp, "")
	if err != nil {
		t.Fatalf("failed to find content %s", err)
	}

	select {
	case r := <-results:
		if r.File != "container.go" {
			t.Errorf("expected .File to be container.go but got %v", r.File)
		}

		match1 := r.Matches[0]
		expected := Line{
			Row:                1,
			Count:              1,
			Content:            "hello x 2",
			HighlightedContent: fmt.Sprintf("hello %sx 2%s", delimStart, delimEnd),
		}
		if *match1 != expected {
			t.Errorf("expected line to equal %#v but got %#v", expected, *match1)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timed out")
	}
}

func TestFindFilePattern(t *testing.T) {
	var files = []File{
		{name: "foo.go", d: "this foo has bar"},
		{name: "foo.js", d: "this foo has bar"},
	}
	root, err := createFiles(files)
	if err != nil {
		t.Errorf("failed to create files %s", err)
	}
	defer os.RemoveAll(root)
	f := NewFinder(root)
	useRegexp := false
	// test we only find in foo.go, not foo.js
	filePattern := SanitizeFilePattern("*.go")
	chanResults, err := f.FindContent(`foo`, false, useRegexp, filePattern)
	if err != nil {
		t.Fatalf("failed to find content %s", err)
	}

	results := getAllResults(t, chanResults)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %v", len(results))
		return
	}
	r := results[0]
	if r.File != "foo.go" {
		t.Errorf("expected .File to be foo.go but got %v", r.File)
	}
	match1 := r.Matches[0]
	expected := Line{
		Row:                1,
		Count:              1,
		Content:            "this foo has bar",
		HighlightedContent: fmt.Sprintf("this %sfoo%s has bar", delimStart, delimEnd),
	}
	if *match1 != expected {
		t.Errorf("expected line to equal %#v but got %#v", expected, *match1)
	}
}

func mkdir(root string, dirs []string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			return err
		}
	}
	return nil
}
