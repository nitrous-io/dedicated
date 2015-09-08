package finder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkFuzzyFileFind(b *testing.B) {
	testDir, err := setupTestDir()
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(testDir)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := FuzzyFindFile(testDir, "contain")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkFileFind(b *testing.B) {
	testDir, err := setupTestDir()
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(testDir)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := FindFiles(testDir, "contain")
		if err != nil {
			panic(err)
		}
	}
}

func setupTestDir() (string, error) {
	testDir, err := ioutil.TempDir("/tmp", "benchmark-file-finder")
	if err != nil {
		return "", err
	}

	testFiles := []string{
		"README.md",
		"controllers/containers/containers.go",
		"controllers/containers/containers_test.go",
		"models/container.go",
		"models/workspace.go",
		"models/config.json",
		"dist/server",
		".git/HEAD",
	}

	for _, p := range testFiles {
		path := filepath.Join(testDir, p)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		f, err := os.Create(path)
		if err != nil {
			return "", err
		}
		f.Close()
	}

	return testDir, nil
}
