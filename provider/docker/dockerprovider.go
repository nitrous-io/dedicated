package docker

import (
	"archive/tar"
	"bytes"
	"strconv"
	"strings"
	"text/template"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider"
)

var dockerfileUbuntu *template.Template

func init() {
	dockerfileUbuntu = template.Must(template.New("ubuntu").Parse(dockerfileUbuntuTmpl))
}

type TemplateData struct {
	Image      string
	PublicKeys []string
}

type DockerProvider struct {
	client   *dockerclient.Client
	store    *datastore.Datastore
	portPool *PortPool
}

func NewDockerProvider(store *datastore.Datastore) (*DockerProvider, error) {
	client, err := dockerclient.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		return nil, err
	}
	return &DockerProvider{client: client, store: store, portPool: NewPortPool(store)}, nil
}

func (p *DockerProvider) List() ([]*provider.ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	imageName, err := p.buildImage(name, image, publicKeys)
	if err != nil {
		return nil, err
	}
	port, err := p.portPool.Acquire()
	if err != nil {
		return nil, err
	}

	container, err := p.runContainer(imageName, name, port)
	if err != nil {
		if err := p.portPool.Release(port); err != nil {
			return nil, err
		}
		return nil, err
	}

	return p.GetInfo(container.ID)
}

func (p *DockerProvider) GetInfo(id string) (*provider.ContainerInfo, error) {
	container, err := p.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(container.HostConfig.PortBindings["22/tcp"][0].HostPort)

	return &provider.ContainerInfo{
		Id:    container.ID,
		Name:  container.Name[1:],
		Image: container.Config.Image,
		Port:  uint16(port),
		State: stateFromContainer(container),
	}, nil
}

func (p *DockerProvider) Start(id string) error {
	container, err := p.client.InspectContainer(id)
	if err != nil {
		return err
	}

	if container.State.Running {
		return nil
	}

	return p.client.StartContainer(id, container.HostConfig)
}

func (p *DockerProvider) Stop(id string) error {
	return p.client.StopContainer(id, 15)
}

func (p *DockerProvider) Restart(id string) error {
	return p.client.RestartContainer(id, 15)
}

func (p *DockerProvider) Terminate(id string) error {
	err := p.client.StopContainer(id, 15)
	if err != nil {
		return err
	}
	return p.client.RemoveContainer(dockerclient.RemoveContainerOptions{
		ID:            id,
		RemoveVolumes: false,
		Force:         false,
	})
}

func (p *DockerProvider) buildImage(name string, image string, publicKeys []string) (string, error) {
	for i, v := range publicKeys {
		publicKeys[i] = strings.TrimSpace(v)
	}

	buf := bytes.NewBuffer(nil)
	err := dockerfileUbuntu.Execute(buf, &TemplateData{Image: image, PublicKeys: publicKeys})
	if err != nil {
		return "", err
	}

	t := time.Now()
	inputbuf, outputbuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(buf.Len()), ModTime: t, AccessTime: t, ChangeTime: t})
	tr.Write(buf.Bytes())
	tr.Close()

	imageName := "registry.nitrous.io/" + name
	opts := dockerclient.BuildImageOptions{
		Name:         imageName,
		InputStream:  inputbuf,
		OutputStream: outputbuf,
	}

	err = p.client.BuildImage(opts)
	if err != nil {
		return "", err
	}

	return imageName, err
}

func (p *DockerProvider) runContainer(imageName string, containerName string, port uint16) (*dockerclient.Container, error) {
	container, err := p.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name:   containerName,
		Config: &dockerclient.Config{Image: imageName},
	})
	if err != nil {
		return container, err
	}

	err = p.client.StartContainer(container.ID, &dockerclient.HostConfig{
		PortBindings: map[dockerclient.Port][]dockerclient.PortBinding{
			"22/tcp": {{HostPort: strconv.Itoa(int(port))}},
		},
	})
	return container, err
}

func stateFromContainer(container *dockerclient.Container) string {
	if container.State.Running {
		return provider.ContainerStateRunning
	}
	return provider.ContainerStateStopped
}
