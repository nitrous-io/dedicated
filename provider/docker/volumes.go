package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type VolumeError error

var (
	slugRe                 = regexp.MustCompile(`\A[A-Za-z0-9][A-Za-z0-9\-_]*\z`)
	ErrMissingVolumeSlug   = fmt.Errorf("volume slug not given")
	ErrBadVolumeSlugFormat = fmt.Errorf("volume slug can only contain alphanumeric characters, dash, and underscore.  It must start with an alphanumeric character")
	ErrVolumeExists        = fmt.Errorf("volume already exists")
)

func (p *DockerProvider) CreateVolume(slug string) error {
	if slug == "" {
		return ErrMissingVolumeSlug
	}
	if !slugRe.MatchString(slug) {
		return ErrBadVolumeSlugFormat
	}

	path := p.VolumesDir(slug)

	_, err := os.Stat(path)
	if err == nil || !os.IsNotExist(err) {
		return ErrVolumeExists
	}

	return os.MkdirAll(path, 0755)
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
			p.logger.Warn("ERR: Error in ListVolumes with path %s: %s", m, err)
			continue
		}
		if !fi.IsDir() {
			continue
		}
		vols = append(vols, filepath.Base(m))
	}
	return vols, nil
}

// VolumesDir returns the path on the host for a volume of the given slug.  If
// slug is "", it returns the parent directory of all volumes.
func (p *DockerProvider) VolumesDir(slug string) string {
	if slug == "" {
		return filepath.Join(p.dataPath, "volumes")
	}
	return filepath.Join(p.dataPath, "volumes", slug)
}
