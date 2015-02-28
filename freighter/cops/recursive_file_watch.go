// based on https://github.com/nathany/looper/blob/master/watch.go
// BSD LICENSE: https://github.com/nathany/looper/blob/master/LICENSE
package cops

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-fsnotify/fsnotify"
)

type RecursiveWatcher struct {
	*fsnotify.Watcher
	Folders chan string
}

func NewRecursiveWatcher(path string) (*RecursiveWatcher, error) {
	folders := subfolders(path)
	if len(folders) == 0 {
		return nil, errors.New("No folders to watch.")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	rw := &RecursiveWatcher{
		Watcher: watcher,
		Folders: make(chan string, len(folders)),
	}

	for _, folder := range folders {
		rw.AddFolder(folder)
	}
	return rw, nil
}

func (watcher *RecursiveWatcher) AddFolder(folder string) {
	err := watcher.Add(folder)
	if err != nil {
		log.Println("Error watching: ", folder, err)
	} else {
		log.Printf("Started watching '%s'\n", folder)
	}
}

func notifyAboutEvent(e fsnotify.Event) bool {
	if e.Op&fsnotify.Create == fsnotify.Create {
		return true
	}
	if e.Op&fsnotify.Remove == fsnotify.Remove {
		return true
	}
	if e.Op&fsnotify.Rename == fsnotify.Rename {
		return true
	}
	return false
}

func (watcher *RecursiveWatcher) Run() {
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				//log.Printf("event: '%s'\n", event)
				if notifyAboutEvent(event) {
					fi, err := os.Stat(event.Name)
					if err == nil && fi.IsDir() {
						if !shouldIgnoreFile(filepath.Base(event.Name)) {
							watcher.AddFolder(event.Name)
						}
					}
					if !shouldIgnoreFile(filepath.Base(event.Name)) {
						parentDir := filepath.Dir(event.Name)
						watcher.Folders <- parentDir
					}
				}

			case err := <-watcher.Errors:
				log.Println("error", err)
			}
		}
	}()
}

// subfolders returns a slice of subfolders (recursive), including the folder provided.
func subfolders(path string) (paths []string) {
	filepath.Walk(path, func(newPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			// skip folders that begin with a dot
			if shouldIgnoreFile(name) && name != "." && name != ".." {
				return filepath.SkipDir
			}
			paths = append(paths, newPath)
		}
		return nil
	})
	return paths
}

// shouldIgnoreFile determines if a file or directory should be ignored.
// File names that begin with "." or "_" are ignored by the go tool.
func shouldIgnoreFile(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}
