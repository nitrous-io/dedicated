package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nitrous-io/freighter/provider"
)

type VolumeError error

var (
	slugRe                 = regexp.MustCompile(`\A[A-Za-z0-9][A-Za-z0-9\-_]*\z`)
	ErrMissingVolumeSlug   = fmt.Errorf("volume slug not given")
	ErrBadVolumeSlugFormat = fmt.Errorf("volume slug can only contain alphanumeric characters, dash, and underscore.  It must start with an alphanumeric character")
	ErrVolumeExists        = fmt.Errorf("volume already exists")
	ErrVolumeNotFound      = fmt.Errorf("volume not found")
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
			p.logger.Error("ERR: Error in ListVolumes with path %s: %s", m, err)
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

	// TODO add timeout
	out, err := exec.Command("du", "--summarize", "--bytes", path).CombinedOutput()
	if err != nil {
		return nil, err
	}

	size, err := parseDuOutput(out, path)
	if err != nil {
		p.logger.Errorf("ERR: unexpected du output %s:\n%s", err, string(out))
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

// parses the output of `du --summarize --bytes <path>`
// which is expected to look like: 1234567\tpatharg\n
func parseDuOutput(output []byte, path string) (uint64, error) {
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return 0, fmt.Errorf("expected 2 fields, got %d", len(fields))
	}
	if fields[1] != path {
		return 0, fmt.Errorf("expected path in 2nd field, got %s", fields[1])
	}
	size, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error converting size: %v", err)
	}
	return size, nil
}
