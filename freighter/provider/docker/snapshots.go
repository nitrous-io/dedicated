package docker

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nitrous-io/dedicated/freighter/provider"
)

func (p *DockerProvider) CreateSnapshot() (*provider.SnapshotInfo, error) {
	containers, err := p.List()
	if err != nil {
		return nil, err
	}
	volumes, err := p.ListVolumes()
	if err != nil {
		return nil, err
	}

	base, err := ioutil.TempDir("", "snapshot")
	if err != nil {
		return nil, err
	}

	hostBase := filepath.Join(base, "host")
	containerBase := filepath.Join(hostBase, "containers")
	volumeBase := filepath.Join(hostBase, "volumes")

	err = os.MkdirAll(containerBase, 0755)
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(volumeBase, 0755)
	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		err := p.createContainerSnapshot(container, filepath.Join(containerBase, fmt.Sprintf("%s.tar", container.Name)))
		if err != nil {
			return nil, err
		}
	}

	for _, volume := range volumes {
		err := p.createVolumeSnapshot(volume, filepath.Join(volumeBase, fmt.Sprintf("%s.tar", volume)))
		if err != nil {
			return nil, err
		}
	}

	// create s3 upload target
	target := filepath.Join(base, "snapshot.tgz")
	fd, err := os.OpenFile(target, os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	// create master tgz
	cmd := exec.Command("tar", "-C", base, "-cz", ".")
	rd, _ := cmd.StdoutPipe()
	cmd.Start()
	ch := make(chan int64)
	go copyAndReturnWritten(fd, rd, ch)
	size := <-ch
	if err = cmd.Wait(); err != nil {
		return nil, err
	}

	snapshot := &provider.SnapshotInfo{Size: size, Url: target}
	return snapshot, nil
}

func copyAndReturnWritten(wr io.Writer, rd io.Reader, ch chan int64) {
	written, _ := io.Copy(wr, rd)
	ch <- written
}

func (p *DockerProvider) createContainerSnapshot(container *provider.ContainerInfo, filename string) error {
	w, err := os.OpenFile(filename, os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer w.Close()
	return p.Export(container.ID, w)
}

func (p *DockerProvider) createVolumeSnapshot(volume string, filename string) error {
	w, err := os.OpenFile(filename, os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer w.Close()
	return p.ExportVolume(volume, w)
}
