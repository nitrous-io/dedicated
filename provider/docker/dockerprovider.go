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
	client         *dockerclient.Client
	store          *datastore.Datastore
	dataPath       string
	portPool       *PortPool
	imageNamespace string
}

func NewDockerProvider(store *datastore.Datastore, dataPath string, imageNamespace string) (*DockerProvider, error) {
	client, err := dockerclient.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		return nil, err
	}
	return &DockerProvider{client: client, store: store, dataPath: dataPath, portPool: NewPortPool(store), imageNamespace: imageNamespace + "."}, nil
}

func (self *DockerProvider) List() ([]*provider.ContainerInfo, error) {
	apicontainers, err := self.client.ListContainers(dockerclient.ListContainersOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}

	list := []*provider.ContainerInfo{}

	for _, apicontainer := range apicontainers {
		if strings.HasPrefix(apicontainer.Image, self.imageNamespace) {
			info, err := self.GetInfo(apicontainer.ID)
			if err != nil {
				return nil, err
			}
			list = append(list, info)
		}
	}

	return list, nil
}

func (self *DockerProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	imageName, err := self.buildImage(name, image)
	if err != nil {
		return nil, err
	}

	// validate pubkeys and remove duplicates
	keymap, err := keysToKeyMap(publicKeys)
	if err != nil {
		return nil, err
	}
	publicKeys = keyMapToKeys(keymap)

	port, err := self.portPool.Acquire()
	if err != nil {
		return nil, err
	}

	err = self.createAuthorizedKeys(name, publicKeys)
	if err != nil {
		return nil, err
	}

	container, err := self.runContainer(imageName, name, port)
	if err != nil {
		if err := self.portPool.Release(port); err != nil {
			return nil, err
		}
		return nil, err
	}

	return self.GetInfo(container.ID)
}

func (self *DockerProvider) GetInfo(id string) (*provider.ContainerInfo, error) {
	container, err := self.client.InspectContainer(id)
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

func (self *DockerProvider) Start(id string) error {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return err
	}

	if container.State.Running {
		return nil
	}

	return self.client.StartContainer(id, container.HostConfig)
}

func (self *DockerProvider) Stop(id string) error {
	return self.client.StopContainer(id, 15)
}

func (self *DockerProvider) Restart(id string) error {
	return self.client.RestartContainer(id, 15)
}

func (self *DockerProvider) Terminate(id string) error {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return err
	}

	if container.State.Running {
		err = self.client.StopContainer(id, 15)
		if err != nil {
			return err
		}
	}

	err = self.client.RemoveContainer(dockerclient.RemoveContainerOptions{ID: id})
	if err != nil {
		return err
	}

	err = self.client.RemoveImage(container.Config.Image)
	if err != nil {
		return err
	}

	return self.portPool.Release(portFromContainer(container))
}

func (self *DockerProvider) ListPublicKeys(id string) (map[string]string, error) {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	keys, err := self.readAuthorizedKeys(nameFromContainer(container))
	if err != nil {
		return nil, err
	}

	return keysToKeyMap(keys)
}

func (self *DockerProvider) AddPublicKeys(id string, publicKeys []string) error {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return err
	}
	name := nameFromContainer(container)

	existingKeys, err := self.readAuthorizedKeys(name)
	if err != nil {
		return err
	}

	// validate pubkeys and remove duplicates
	keymap, err := keysToKeyMap(append(publicKeys, existingKeys...))
	if err != nil {
		return err
	}
	publicKeys = keyMapToKeys(keymap)

	return self.createAuthorizedKeys(name, publicKeys)
}

func (self *DockerProvider) RemovePublicKey(id string, keyFingerprint string) error {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return err
	}
	name := nameFromContainer(container)

	keys, err := self.readAuthorizedKeys(name)
	if err != nil {
		return err
	}

	keymap, err := keysToKeyMap(keys)
	if err != nil {
		return err
	}
	delete(keymap, keyFingerprint)
	keys = keyMapToKeys(keymap)

	return self.createAuthorizedKeys(name, keys)
}

func (self *DockerProvider) IsNotFoundError(err error) bool {
	_, ok := err.(*dockerclient.NoSuchContainer)
	return ok
}

func (self *DockerProvider) buildImage(name string, image string) (string, error) {
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

	imageName := self.imageNamespace + name
	opts := dockerclient.BuildImageOptions{
		Name:           imageName,
		InputStream:    inputbuf,
		OutputStream:   outputbuf,
		RmTmpContainer: false,
	}

	err = self.client.BuildImage(opts)
	if err != nil {
		return "", err
	}

	return imageName, err
}

func (self *DockerProvider) containerSettingsDir(containerName string) string {
	return path.Join(self.dataPath, "containers", containerName)
}

func (self *DockerProvider) createAuthorizedKeys(containerName string, publicKeys []string) error {
	dir := path.Join(self.containerSettingsDir(containerName), "ssh")
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

func (self *DockerProvider) readAuthorizedKeys(containerName string) ([]string, error) {
	keysFile := path.Join(self.containerSettingsDir(containerName), "ssh", "authorized_keys")
	bkeys, err := ioutil.ReadFile(keysFile)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(bkeys)), "\n"), nil
}

func (self *DockerProvider) runContainer(imageName string, containerName string, port uint16) (*dockerclient.Container, error) {
	container, err := self.client.CreateContainer(dockerclient.CreateContainerOptions{
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

	err = self.client.StartContainer(container.ID, &dockerclient.HostConfig{
		Binds: []string{
			(self.containerSettingsDir(containerName) + "/ssh:/etc/ssh/freighter:ro"),
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
