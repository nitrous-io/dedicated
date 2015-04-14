package docker

import (
	"fmt"
	"path/filepath"
	"strings"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker/parsers"
	"github.com/nitrous-io/dedicated/pkg/uuid"
)

var (
	ErrInvalidPortSpec  = fmt.Errorf("port binding invalid")
	ErrInvalidMountSpec = fmt.Errorf("volume specification invalid")
)

type PortBindingsMap map[dockerclient.Port][]dockerclient.PortBinding

// Serializes docker portbindings back to
// provider.ContainerConfig's format.
//
// For running containers, the docker-assigned host port will be filled in, so
// returned portspecs are limited to:
//   public:hostPort:containerPort
//   private:hostPort:containerPort
func serializePortBindings(bindingsMap PortBindingsMap) []string {
	resultBindings := []string{}
	for containerPort, portBindings := range bindingsMap {
		// portBindings is nil when port is exposed with no
		// HostConfig.PortBinding and HostConfig.PublishAllPorts is false
		// TODO investigate how else to resolve
		if portBindings != nil {
			for _, binding := range portBindings {
				access := "private"
				if binding.HostIp == "0.0.0.0" {
					access = "public"
				}

				resultBindings = append(resultBindings, fmt.Sprintf("%s:%s:%s",
					access, binding.HostPort, string(containerPort)))
			}
		}
	}

	return resultBindings
}

// Get a normalized version of HostConfig.PortBindings
// that include all exposed ports. This is necessary because
// the original exposed ports specified inside Dockerfile
// are not included in the config port bindings list
func normalizedConfigPortBindings(container *dockerclient.Container, gateway string) PortBindingsMap {
	configPortBindings := container.HostConfig.PortBindings
	if configPortBindings == nil {
		configPortBindings = make(PortBindingsMap)
	}

	exposedPorts := container.Config.ExposedPorts

	hostIp := gateway
	if container.HostConfig.PublishAllPorts {
		hostIp = "0.0.0.0"
	}

	for port, _ := range exposedPorts {
		if _, ok := configPortBindings[port]; !ok {
			configPortBindings[port] = []dockerclient.PortBinding{
				{
					HostPort: "",
					HostIp:   hostIp,
				},
			}
		}
	}

	return configPortBindings
}

// configFromContainer populates a ContainerConfig given a container
func (p *DockerProvider) configFromContainer(container *dockerclient.Container) *provider.ContainerConfig {
	configPortBindings := normalizedConfigPortBindings(container, p.gateway)
	runtimePortBindings := container.NetworkSettings.Ports

	serializedConfigPortBindings := serializePortBindings(configPortBindings)
	serializedRuntimePortBindings := serializedConfigPortBindings
	if container.State.Running {
		serializedRuntimePortBindings = serializePortBindings(runtimePortBindings)
	}

	containerConfig := &provider.ContainerConfig{
		Privileged:         &container.HostConfig.Privileged,
		Cmd:                container.Config.Cmd,
		Env:                container.Config.Env,
		PublishAll:         &container.HostConfig.PublishAllPorts,
		Mounts:             mountBindsFromDocker(container.HostConfig.Binds, p.VolumesDir("")),
		PortBindings:       serializedRuntimePortBindings,
		ConfigPortBindings: serializedConfigPortBindings,
	}

	return containerConfig
}

func (p *DockerProvider) mkContainerConfig(cfg *provider.ContainerConfig, isTrustedImage bool) (*dockerclient.Config, error) {
	dcConfig := &dockerclient.Config{
		Hostname:     cfg.Hostname,
		Image:        cfg.Image,
		ExposedPorts: map[dockerclient.Port]struct{}{},
		OpenStdin:    true,
		Tty:          true,
	}

	dcConfig.Env = make([]string, len(cfg.Env))
	copy(dcConfig.Env, cfg.Env)

	dcConfig.Cmd = make([]string, len(cfg.Cmd))
	copy(dcConfig.Cmd, cfg.Cmd)

	bs, err := parsers.ParsePortBindings(cfg.PortBindings, p.gateway)
	if err != nil {
		return nil, err
	}
	// all exposed ports from the container will need to be saved here,
	// otherwise docker will not create the iptables rules and the local proxy
	// even if it saves the port pairs ( as of v 1.4.1 ).
	// The actual public:private ports' rules will then be calculated from the
	// HostConfig.
	for port, _ := range bs {
		dcConfig.ExposedPorts[dockerclient.Port(port)] = struct{}{}
	}

	if isTrustedImage {
		dcConfig.ExposedPorts[dockerclient.Port("22/tcp")] = struct{}{}
	}

	return dcConfig, nil
}

