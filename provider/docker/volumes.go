package docker

import (
	"fmt"
	"os"
	"path"
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

func (p *DockerProvider) VolumesDir(slug string) string {
	return path.Join(p.dataPath, "volumes", slug)
}
