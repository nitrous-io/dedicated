package docker

import (
	"archive/tar"
	"bytes"
	"errors"
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
	logger         *util.Logger
}

func NewDockerProvider(logger *util.Logger, store *datastore.Datastore, dataPath string, imageNamespace string) (*DockerProvider, error) {
	if logger == nil || store == nil || dataPath == "" || imageNamespace == "" {
		return nil, errors.New("docker.NewDockerProvider: invalid argument")
	}
	client, err := dockerclient.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		return nil, err
	}
	return &DockerProvider{client: client, store: store, dataPath: dataPath, portPool: NewPortPool(store), imageNamespace: imageNamespace + ".", logger: logger}, nil
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
	self.logger.Info("DockerProvider: acquired port", port)

	err = self.createAuthorizedKeys(name, publicKeys)
	if err != nil {
		return nil, err
	}

	container, err := self.createContainer(imageName, name, port)
	if err != nil {
		self.logger.Info("DockerProvider: releasing port", port)
		errr := self.portPool.Release(port)
		if errr != nil {
			return nil, errr
		}
		self.logger.Info("DockerProvider: released port", port)
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
		Id:      container.ID,
		Name:    nameFromContainer(container),
		Image:   container.Config.Image,
		Host:    container.NetworkSettings.IPAddress,
		SSHPort: portFromContainer(container),
		State:   stateFromContainer(container),
	}, nil
}

func (self *DockerProvider) Start(id string) (*provider.ContainerInfo, error) {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	if !container.State.Running {
		self.logger.Info("DockerProvider: starting container", id)
		start := time.Now()

		err = self.client.StartContainer(id, container.HostConfig)
		if err != nil {
			return nil, err
		}

		self.logger.TimeInfo(start, "DockerProvider: started container", id, "in")
	}
	return self.GetInfo(id)
}

func (self *DockerProvider) Stop(id string) (*provider.ContainerInfo, error) {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	if container.State.Running {
		self.logger.Info("DockerProvider: stopping container", id)
		start := time.Now()

		err := self.client.StopContainer(id, 15)
		if err != nil {
			return nil, err
		}

		self.logger.TimeInfo(start, "DockerProvider: stopped container", id, "in")
	}
	return self.GetInfo(id)
}

func (self *DockerProvider) Restart(id string) (*provider.ContainerInfo, error) {
	_, err := self.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	self.logger.Info("DockerProvider: restarting container", id)
	start := time.Now()

	err = self.client.RestartContainer(id, 15)
	if err != nil {
		return nil, err
	}

	self.logger.TimeInfo(start, "DockerProvider: restarted container", id, "in")
	return self.GetInfo(id)
}

func (self *DockerProvider) Terminate(id string) (*provider.ContainerInfo, error) {
	container, err := self.client.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	self.logger.Info("DockerProvider: terminating container", id)
	terminationStart := time.Now()

	info, err := self.Stop(id)
	if err != nil {
		return nil, err
	}

	self.logger.Info("DockerProvider: removing container", id)
	start := time.Now()

	err = self.client.RemoveContainer(dockerclient.RemoveContainerOptions{ID: id})
	if err != nil {
		return nil, err
	}

	self.logger.TimeInfo(start, "DockerProvider: removed container", id, "in")

	containerImage := container.Config.Image
	self.logger.Info("DockerProvider: removing container image", containerImage)
	start = time.Now()

	err = self.client.RemoveImage(containerImage)
	if err != nil {
		return nil, err
	}

	self.logger.TimeInfo(start, "DockerProvider: removed container image", containerImage, "in")

	port := portFromContainer(container)
	self.logger.Info("DockerProvider: releasing port", port)
	err = self.portPool.Release(port)
	if err != nil {
		return nil, err
	}
	self.logger.Info("DockerProvider: released port", port)

	info.Host = ""
	info.SSHPort = 0
	info.State = ""

	self.logger.TimeInfo(terminationStart, "DockerProvider: terminated container", id, "in")
	return info, nil
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
	imageName := self.imageNamespace + name
	self.logger.Info("DockerProvider: building container image", imageName, "with base image", image)
	start := time.Now()

	buf := bytes.NewBuffer(nil)
	err := dockerfileUbuntu.Execute(buf, &TemplateData{Name: name, Image: image})
	if err != nil {
		return "", err
	}

	inputbuf, outputbuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(buf.Len()), ModTime: start, AccessTime: start, ChangeTime: start})
	tr.Write(buf.Bytes())
	tr.Close()

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

	self.logger.TimeInfo(start, "DockerProvider: built container image", imageName, "in")
	return imageName, err
}

func (self *DockerProvider) containerSettingsDir(containerName string) string {
	return path.Join(self.dataPath, "containers", containerName)
}

func (self *DockerProvider) createAuthorizedKeys(containerName string, publicKeys []string) error {
	self.logger.Info("DockerProvider: creating SSH authorized keys file for container", containerName, "with", len(publicKeys), "entries")
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

	self.logger.Info("DockerProvider: created SSH authorized keys file for container", containerName)
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

func (self *DockerProvider) createContainer(imageName string, containerName string, port uint16) (*dockerclient.Container, error) {
	self.logger.Info("DockerProvider: creating container", containerName, "with image", imageName)
	start := time.Now()

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

	id := container.ID
	self.logger.TimeInfo(start, "DockerProvider: created container", containerName, id, "in")
	self.logger.Info("DockerProvider: starting container", containerName)
	start = time.Now()

	err = self.client.StartContainer(id, &dockerclient.HostConfig{
		Binds: []string{
			(self.containerSettingsDir(containerName) + "/ssh:/etc/ssh/freighter:ro"),
		},
		PortBindings: map[dockerclient.Port][]dockerclient.PortBinding{
			"22/tcp": {{HostPort: strconv.Itoa(int(port))}},
		},
	})

	self.logger.TimeInfo(start, "DockerProvider: started container", containerName, id, "in")
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
