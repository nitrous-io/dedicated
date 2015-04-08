package finder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindContent(t *testing.T) {
	root, err := ioutil.TempDir("/tmp", "test-find-content")
	if err != nil {
		t.Errorf("failed to create temp dir %s", err)
	}
	defer os.RemoveAll(root)

	if err := ioutil.WriteFile(filepath.Join(root, "container.go"),
		[]byte("hello world\nhehe"), 0755); err != nil {
		t.Errorf("failed to prepare context %s", err)
	}

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
	root, err := ioutil.TempDir("/tmp", "test-find-content")
	if err != nil {
		t.Errorf("failed to create temp dir %s", err)
	}
	defer os.RemoveAll(root)

	if err := ioutil.WriteFile(filepath.Join(root, "container.go"),
		[]byte("Hello WORLD"), 0755); err != nil {
		t.Errorf("failed to prepare context %s", err)
	}

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
	root, err := ioutil.TempDir("/tmp", "test-find-content")
	if err != nil {
		t.Errorf("failed to create temp dir %s", err)
	}
	defer os.RemoveAll(root)

	if err := ioutil.WriteFile(filepath.Join(root, "container.go"),
		[]byte("hello x 2"), 0755); err != nil {
		t.Errorf("failed to prepare context %s", err)
	}

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

func mkdir(root string, dirs []string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			return err
		}
	}
	return nil
}

func touch(root string, files []string) error {
	for _, name := range files {
		f, err := os.Create(filepath.Join(root, name))
		if err != nil {
			return err
		}
		f.Close()
	}
	return nil
}
