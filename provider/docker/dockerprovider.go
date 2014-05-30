package docker

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/util"
)

var dockerfileUbuntu = template.Must(template.New("ubuntu").Parse(dockerfileUbuntuTmpl))

type TemplateData struct {
	Name  string
	Image string
}

type DockerProvider struct {
	client   *dockerclient.Client
	store    *datastore.Datastore
	dataPath string
	portPool *PortPool
}

func NewDockerProvider(store *datastore.Datastore, dataPath string) (*DockerProvider, error) {
	client, err := dockerclient.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		return nil, err
	}
	return &DockerProvider{client: client, store: store, dataPath: dataPath, portPool: NewPortPool(store)}, nil
}

func (p *DockerProvider) List() ([]*provider.ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	imageName, err := p.buildImage(name, image)
	if err != nil {
		return nil, err
	}

	// validate pubkeys and remove duplicates
	keymap, err := keysToKeyMap(publicKeys)
	if err != nil {
		return nil, err
	}
	publicKeys = keyMapToKeys(keymap)

	port, err := p.portPool.Acquire()
	if err != nil {
		return nil, err
	}

	err = p.createAuthorizedKeys(name, publicKeys)
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

	return &provider.ContainerInfo{
		Id:    container.ID,
		Name:  nameFromContainer(container),
		Image: container.Config.Image,
		Port:  portFromContainer(container),
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
	container, err := p.client.InspectContainer(id)
	if err != nil {
		return err
	}

	if container.State.Running {
		err = p.client.StopContainer(id, 15)
		if err != nil {
			return err
		}
	}

	err = p.client.RemoveContainer(dockerclient.RemoveContainerOptions{
		ID:            id,
		RemoveVolumes: false,
		Force:         false,
	})
	if err != nil {
		return err
	}

	err = p.client.RemoveImage(container.Config.Image)
	if err != nil {
		return err
	}

	return p.portPool.Release(portFromContainer(container))
}

func (p *DockerProvider) ListPublicKeys(id string) (map[string]string, error) {
	container, err := p.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	keys, err := p.readAuthorizedKeys(nameFromContainer(container))
	if err != nil {
		return nil, err
	}

	return keysToKeyMap(keys)
}

func (p *DockerProvider) AddPublicKeys(id string, publicKeys []string) error {
	container, err := p.client.InspectContainer(id)
	if err != nil {
		return err
	}
	name := nameFromContainer(container)

	existingKeys, err := p.readAuthorizedKeys(name)
	if err != nil {
		return err
	}

	// validate pubkeys and remove duplicates
	keymap, err := keysToKeyMap(append(publicKeys, existingKeys...))
	if err != nil {
		return err
	}
	publicKeys = keyMapToKeys(keymap)

	return p.createAuthorizedKeys(name, publicKeys)
}

func (p *DockerProvider) RemovePublicKey(id string, keyFingerprint string) error {
	container, err := p.client.InspectContainer(id)
	if err != nil {
		return err
	}
	name := nameFromContainer(container)

	keys, err := p.readAuthorizedKeys(name)
	if err != nil {
		return err
	}

	keymap, err := keysToKeyMap(keys)
	if err != nil {
		return err
	}
	delete(keymap, keyFingerprint)
	keys = keyMapToKeys(keymap)

	return p.createAuthorizedKeys(name, keys)
}

func (p *DockerProvider) buildImage(name string, image string) (string, error) {
	buf := bytes.NewBuffer(nil)
	err := dockerfileUbuntu.Execute(buf, &TemplateData{Name: name, Image: image})
	if err != nil {
		return "", err
	}

	t := time.Now()
	inputbuf, outputbuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(buf.Len()), ModTime: t, AccessTime: t, ChangeTime: t})
	tr.Write(buf.Bytes())
	tr.Close()

	imageName := "image-" + name
	opts := dockerclient.BuildImageOptions{
		Name:           imageName,
		InputStream:    inputbuf,
		OutputStream:   outputbuf,
		RmTmpContainer: true,
	}

	err = p.client.BuildImage(opts)
	if err != nil {
		return "", err
	}

	return imageName, err
}

func (p *DockerProvider) containerSettingsDir(containerName string) string {
	return path.Join(p.dataPath, "containers", containerName)
}

func (p *DockerProvider) createAuthorizedKeys(containerName string, publicKeys []string) error {
	dir := path.Join(p.containerSettingsDir(containerName), "ssh")
	err := os.MkdirAll(dir, 0770)
	if err != nil {
		return err
	}
	filename := path.Join(dir, "authorized_keys")
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, key := range publicKeys {
		key = strings.TrimSpace(strings.Replace(key, "\n", "", -1)) + "\n"
		_, err = file.Write([]byte(key))
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *DockerProvider) readAuthorizedKeys(containerName string) ([]string, error) {
	keysFile := path.Join(p.containerSettingsDir(containerName), "ssh", "authorized_keys")
	bkeys, err := ioutil.ReadFile(keysFile)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(bkeys)), "\n"), nil
}

func (p *DockerProvider) runContainer(imageName string, containerName string, port uint16) (*dockerclient.Container, error) {
	container, err := p.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name: containerName,
		Config: &dockerclient.Config{
			Hostname: containerName,
			Image:    imageName,
			Volumes: map[string]struct{}{
				"/etc/ssh/freighter": {},
			},
		},
	})
	if err != nil {
		return container, err
	}

	err = p.client.StartContainer(container.ID, &dockerclient.HostConfig{
		Binds: []string{
			(p.containerSettingsDir(containerName) + "/ssh:/etc/ssh/freighter:ro"),
		},
		PortBindings: map[dockerclient.Port][]dockerclient.PortBinding{
			"22/tcp": {{HostPort: strconv.Itoa(int(port))}},
		},
	})
	return container, err
}

func nameFromContainer(container *dockerclient.Container) string {
	return container.Name[1:]
}

func stateFromContainer(container *dockerclient.Container) string {
	if container.State.Running {
		return provider.ContainerStateRunning
	}
	return provider.ContainerStateStopped
}

func portFromContainer(container *dockerclient.Container) uint16 {
	port, _ := strconv.Atoi(container.HostConfig.PortBindings["22/tcp"][0].HostPort)
	return uint16(port)
}

// converts []pubkey to map[keyFingerprint]pubkey
func keysToKeyMap(keys []string) (map[string]string, error) {
	keymap := map[string]string{}
	for _, key := range keys {
		fingerprint, err := util.Md5Fingerprint(key)
		if err != nil {
			return nil, err
		}
		keymap[fingerprint] = key
	}
	return keymap, nil
}

// converts map[keyFingerprint]pubkey back to []pubkey
func keyMapToKeys(keymap map[string]string) []string {
	keys := make([]string, len(keymap))
	i := 0
	for _, key := range keymap {
		keys[i] = key
		i++
	}
	return keys
}
