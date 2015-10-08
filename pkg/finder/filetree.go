package finder

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/go-fsnotify/fsnotify"
)

type filetreeCaches struct {
	caches map[string]*filetreeCache
	mu     sync.Mutex
}

type filetreeCache struct {
	paths           []string
	valid           bool // False when this cache is no longer valid.
	fileWatcher     *fsnotify.Watcher
	quitFileWatcher chan struct{}
}

func (c *filetreeCache) invalidate() {
	c.valid = false
	close(c.quitFileWatcher)
}

func (c *filetreeCaches) get(dir string) ([]string, bool) {
	c.mu.Lock()
	cache, ok := c.caches[dir]
	c.mu.Unlock()

	if !ok {
		return nil, false
	}

	if !cache.valid {
		return nil, false
	}

	return cache.paths, true
}

func (c *filetreeCaches) set(dir string, tree, subdirs []string) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Warnf("unable to start fsnotify watcher, dir %q won't be cached, err: %v", dir, err)
		return
	}

	cache := &filetreeCache{
		paths:           tree,
		valid:           true,
		fileWatcher:     fw,
		quitFileWatcher: make(chan struct{}),
	}

	// Add all directories in the tree to watcher (fsnotify does not recursively
	// watch sub directories).
	fw.Add(dir)
	for _, d := range subdirs {
		fw.Add(d)
	}

	go func() {
		defer fw.Close()
		for {
			select {
			case event := <-fw.Events:
				if event.Op&fsnotify.Create == fsnotify.Create {
					cache.invalidate()
					return
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					cache.invalidate()
					return
				}
				if event.Op&fsnotify.Rename == fsnotify.Rename {
					cache.invalidate()
					return
				}
			case err := <-fw.Errors:
				logrus.Errorf("fsnotify watcher error on dir %q, err: %v", dir, err)
			case <-cache.quitFileWatcher:
				logrus.Infof("fsnotify watcher on dir %q quitting", dir)
			}
		}
	}()

	c.mu.Lock()
	c.caches[dir] = cache
	c.mu.Unlock()
}

var (
	ftCaches = &filetreeCaches{caches: make(map[string]*filetreeCache)}
)

// filetree returns a list of paths of all files and directories under a
// directory. The paths are relative to the given directory.
func filetree(dir string) ([]string, error) {
	if paths, ok := ftCaches.get(dir); ok {
		return paths, nil
	}

	// If given a symlink to a dir, resolve it to its real path because
	// filepath.Walk does not handle symlinks.
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}

	var paths, subdirs []string

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logrus.Warnf("error walking %s, err: %v", path, err)
			return nil // Don't return an error so that we continue walking elsewhere.
		}

		// TODO Support ignoring directories and patterns (e.g. .git, .svn).

		mode := info.Mode()

		if mode.IsDir() {
			subdirs = append(subdirs, path)
		}

		if !mode.IsRegular() {
			return nil
		}

		// Strip out directory from path since filepath.Walk will prefix all paths
		// with it.
		relPath, err := filepath.Rel(realDir, path)
		if err != nil {
			paths = append(paths, path)
		} else {
			paths = append(paths, relPath)
		}

		return nil
	}

	if err := filepath.Walk(realDir, walkFn); err != nil {
		return nil, err
	}

	ftCaches.set(dir, paths, subdirs)

	return paths, nil
}
