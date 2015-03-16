package finder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// expResults must be ordered by score
func testMatches(t *testing.T, root, term string, expResults []string) {
	res, err := FindFiles(root, term)
	if err != nil {
		t.Errorf("FindFiles('%s') failed with %s", term, err)
		return
	}
	if len(res) != len(expResults) {
		t.Errorf("FindFile('%s'): got %d results, expected %d, results: %#v", term, len(res), len(expResults), res)
		return
	}
	for i := 0; i < len(res); i++ {
		got := res[i].FilePath
		exp := filepath.Join(root, expResults[i])
		if exp != got {
			t.Errorf("FindFile('%s'): result %d is '%s', expected: '%s'", term, i, got, exp)
			return
		}
	}
}

func verifyCachedDir(t *testing.T, di *DirInfo) {
	if di.nDirs != 0 {
		t.Errorf("expected 0 dirs, got %d", di.nDirs)
	}
	n := di.nFiles
	if n != 2 {
		t.Errorf("expected 2 files, got %d", di.nFiles)
	}
	files := make([]string, n, n)
	for i := 0; i < n; i++ {
		files[i] = di.GetFile(i)
	}
	sort.Strings(files)
	if files[0] != "nomatching" {
		t.Errorf("expected '%s' at index 0, got '%s'", "nomatching", files[0])
	}
	if files[1] != "test.go" {
		t.Errorf("expected '%s' at index 1, got '%s'", "test.go", files[1])
	}
}

// it's simpler to re-use dir setup this way
func testCachedDir(t *testing.T, root string) {
	path := filepath.Join(root, "dist")
	di1, err := getCachedDir(path)
	if err != nil {
		t.Errorf("getCachedDir('%s') failed with '%s'", path, err)
	}
	verifyCachedDir(t, di1)
	// now should come from cache, so should be same as di2
	di2, err := getCachedDir(path)
	if err != nil {
		t.Errorf("getCachedDir('%s') failed with '%s'", path, err)
	}
	if di1 != di2 {
		t.Errorf("di1 != di2 (%p != %p)", di1, di2)
	}
}

func TestFindFiles(t *testing.T) {
	files := []string{
		"daemon/container.go",
		"dist/test.go",
		"dist/nomatching",
		"foo.txt",
		".svn/foo.txt",
		"bar/foo/xyz.go",
		"GLAM.cpp",
	}
	root, err := ioutil.TempDir("/tmp", "test-find-file")
	if err != nil {
		t.Errorf("failed to create temp dir %s", err)
	}

	defer os.RemoveAll(root)
	for _, partPath := range files {
		path := filepath.Join(root, partPath)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Errorf("failed to create dir %s, err: %s", dir, err)
			return
		}
		f, err := os.Create(path)
		if err != nil {
			t.Errorf("failed to create file %s, err: %s", path, err)
			return
		}
		f.Close()
	}

	testMatches(t, root, "st/no", []string{"dist/nomatching"})
	testMatches(t, root, "glam", []string{"GLAM.cpp"})
	testMatches(t, root, "daem", []string{"daemon/container.go"})
	testMatches(t, root, "DAem", []string{"daemon/container.go"})
	testMatches(t, root, "daem cont", []string{"daemon/container.go"})
	testMatches(t, root, "foo", []string{"foo.txt", "bar/foo/xyz.go"})
	testMatches(t, root, "foo y", []string{"bar/foo/xyz.go"})
	testMatches(t, root, ".go", []string{"bar/foo/xyz.go", "dist/test.go", "daemon/container.go"})
	testMatches(t, root, "daa", []string{})
	testMatches(t, root, "bar xyz lo", []string{})
	testCachedDir(t, root)
}
