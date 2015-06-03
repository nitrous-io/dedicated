package docker

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

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

type Host struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	PublicKeys         []string   `json:"public_keys"`
	IpAddress          string     `json:"ip_address"`
	FreighterdLocation string     `json:"freighterd_location"`
	DockerLocation     string     `json:"docker_location"`
	TLS                *tlsConfig `json:"tls"`
	Secret             string     `json:"secret"`

	uuid   string // request UUID set by user
	client *dockerclient.Client
}

// Create creates the docker container which acts as a container host, and write the
// progress into w
func (h *Host) Create(w io.Writer) (*host.Info, error) {
	w = io.MultiWriter(os.Stdout, w)
	for i, k := range h.PublicKeys {
		h.PublicKeys[i] = strings.TrimSpace(k)
	}
	fmt.Fprintf(w, "Creating host image")
	imageName, err := h.createImage()
	if err != nil {
		fmt.Fprintln(w, "Failed to create host image")
		return nil, err
	}
	fmt.Fprintf(w, "Created host image")
	fmt.Fprintf(w, "Creating host with image “%s”", imageName)

	c, err := h.createContainer(imageName)
	if err != nil {
		fmt.Fprintf(w, "Failed to create host")
		return nil, err
	}
	fmt.Fprintf(w, "Host created")
	return c, nil
}

// Create creates the docker container which acts as a container host, and write the
// progress into queue
func (h *Host) CreateWithQueue(qp *queue.QueuePublisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	for i, k := range h.PublicKeys {
		h.PublicKeys[i] = strings.TrimSpace(k)
	}
	host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Creating host")
	imageName, err := h.createImage()
	if err != nil {
		host.PublishMessage(qp, event.HostCreateFailed, h.uuid, queueInfo, "Failed to create host's image")
		return nil, err
	}
	host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Created host image")

	c, err := h.createContainer(imageName)
	if err != nil {
		host.PublishMessage(qp, event.HostCreateFailed, h.uuid, queueInfo, "Failed to create host's container")
		return nil, err
	}
	host.PublishHost(qp, event.HostCreated, h.uuid, queueInfo, c)
	return c, nil
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	mw := io.MultiWriter(os.Stdout, w)

	container, err := h.client.InspectContainer(h.ID)
	if err != nil {
		return nil, err
	}

	if container.Name != h.Name {
		// Update /etc/freighterd/config.json's with new host slug.
		err := h.updateConfigFile(container, mw)
		if err != nil {
			fmt.Fprintf(mw, "Failed to update container's config file, err: %v\n", err)
			return nil, err
		}

		// TODO We can replace this commit -> create -> remove dance with native
		// Docker rename in 1.5.0.
		fmt.Fprintf(mw, "Renaming container from %q to %q\n", container.Name, h.Name)
		fmt.Fprintln(mw, "Committing container")
		img, err := h.client.CommitContainer(dockerclient.CommitContainerOptions{
			Container:  container.ID,
			Repository: fmt.Sprintf("renamed-%s", container.ID),
			Tag:        "latest",
		})
		if err != nil {
			fmt.Fprintf(mw, "Failed to commit container, err: %v\n", err)
			return nil, err
		}
		fmt.Fprintf(mw, "Container committed to image %s\n", img.ID)

		c, err := h.createContainer(img.ID)
		if err != nil {
			fmt.Fprintf(mw, "Failed to create container from committed image, err: %v\n", err)
			return nil, err
		}

		// Remove old container.
		if err := h.client.RemoveContainer(dockerclient.RemoveContainerOptions{
			ID:    container.ID,
			Force: true,
		}); err != nil {
			// Log and proceed anyway.
			fmt.Fprintf(mw, "Failed to remove old container, err: %v\n", err)
		}

		fmt.Fprintf(mw, "Container renamed. New ID: %s, old ID: %s)\n", c.ID, container.ID)
		return c, nil
	}

	return &host.Info{
		Provider:       h.DriverName(),
		ID:             h.ID,
		Name:           h.Name,
		IPAddress:      "127.0.0.1",
		FreighterdPort: uint16(getFreighterdPort(container)),
		State:          getState(container),
	}, nil
}

func (h *Host) Respec() (*host.Info, error) {
	return h.Info()
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
		OutputStream: os.Stdout,
	}
	log.Printf("docker: building image %s", imageName)
	if err := h.client.BuildImage(opts); err != nil {
		return "", err
	}
	return imageName, nil
}

func (h *Host) createContainer(imageName string) (*host.Info, error) {
	log.Printf("docker: creating container from image %+s", imageName)
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

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	host := &Host{
		uuid: defaultConfig["uuid"], // this is injected by the server per request
	}
	if err := json.Unmarshal([]byte(config), host); err != nil {
		return nil, err
	}
	// fetch docker client endpoint from defaultConfig
	endpoint, ok := defaultConfig["endpoint"]
	if !ok {
		endpoint = "unix:///var/run/docker.sock"
	}

	freighterdLocation, _ := defaultConfig["freighterd_location"]
	_, err := exec.LookPath(freighterdLocation)
	if err != nil {
		return nil, fmt.Errorf("Error finding freighterd binary at %s: %v", freighterdLocation, err)
	}

	host.FreighterdLocation = freighterdLocation

	dockerLocation, _ := defaultConfig["docker_location"]
	if dockerLocation != "" {
		if _, err := exec.LookPath(dockerLocation); err != nil {
			return nil, fmt.Errorf("Error finding docker binary at %q: %s", dockerLocation, err)
		}
		host.DockerLocation = dockerLocation
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
		fmt.Sprintf("%s:/freighterd", h.FreighterdLocation),
	}
	if h.DockerLocation != "" {
		binds = append(binds, fmt.Sprintf("%s:/usr/bin/docker", h.DockerLocation))
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

func (h *Host) updateConfigFile(container *dockerclient.Container, w io.Writer) error {
	cfg := &configFile{}

	out, err := exec.Command("docker", "exec", container.ID,
		"/bin/bash", "-c", fmt.Sprintf(`[ -f %s ]`, ConfigLocation)).CombinedOutput()
	if err != nil {
		fmt.Fprintf(w, "Failed to determine if config file exists, err: %v, output: %s\n", err, out)
		cfg = h.configFile()
	} else {
		// No error means that the file exists, so we read its contents.
		out, err := exec.Command("docker", "exec", container.ID, "cat", ConfigLocation).CombinedOutput()
		if err != nil {
			fmt.Fprintf(w, "Failed to read config file, err: %v, output: %s\n", err, out)
			return err
		}
		if err := json.Unmarshal(out, cfg); err != nil {
			fmt.Fprintf(w, "Failed to unmarshal config file, err: %v\n", err)
			return err
		}
		cfg.NotifyURL = h.notifyURL()
	}

	buf, err := json.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(w, "Failed to marshal config file, err: %v\n", err)
		return err
	}
	out, err = exec.Command("docker", "exec", container.ID, "/bin/bash", "-c",
		fmt.Sprintf("echo -e %q > %s", buf, ConfigLocation)).CombinedOutput()
	if err != nil {
		fmt.Fprintf(w, "Failed to write to config file, err: %v, output: %s\n", err, out)
		return err
	}

	return nil
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
	return fmt.Sprintf("http://172.17.42.1:3333/api/v0/hosts/%s/freighterd_hook", h.Name)
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
	host.Hosts["docker"] = NewHost
}