func (p *DockerProvider) mkHostConfig(container *dockerclient.Container, cfg *provider.ContainerConfig, isTrustedImage bool) (*dockerclient.HostConfig, error) {
	newCfg := dockerclient.HostConfig{}
	hostCfg := container.HostConfig
	if hostCfg != nil {
		newCfg = *hostCfg
	}

	// Set defaults.
	if newCfg.Binds == nil {
		newCfg.Binds = []string{}
	}
	if newCfg.PortBindings == nil {
		newCfg.PortBindings = map[dockerclient.Port][]dockerclient.PortBinding{}
	}

	if cfg == nil {
		return &newCfg, nil
	}

	newCfg.Privileged = hostCfg.Privileged
	if cfg.Privileged != nil {
		newCfg.Privileged = *cfg.Privileged
	}

	newCfg.PublishAllPorts = hostCfg.PublishAllPorts
	if isTrustedImage {
		// Publish all exposed ports so that port 22 is published.
		newCfg.PublishAllPorts = true
	}
	if cfg.PublishAll != nil {
		newCfg.PublishAllPorts = *cfg.PublishAll
	}

	if cfg.PortBindings != nil {
		bs, err := parsers.ParsePortBindings(cfg.PortBindings, p.gateway)
		if err != nil {
			return nil, err
		}
		newCfg.PortBindings = bs
	}

	if cfg.Mounts != nil {
		ms, err := mountBindsToDocker(cfg.Mounts, p.VolumesDir(""))
		if err != nil {
			return nil, err
		}
		newCfg.Binds = ms
	}
	// add built-in volumes. This is only valid when the container is started
	// for the first time. A reconfigured container will not triggered this as
	// built-in volumes will be reset. Therefore, it's safe to generate a
	// probably one-time-unique slug for the volume instead of all-time-unique
	// slug

	if container.Config != nil {
		for vname := range container.Config.Volumes {
			if vname == nitrousSSHFilepath || vname == nitrousSlugFilepath {
				continue
			}
			// container's name started with `/` which we dont want
			slug := fmt.Sprintf("%s-%s", container.Name[1:], uuid.Make())
			vinfo, err := p.CreateVolume(slug)
			if err != nil {
				return nil, fmt.Errorf("failed to create volume %q(%q): %s", vname, slug, err)
			}
			addVolume(p.VolumesDir(vinfo.Slug), vname, &newCfg)
		}
	}

	// add required nitrous configs
	addVolume(p.ContainerSettingsDir(cfg.Hostname)+"/ssh", nitrousSSHFilepath+":ro", &newCfg)
	addVolume(p.ContainerSettingsDir(cfg.Hostname)+"/"+nitrousSlugFilename, "/"+nitrousSlugFilename+":ro", &newCfg)

	// ExtraHosts is only available in Docker 1.3.0+.
	newCfg.ExtraHosts = append(newCfg.ExtraHosts, fmt.Sprintf("host:%s", p.gateway))

	return &newCfg, nil
}

func addVolume(hostPath, containerPath string, cfg *dockerclient.HostConfig) {
	hasBind := false
	for _, bind := range cfg.Binds {
		if strings.HasSuffix(bind, containerPath) {
			hasBind = true
			break
		}
	}
	if !hasBind {
		cfg.Binds = append(cfg.Binds, hostPath+":"+containerPath)
	}
}

func mountBindsToDocker(mountSpecs []string, hostVolsPath string) ([]string, error) {
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

func mountBindsFromDocker(dockerSpecs []string, hostVolsPath string) []string {
	binds := []string{}
	for _, spec := range dockerSpecs {
		parts := strings.Split(spec, ":")
		if len(parts) == 3 { // hostPath:containerPath:ro style spec
			binds = append(binds, spec)
			continue
		}
		if len(parts) != 2 {
			continue
		}

		if !strings.HasPrefix(parts[0], hostVolsPath) { // non-nitrous style spec
			binds = append(binds, spec)
			continue
		}

		// nitrous volume handling
		// looks like "/var/lib/freighter/volumes/database-75:/var/lib/postgresql/data"
		slug := filepath.Base(parts[0])
		if parts[0] == filepath.Join(hostVolsPath, slug) && parts[1] == filepath.Join("/volumes", slug) {
			binds = append(binds, slug) // volumeSlug spec
			continue
		}
		binds = append(binds, fmt.Sprintf("%s:%s", slug, parts[1])) // volumeSlug:mountPoint spec
	}
	return binds
}

// parseMountSpec supports any of:
//   volumeSlug:mountPoint
//   volumeSlug
//   hostPath:containerPath:ro
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
	case 3:
		return mspec, nil
	default:
		return "", ErrInvalidMountSpec
	}
	return fmt.Sprintf("%s:%s", filepath.Join(hostVolsPath, parts[0]), mountPoint), nil
}
