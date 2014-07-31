package docker

import (
	"fmt"
	"path/filepath"
	"strings"

	dockerclient "github.com/fsouza/go-dockerclient"

	"github.com/nitrous-io/freighter/provider"
)

var (
	ErrInvalidPortSpec  = fmt.Errorf("port binding invalid")
	ErrInvalidMountSpec = fmt.Errorf("volume specification invalid")
)

// CreateConfigs returns the dockerclient Config and HostConfig needed to
// create a container.  These can be customized by cfg ContainerConfig.
func (p *DockerProvider) CreateConfigs(containerName, imageName string, cfg *provider.ContainerConfig) (*dockerclient.Config, *dockerclient.HostConfig, error) {
	dcConfig := &dockerclient.Config{
		Hostname:     containerName,
		Image:        imageName,
		Volumes:      map[string]struct{}{},
		ExposedPorts: map[dockerclient.Port]struct{}{},
		Env:          []string{},
	}
	dcHostConfig := &dockerclient.HostConfig{
		Binds:           []string{},
		PortBindings:    map[dockerclient.Port][]dockerclient.PortBinding{},
		PublishAllPorts: true,
	}
	if cfg != nil {
		dcConfig.Env = cfg.Env
		bs, err := dockerPortBindings(cfg.PortBindings)
		if err != nil {
			return nil, nil, err
		}
		dcHostConfig.PortBindings = bs
		for port, _ := range bs {
			dcConfig.ExposedPorts[dockerclient.Port(port)] = struct{}{}
		}
		ms, err := dockerMountBinds(cfg.Mounts, p.VolumesDir(""))
		if err != nil {
			return nil, nil, err
		}
		dcHostConfig.Binds = ms
	}

	dcConfig.ExposedPorts[dockerclient.Port("22/tcp")] = struct{}{}
	dcConfig.Volumes["/etc/ssh/freighter"] = struct{}{}
	dcHostConfig.Binds = append(dcHostConfig.Binds, fmt.Sprintf("%s/ssh:/etc/ssh/freighter:ro", p.containerSettingsDir(containerName)))

	return dcConfig, dcHostConfig, nil
}

// StartConfig returns the dockerclient HostConfig needed to start a container.
// It can be customized by cfg ContainerConfig.
func (p *DockerProvider) StartConfig(container *dockerclient.Container, cfg *provider.ContainerConfig) (*dockerclient.HostConfig, error) {
	dcHostConfig := container.HostConfig
	if cfg == nil {
		return dcHostConfig, nil
	}
	// if config field is nil, ignore
	// if config field is non-nil but len == 0, override with empty
	if cfg.PortBindings != nil {
		bs, err := dockerPortBindings(cfg.PortBindings)
		if err != nil {
			return nil, err
		}
		dcHostConfig.PortBindings = bs
	}
	if cfg.Mounts != nil {
		ms, err := dockerMountBinds(cfg.Mounts, p.VolumesDir(""))
		if err != nil {
			return nil, err
		}
		dcHostConfig.Binds = ms
		dcHostConfig.Binds = append(dcHostConfig.Binds, fmt.Sprintf("%s/ssh:/etc/ssh/freighter:ro", p.containerSettingsDir(container.Config.Hostname)))
	}
	return dcHostConfig, nil
}

func dockerMountBinds(mountSpecs []string, hostVolsPath string) ([]string, error) {
	binds := []string{}
	for _, mspec := range mountSpecs {
		// TODO do we want to validate host volume existence at this point?
		bind, err := parseMountSpec(mspec, hostVolsPath)
		if err != nil {
			return []string{}, err
		}
		binds = append(binds, bind)
	}
	return binds, nil
}

// parseMountSpec supports any of:
//   volumeSlug:mountPoint
//   volumeSlug
// It appends the appropriate host path to the volume slug.  No validations on
// mountPoint are performed -- let docker handle.  If mountPoint is missing,
// mount to /volumes/<volumeSlug>
func parseMountSpec(mspec string, hostVolsPath string) (string, error) {
	var mountPoint string
	parts := strings.Split(mspec, ":")
	switch len(parts) {
	case 1:
		mountPoint = fmt.Sprintf("/volumes/%s", parts[0])
	case 2:
		mountPoint = parts[1]
	default:
		return "", ErrInvalidMountSpec
	}
	return fmt.Sprintf("%s:%s", filepath.Join(hostVolsPath, parts[0]), mountPoint), nil
}

func dockerPortBindings(portSpecs []string) (map[dockerclient.Port][]dockerclient.PortBinding, error) {
	bindings := map[dockerclient.Port][]dockerclient.PortBinding{}
	for _, portSpec := range portSpecs {
		p, pb, err := parsePortSpec(portSpec)
		if err != nil {
			return nil, err
		}
		if _, ok := bindings[p]; ok {
			bindings[p] = append(bindings[p], pb)
		} else {
			bindings[p] = []dockerclient.PortBinding{pb}
		}
	}
	return bindings, nil
}

// parsePortSpec supports any of:
//   ip:hostPort:containerPort
//   ip::containerPort
//   hostPort:containerPort ... :
func parsePortSpec(portSpec string) (dockerclient.Port, dockerclient.PortBinding, error) {
	parts := strings.Split(portSpec, ":")
	var hostIP, hostPort, containerPort string
	switch { // TODO more stringent checking
	case len(parts) == 3 && parts[1] == "": // ip::containerPort
		hostIP, hostPort, containerPort = parts[0], "", parts[2]
	case len(parts) == 3 && parts[1] != "": // ip:hostPort:containerPort
		hostIP, hostPort, containerPort = parts[0], parts[1], parts[2]
	case len(parts) == 2: // hostPort:containerPort
		hostIP, hostPort, containerPort = "127.0.0.1", parts[0], parts[1]
	default:
		return "", dockerclient.PortBinding{}, ErrInvalidPortSpec
	}
	return dockerclient.Port(containerPort),
		dockerclient.PortBinding{HostIp: hostIP, HostPort: hostPort}, nil
}
