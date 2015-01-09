package finder

import (
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
		[]byte("hello world\nhehehe"), 0755); err != nil {
		t.Errorf("failed to prepare context %s", err)
	}

	f := NewFinder(root)
	results, err := f.FindContent("he*")
	if err != nil {
		t.Fatalf("failed to find content %s", err)
	}

	select {
	case r1 := <-results:
		match1 := r1.Matches[0]
		match2 := r1.Matches[1]
		if r1.File != "container.go" ||
			!(match1.Row == 1 && match1.Content == "\x1b[30;43mhe\x1b[0m\x1b[Kllo world" && match1.Count == 1) ||
			!(match2.Row == 2 && match2.Content == "\x1b[30;43mhe\x1b[0m\x1b[K\x1b[30;43mhe\x1b[0m\x1b[K\x1b[30;43mhe\x1b[0m\x1b[K" && match2.Count == 3) {
			t.Errorf("unexpected results(%s %#v %#v)", r1.File, match1, match2)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timed out")
	}
}

func TestFindFiles(t *testing.T) {
	root, err := ioutil.TempDir("/tmp", "test-find-files")
	if err != nil {
		t.Errorf("failed to create temp dir %s", err)
	}
	defer os.RemoveAll(root)

	if err := mkdir(root, []string{"daemon", "dist"}); err != nil {
		t.Errorf("failed to prepare context %s", err)
	}

	if err := touch(root, []string{"daemon/container.go", "dist/test.go"}); err != nil {
		t.Errorf("failed to prepare context %s", err)
	}

	f := NewFinder(root)
	results, err := f.FindFiles("daecon")
	if err != nil {
		t.Errorf("failed to find files %s", err)
	}

	select {
	case r1 := <-results:
		if r1 != "daemon/container.go" {
			t.Errorf("Results %#v invalid", r1)
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
