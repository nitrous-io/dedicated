package finder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFiletree(t *testing.T) {
	paths, err := filetree(testDir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != len(testFiles) {
		t.Fatalf("filetree(%q): expected %d paths but got %d", testDir, len(testFiles), len(paths))
	}

	dir := filepath.Join(testDir, "dist")
	paths, err = filetree(dir)
	if err != nil {
		t.Fatalf("filetree(%q): unexpected err %v", dir, err)
	}
	if len(paths) != 1 {
		t.Fatalf("filetree(%q): expected 1 path in %s but got %d", dir, dir, len(paths))
	}
	// Returned paths are relative to directory given to filetree().
	if paths[0] != "server" {
		t.Fatalf("filetree(%q): expected %v to contain %q, but it didn't", dir, paths, "server")
	}
}

func TestFiletreeInvalidateCache(t *testing.T) {
	paths, err := filetree(testDir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != len(testFiles) {
		t.Fatalf("filetree(%q): expected %d paths but got %d", testDir, len(testFiles), len(paths))
	}

	// Add file to a subdirectory, so we can verify that it is returned (instead
	// of being masked by the cache).
	newfile := filepath.Join(testDir, "dist", "untitled.txt")
	f, err := os.Create(newfile)
	if err != nil {
		t.Fatalf("filetree(%q): unexpected err creating new file %q, err: %v", testDir, newfile, err)
	}
	f.Close()

	// Give time for cache to be invalidated, since we use inotify.
	time.Sleep(100 * time.Millisecond)

	paths, err = filetree(testDir)
	if err != nil {
		t.Fatalf("filetree(%q): unexpected err %v", testDir, err)
	}
	if len(paths) != len(testFiles)+1 {
		t.Fatalf("filetree(%q): expected %d paths but got %d", testDir, len(testFiles)+1, len(paths))
	}
}
