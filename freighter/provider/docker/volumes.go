package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/freighter/provider"
)

type VolumeError error

var (
	slugRe                 = regexp.MustCompile(`\A[A-Za-z0-9][A-Za-z0-9\-_]*\z`)
	ErrMissingVolumeSlug   = fmt.Errorf("volume slug not given")
	ErrBadVolumeSlugFormat = fmt.Errorf("volume slug can only contain alphanumeric characters, dash, and underscore.  It must start with an alphanumeric character")
	ErrVolumeExists        = fmt.Errorf("volume already exists")
	ErrVolumeNotFound      = fmt.Errorf("volume not found")
)

func (p *DockerProvider) CreateVolume(slug string) (*provider.VolumeInfo, error) {
	if slug == "" {
		return nil, ErrMissingVolumeSlug
	}
	if !slugRe.MatchString(slug) {
		return nil, ErrBadVolumeSlugFormat
	}

	path := p.VolumesDir(slug)

	_, err := os.Stat(path)
	if err == nil || !os.IsNotExist(err) {
		return nil, ErrVolumeExists
	}

	err = os.MkdirAll(path, 0755)
	if err != nil {
		return nil, err
	}
	return p.GetVolumeInfo(slug)
}

func (p *DockerProvider) ListVolumes() ([]string, error) {
	vols := []string{}
	volumesBase := p.VolumesDir("")
	matches, err := filepath.Glob(filepath.Join(volumesBase, "*"))
	if err != nil {
		return []string{}, err
	}
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			// do not allow a single problematic path to prevent access to list
			logrus.Errorf("ERR: Error in ListVolumes with path %s: %s", m, err)
			continue
		}
		if !fi.IsDir() {
			continue
		}
		vols = append(vols, filepath.Base(m))
	}
	return vols, nil
}

func (p *DockerProvider) RemoveVolume(slug string) error {
	if slug == "" {
		return ErrMissingVolumeSlug
	}

	path := p.VolumesDir(slug)
	if path == p.VolumesDir("") { // double-check to be safe
		return fmt.Errorf("Cannot delete all volumes")
	}

	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return ErrVolumeNotFound
	}

	return os.RemoveAll(path)
}

func (p *DockerProvider) GetVolumeInfo(slug string) (*provider.VolumeInfo, error) {
	if slug == "" {
		return nil, ErrMissingVolumeSlug
	}

	path := p.VolumesDir(slug)
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return nil, ErrVolumeNotFound
	}

	size, err := treeSize(path)
	if err != nil {
		logrus.Errorf("ERR treeSize: %s", err)
		return nil, fmt.Errorf("unexpected error while checking disk usage")
	}
	vi := &provider.VolumeInfo{
		Slug: slug,
		Size: size,
	}

	return vi, nil
}

// VolumesDir returns the path on the host for a volume of the given slug.  If
// slug is "", it returns the parent directory of all volumes.
func (p *DockerProvider) VolumesDir(slug string) string {
	if slug == "" {
		return filepath.Join(p.dataPath, "volumes")
	}
	return filepath.Join(p.dataPath, "volumes", slug)
}

// treeSize walks a directory tree and returns its total size in bytes.
// Adapted from dotcloud/docker/utils
func treeSize(path string) (uint64, error) {
	data := make(map[uint64]struct{})
	size := uint64(0)
	err := filepath.Walk(path, func(d string, fileInfo os.FileInfo, e error) error {
		// Ignore directory sizes
		if fileInfo == nil {
			return nil
		}

		s := fileInfo.Size()
		if fileInfo.IsDir() || s == 0 {
			return nil
		}

		// Check inode to handle hard links correctly
		inode := fileInfo.Sys().(*syscall.Stat_t).Ino
		// inode is not a uint64 on all platforms. Cast it to avoid issues.
		if _, exists := data[uint64(inode)]; exists {
			return nil
		}
		// inode is not a uint64 on all platforms. Cast it to avoid issues.
		data[uint64(inode)] = struct{}{}

		size += uint64(s)

		return nil
	})
	if err != nil {
		return 0, err
	}
	return size, nil
}
