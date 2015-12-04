package docker

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/pkg/queue"
)

const DefaultPort uint16 = 65333

var (
	// Paths to files within each Docker host.
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
	ConfigLocation  = "/etc/freighterd/config.json"
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type driverConfig struct {
	Endpoint           string `json:"endpoint"`
	FreighterdLocation string `json:"freighterd_location"`
	DockerLocation     string `json:"docker_location"`
	// Path to CA certificate of registry (needed if registry's cert is self-signed).
	RegistryCALocation string `json:"registry_ca_location"`
	// Path to CA certificate of registry inside each host.
	// I'd use a Docker volume to inject this, but you can't use a volume path
	// that contains a colon :"( - https://github.com/docker/docker/issues/8604.
	RegistryCADestination string `json:"registry_ca_destination"`
}

type Host struct {
	driverConfig

	ID         string     `json:"id"`
	Name       string     `json:"name"`
	PublicKeys []string   `json:"public_keys"`
	IpAddress  string     `json:"ip_address"`
	TLS        *tlsConfig `json:"tls"`
	Secret     string     `json:"secret"`

	requestID string

	client *dockerclient.Client
}

// Create creates the docker container which acts as a container host, and write the
// progress into queue
func (h *Host) CreateWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	for i, k := range h.PublicKeys {
		h.PublicKeys[i] = strings.TrimSpace(k)
	}
	host.PublishMessage(qp, event.HostCreating, h.requestID, queueInfo, "Creating host", queue.Create)
	imageName, err := h.createImage()
	if err != nil {
		host.PublishMessage(qp, event.HostCreateFailed, h.requestID, queueInfo, "Failed to create host's image", queue.Create)
		return nil, err
	}
	host.PublishMessage(qp, event.HostCreating, h.requestID, queueInfo, "Created host image", queue.Create)

	c, err := h.createContainer(imageName)
	if err != nil {
		host.PublishMessage(qp, event.HostCreateFailed, h.requestID, queueInfo, "Failed to create host's container", queue.Create)
		return nil, err
	}
	host.PublishHost(qp, event.HostCreated, h.requestID, queueInfo, c, queue.Create, "")
	return c, nil
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	container, err := h.client.InspectContainer(h.ID)
	if err != nil {
		return nil, err
	}

	if container.Name != h.Name {
		fmt.Fprintf(w, "Renaming container from %q to %q\n", container.Name, h.Name)
		if err := h.client.RenameContainer(dockerclient.RenameContainerOptions{
			ID:   container.ID,
			Name: h.Name,
		}); err != nil {
			fmt.Fprintf(w, "Failed to rename container, err: %v\n", err)
			return nil, err
		}
	}
	return h.Info()
}

func (h *Host) RespecWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	host.PublishMessage(qp, event.HostRespeccing, h.requestID, queueInfo, "Respeccing host", queue.Respec)
	inf, err := h.Info()
	if err != nil {
		host.PublishMessage(qp, event.HostRespecFailed, h.requestID, queueInfo, err.Error(), queue.Respec)
		return nil, err
	}

	host.PublishHost(qp, event.HostRespecced, h.requestID, queueInfo, inf, queue.Respec, "")
	return inf, nil
}

func (h *Host) Destroy() error {
	return h.client.RemoveContainer(dockerclient.RemoveContainerOptions{
		ID:    h.ID,
		Force: true,
	})
}

func (h *Host) Info() (*host.Info, error) {
	container, err := h.client.InspectContainer(h.ID)
	if err != nil {
		return nil, err
	}

	return &host.Info{
		Provider:       h.DriverName(),
		ID:             h.ID,
		Region:         "local",
		Name:           h.Name,
		IPAddress:      "127.0.0.1",
		FreighterdPort: uint16(getFreighterdPort(container)),
		State:          getState(container),
	}, nil
}

