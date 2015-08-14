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
	testMatches(t, root, "b/o/.go", []string{"bar/foo/xyz.go"})
	testMatches(t, root, "a/yz", []string{"bar/foo/xyz.go"})
	testMatches(t, root, "r/oo/yz", []string{"bar/foo/xyz.go"})
	testMatches(t, root, "r/oo", []string{"bar/foo/xyz.go"})
	testMatches(t, root, "daa", []string{})
	testMatches(t, root, "bar xyz lo", []string{})
	testCachedDir(t, root)

	// Test that it works the same when given a symlink to a directory.
	symlinkPath := filepath.Join("/tmp", "a-symlink")
	if err := os.Symlink(root, symlinkPath); err != nil {
		t.Errorf("failed to create symlink to %s at %s, err: %v", root, symlinkPath, err)
	}

	testMatches(t, symlinkPath, "st/no", []string{"dist/nomatching"})
	testMatches(t, root, "glam", []string{"GLAM.cpp"})
	testMatches(t, root, "daem", []string{"daemon/container.go"})
	testMatches(t, root, "DAem", []string{"daemon/container.go"})
	testCachedDir(t, symlinkPath)

	if err := os.Remove(symlinkPath); err != nil {
		t.Errorf("failed to cleanup symlink at %s, err: %v", symlinkPath, err)
	}
}

func TestMergeRegions(t *testing.T) {
	// each test case is 6 numbers, 2 numbers per MatchPos specifying (pos, len)
	// first 2 MatchPos are args to mergeRegions(), 3rd is expected result
	merged := []int{
		0, 1, 0, 3, 0, 3,
		0, 3, 0, 1, 0, 3,
		1, 8, 3, 1, 1, 8,
		3, 1, 1, 8, 1, 8,
		0, 108, 106, 2, 0, 108,
		106, 2, 0, 108, 0, 108,
	}
	n := len(merged) / 6
	for i := 0; i < n; i++ {
		p1 := MatchPos{Pos: merged[i*6], Len: merged[i*6+1]}
		p2 := MatchPos{Pos: merged[i*6+2], Len: merged[i*6+3]}
		exp := MatchPos{Pos: merged[i*6+4], Len: merged[i*6+5]}
		didMerge, got := mergeRegions(p1, p2)
		if !didMerge {
			t.Errorf("mergeRegions() didn't merge %#v and %#v", p1, p2)
		} else {
			if got.Len != exp.Len || got.Pos != exp.Pos {
				t.Errorf("mergeRegions() got: %s exp: %s, p1: %s, p2: %s", got, exp, p1, p2)
			}
		}
	}
	// each test case is 4 numbers, 2 numbers per MatchPos specifying (pos, len)
	// we send 2 MatchPos and check that they do not overlap
	nonMerged := []int{
		0, 0, 1, 1,
		1, 1, 0, 0,
		0, 5, 6, 1,
		6, 1, 0, 5,
		3, 4, 8, 3,
		8, 3, 3, 4,
	}
	n = len(nonMerged) / 4
	for i := 0; i < n; i++ {
		p1 := MatchPos{Pos: nonMerged[i*4], Len: nonMerged[i*4+1]}
		p2 := MatchPos{Pos: nonMerged[i*4+2], Len: nonMerged[i*4+3]}
		didMerge, got := mergeRegions(p1, p2)
		if didMerge {
			t.Errorf("mergeRegions() unexpectedly merged p1: %s, p2: %s into %s", p1, p2, got)
		}
	}
}
