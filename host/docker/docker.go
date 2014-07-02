package docker

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"strings"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/hostbuilder/host"
)

type Host struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	PublicKeys []string `json:"public_keys"`
	IpAddress  string   `json:"ip_address"`

	client *dockerclient.Client
}

func (h *Host) Create() (*host.Info, error) {
	imageName, err := h.createImage()
	if err != nil {
		return nil, err
	}
	containerId, err := h.createContainer(imageName)
	if err != nil {
		return nil, err
	}
	h.Id = containerId
	return h.Info()
}

func (h *Host) Destroy() error {
	return h.client.RemoveContainer(dockerclient.RemoveContainerOptions{
		ID:    h.Id,
		Force: true,
	})
}

func (h *Host) Info() (*host.Info, error) {
	container, err := h.client.InspectContainer(h.Id)
	if err != nil {
		return nil, err
	}
	return &host.Info{
		Provider:  "docker",
		Id:        h.Id,
		IpAddress: container.NetworkSettings.IPAddress,
	}, nil
}

func (h *Host) Stop() error {
	return h.client.KillContainer(dockerclient.KillContainerOptions{
		ID: h.Id,
	})
}

func (h *Host) Start() error {
	return h.client.StartContainer(h.Id, &dockerclient.HostConfig{
		Privileged: true,
	})
}

func (h *Host) Restart() error {
	return h.client.RestartContainer(h.Id, uint(5))
}

func (h *Host) createImage() (string, error) {
	buf := bytes.NewBuffer(nil)
	if err := dockerfileTemplate.Execute(buf, h); err != nil {
		return "", err
	}

	t := time.Now()
	inputbuf, outputbuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{
		Name:       "Dockerfile",
		Size:       int64(buf.Len()),
		ModTime:    t,
		AccessTime: t,
		ChangeTime: t,
	})
	tr.Write(buf.Bytes())
	tr.Close()

	imageName := "registry.nitrous.io/" + h.Name
	opts := dockerclient.BuildImageOptions{
		Name:         imageName,
		InputStream:  inputbuf,
		OutputStream: outputbuf,
	}
	if err := h.client.BuildImage(opts); err != nil {
		return "", err
	}
	return imageName, nil
}

func (h *Host) createContainer(imageName string) (string, error) {
	container, err := h.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name:   h.Name,
		Config: &dockerclient.Config{Hostname: h.Name, Image: imageName},
	})
	if err != nil {
		return "", err
	}
	// we want privileged because this will be a dind container
	err = h.client.StartContainer(container.ID, &dockerclient.HostConfig{
		Privileged: true,
	})
	if err != nil {
		return "", err
	}
	return container.ID, nil
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	host := &Host{}
	if err := json.Unmarshal([]byte(config), host); err != nil {
		return nil, err
	}
	// trim all spaces for public keys
	for i, v := range host.PublicKeys {
		host.PublicKeys[i] = strings.TrimSpace(v)
	}
	// fetch docker client endpoint from defaultConfig
	endpoint, ok := defaultConfig["endpoint"]
	if !ok {
		endpoint = "unix:///var/run/docker.sock"
	}
	client, err := dockerclient.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	host.client = client
	return host, nil
}

func init() {
	host.Hosts["docker"] = NewHost
}
