package integration

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"

	"github.com/nitrous-io/dedicated/freighter/provider/docker"
)

func createImageTarball(df string) (io.Reader, error) {
	buf := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buf)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "Dockerfile", Size: int64(len(df))}); err != nil {
		return nil, err
	}
	if _, err := tarWriter.Write([]byte(df)); err != nil {
		return nil, err
	}
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

func createNewProvider() (*docker.DockerProvider, string, error) {
	data, err := ioutil.TempDir("", "freightertest")
	if err != nil {
		return nil, "", err
	}
	prov, err := docker.NewDockerProvider(data)
	if err != nil {
		os.RemoveAll(data)
		return nil, "", err
	}
	return prov, data, nil
}