func (h *Host) List() ([]*host.Info, error) {
	list, err := h.client.ListContainers(dockerclient.ListContainersOptions{All: true, Size: false})
	if err != nil {
		return nil, err
	}

	ret := make([]*host.Info, 0, len(list))
	for _, cn := range list {
		container, err := h.client.InspectContainer(cn.ID)
		if err != nil {
			return nil, err
		}

		h := &host.Info{
			Provider:       h.DriverName(),
			ID:             cn.ID,
			IPAddress:      "127.0.0.1",
			FreighterdPort: uint16(getFreighterdPort(container)),
			State:          getState(container),
		}
		if len(cn.Names) > 0 {
			h.Name = cn.Names[0]
		}
		ret = append(ret, h)
	}
	return ret, nil
}

func (h *Host) Stop() (*host.Info, error) {
	if err := h.client.StopContainer(h.ID, uint(5)); err != nil {
		switch err.(type) {
		case *dockerclient.ContainerNotRunning:
			return h.Info()
		default:
			return nil, err
		}
	}

	return h.Info()
}

func (h *Host) StopWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	host.PublishMessage(qp, event.HostStopping, h.requestID, queueInfo, "Stopping host", queue.Stop)

	if err := h.client.StopContainer(h.ID, uint(5)); err != nil {
		switch err.(type) {
		case *dockerclient.ContainerNotRunning:
			info, hostInfoErr := h.Info()
			if hostInfoErr == nil {
				host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, "Containers are not running")
			} else {
				logrus.Debugf("[ERROR] uuid=%s docker: failed to send a message due to failure on getting host info: %v", h.requestID, hostInfoErr)
			}
			return info, err
		default:
			info, hostInfoErr := h.Info()
			if hostInfoErr == nil {
				host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, err.Error())
			} else {
				logrus.Debugf("[ERROR] uuid=%s docker: failed to send a message due to failure on getting host info: %v", h.requestID, hostInfoErr)
			}
			return info, err
		}
	}

	info, err := h.Info()
	if err == nil {
		host.PublishHost(qp, event.HostStopped, h.requestID, queueInfo, info, queue.Stop, "")
	}

	return info, err
}

func (h *Host) Start() (*host.Info, error) {
	return h.start()
}

func (h *Host) Restart() (*host.Info, error) {
	if err := h.client.StopContainer(h.ID, uint(5)); err != nil {
		switch err.(type) {
		case *dockerclient.ContainerNotRunning:
			// Continue.
		default:
			return nil, err
		}
	}

	return h.start()
}

func (h *Host) DriverName() string {
	return "docker"
}

// Settings returns an empty settings.
// TODO: make docker provider uses this instead of the config file definition
// below
func (h *Host) Settings() *host.Settings {
	return &host.Settings{}
}

func (h *Host) SSHConfig() *host.SSHConfig {
	return nil
}

func (h *Host) SetRequestID(id string) {
	h.requestID = id
}

// configFile is used to marshal/unmarshal /etc/freighter/config.json.
type configFile struct {
	DataPath  string       `json:"data_path"`
	TLSVerify bool         `json:"tls_verify"`
	TLSCA     string       `json:"tls_ca"`
	TLSCert   string       `json:"tls_cert"`
	TLSKey    string       `json:"tls_key"`
	Secret    string       `json:"secret"`
	NotifyURL string       `json:"notify_url"`
	Listen    []listenSpec `json:"listen"`
}

