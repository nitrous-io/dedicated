package finder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFiletree(t *testing.T) {
	files := []string{"README.md", "dist/server"}
	dir, err := setupTestDir(files...)
	if err != nil {
		t.Fatalf("filetree(): error setting up directory for test")
	}
	defer os.RemoveAll(dir)

	paths, err := filetree(dir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != len(files) {
		t.Fatalf("filetree(%q): expected %d paths but got %d", dir, len(files), len(paths))
	}

	subdir := filepath.Join(dir, "dist")
	paths, err = filetree(subdir)
	if err != nil {
		t.Fatalf("filetree(%q): unexpected err %v", subdir, err)
	}
	if len(paths) != 1 {
		t.Fatalf("filetree(%q): expected 1 path in %s but got %d", subdir, subdir, len(paths))
	}
	// Returned paths are relative to directory given to filetree().
	if paths[0] != "server" {
		t.Fatalf("filetree(%q): expected %v to contain %q, but it didn't", subdir, paths, "server")
	}
}

func TestFiletreeNewFileInvalidatesCache(t *testing.T) {
	files := []string{"README.md", "dist/server"}
	dir, err := setupTestDir(files...)
	if err != nil {
		t.Fatalf("filetree(): error setting up directory for test")
	}
	defer os.RemoveAll(dir)

	paths, err := filetree(dir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != len(files) {
		t.Fatalf("filetree(%q): expected %d paths but got %d", dir, len(files), len(paths))
	}

	// Add file to a subdirectory, so we can verify that it is returned (instead
	// of being masked by the cache).
	newfile := filepath.Join(dir, "dist", "untitled.txt")
	f, err := os.Create(newfile)
	if err != nil {
		t.Fatalf("filetree(%q): unexpected err creating new file %q, err: %v", dir, newfile, err)
	}
	f.Close()

	// Give time for cache to be invalidated, since we use inotify.
	time.Sleep(100 * time.Millisecond)

	paths, err = filetree(dir)
	if err != nil {
		t.Fatalf("filetree(%q): unexpected err %v", dir, err)
	}
	if len(paths) != len(files)+1 {
		t.Fatalf("filetree(%q): expected %d paths but got %d", dir, len(files)+1, len(paths))
	}
}

func TestFiletreeDeleteFileInvalidatesCache(t *testing.T) {
	files := []string{"README.md", "dist/server"}
	dir, err := setupTestDir(files...)
	if err != nil {
		t.Fatalf("filetree(): error setting up directory for test")
	}
	defer os.RemoveAll(dir)

	paths, err := filetree(dir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != len(files) {
		t.Fatalf("filetree(%q): expected %d paths but got %d", dir, len(files), len(paths))
	}

	// Remove file so that we can verify that it is no longer returned.
	serverFile := filepath.Join(dir, "dist", "server")
	if err := os.Remove(serverFile); err != nil {
		t.Fatalf("filetree(): unexpected err removing file %q, err: %v", serverFile, err)
	}

	// Give time for cache to be invalidated, since we use inotify.
	time.Sleep(100 * time.Millisecond)

	paths, err = filetree(dir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != len(files)-1 {
		t.Fatalf("filetree(%q): expected %d paths but got %d", dir, len(files)-1, len(paths))
	}
}

func TestFiletreeRenameFileInvalidatesCache(t *testing.T) {
	files := []string{"dist/server"}
	dir, err := setupTestDir(files...)
	if err != nil {
		t.Fatalf("filetree(): error setting up directory for test")
	}
	defer os.RemoveAll(dir)

	paths, err := filetree(dir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("filetree(%q): expected 1 path in %s but got %d", dir, dir, len(paths))
	}
	if paths[0] != "dist/server" {
		t.Fatalf("filetree(%q): expected %v to contain %q, but it didn't", dir, paths, "dist/server")
	}

	// Rename file so that we can verify that we return it with its new name.
	serverFile := filepath.Join(dir, "dist", "server")
	newFile := filepath.Join(dir, "dist", "release")
	if err := os.Rename(serverFile, newFile); err != nil {
		t.Fatalf("filetree(): unexpected err removing file %q, err: %v", serverFile, err)
	}

	// Give time for cache to be invalidated, since we use inotify.
	time.Sleep(100 * time.Millisecond)

	paths, err = filetree(dir)
	if err != nil {
		t.Fatalf("filetree(): unexpected err %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("filetree(%q): expected 1 path in %s but got %d", dir, dir, len(paths))
	}
	if paths[0] != "dist/release" {
		t.Fatalf("filetree(%q): expected %v to contain %q, but it didn't", dir, paths, "dist/release")
	}
}