type listenSpec struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`
}

func (h *Host) createImage() (string, error) {
	cfg := h.configFile()
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("unable to marshal config JSON, err: %v", err)
	}
	data := newTemplateData(h)
	data.Config = string(b)
	if h.driverConfig.RegistryCALocation != "" {
		d, err := ioutil.ReadFile(h.driverConfig.RegistryCALocation)
		if err != nil {
			return "", fmt.Errorf("unable to read registry CA cert file at %s, err: %v",
				h.driverConfig.RegistryCALocation, err)
		}
		data.RegistryCA = string(d)
	}

	buf := bytes.NewBuffer(nil)
	if err := dockerfileTemplate.Execute(buf, data); err != nil {
		return "", err
	}

	t := time.Now()
	inputbuf := bytes.NewBuffer(nil)
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
		OutputStream: ioutil.Discard, // not interested in docker building output
	}
	logrus.Debugf("docker: building image %s", imageName)
	if err := h.client.BuildImage(opts); err != nil {
		return "", err
	}
	return imageName, nil
}

func (h *Host) createContainer(imageName string) (*host.Info, error) {
	logrus.Debugf("docker: creating container from image %+s", imageName)
	container, err := h.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name: h.Name,
		Config: &dockerclient.Config{
			Hostname: h.Name,
			Image:    imageName,
			Cmd:      []string{"/start-freighterd"},
		},
	})
	if err != nil {
		return nil, err
	}
	h.ID = container.ID
	info, err := h.Start()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func New(driverOptions string, hostOptions string) (host.Host, error) {
	driverCfg := driverConfig{}
	if err := json.Unmarshal([]byte(driverOptions), &driverCfg); err != nil {
		logrus.Errorf("docker: invalid driver config: %s", driverOptions)
		return nil, err
	}

	_, err := exec.LookPath(driverCfg.FreighterdLocation)
	if err != nil {
		return nil, fmt.Errorf("Error finding freighterd binary at %q: %v", driverCfg.FreighterdLocation, err)
	}

	if dockerLocation := driverCfg.DockerLocation; dockerLocation != "" {
		if _, err := exec.LookPath(dockerLocation); err != nil {
			return nil, fmt.Errorf("Error finding docker binary at %q: %v", dockerLocation, err)
		}
	}

	endpoint := "unix:///var/run/docker.sock"
	if driverCfg.Endpoint != "" {
		endpoint = driverCfg.Endpoint
	}

	host := &Host{driverConfig: driverCfg}
	if err := json.Unmarshal([]byte(hostOptions), host); err != nil {
		return nil, err
	}

	client, err := dockerclient.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	host.client = client

	return host, nil
}

func (h *Host) start() (*host.Info, error) {
	binds := []string{
		fmt.Sprintf("%s:/freighterd", h.driverConfig.FreighterdLocation),
	}
	if h.driverConfig.DockerLocation != "" {
		binds = append(binds, fmt.Sprintf("%s:/usr/bin/docker", h.driverConfig.DockerLocation))
	}

	err := h.client.StartContainer(h.ID, &dockerclient.HostConfig{
		Privileged:      true,
		PublishAllPorts: true,
		Binds:           binds,
	})
	if err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) configFile() *configFile {
	return &configFile{
		DataPath:  "/var/lib/freighter",
		TLSVerify: true,
		TLSCA:     TLSCALocation,
		TLSCert:   TLSCertLocation,
		TLSKey:    TLSKeyLocation,
		Secret:    SecretLocation,
		NotifyURL: h.notifyURL(),
		Listen: []listenSpec{
			{Proto: "unix", Address: "/var/run/freighterd.sock"},
			{Proto: "tcp", Address: fmt.Sprintf("0.0.0.0:%d", DefaultPort)},
		},
	}
}

func (h *Host) notifyURL() string {
	return fmt.Sprintf("http://10.1.42.1:3333/api/v0/hosts/%s/freighterd_hook", h.Name)
}

func getFreighterdPort(container *dockerclient.Container) uint16 {
	portSpec := dockerclient.Port(fmt.Sprintf("%d/tcp", DefaultPort))
	freighterdPortMappings := container.NetworkSettings.Ports[portSpec]
	if len(freighterdPortMappings) != 1 {
		return uint16(0)
	}
	freighterdPort, err := strconv.Atoi(freighterdPortMappings[0].HostPort)
	if err != nil {
		return uint16(0)
	}
	return uint16(freighterdPort)
}

func getState(container *dockerclient.Container) state.State {
	if container.State.Running {
		return state.Running
	}
	return state.Stopped
}

func init() {
	host.Register("docker", &host.RegisteredDriver{
		New: New,
	})
}
