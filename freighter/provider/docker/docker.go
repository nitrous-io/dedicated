package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/freighter/cops"
	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker/parsers"
	"github.com/nitrous-io/dedicated/freighter/util"
	"github.com/nitrous-io/nitrous-common/hmactoken"
)

var (
	dockerHost = "unix:///var/run/docker.sock"
)

const (
	nitrousEnvImage     = "NITROUS_IMAGE_VERSION"
	nitrousEnvUser      = "NITROUS_USERNAME"
	nitrousSlugFilename = ".nitrous-container-slug"
)

var preservedEnvVars = []string{nitrousEnvImage, nitrousEnvUser}

const defaultDockerRegistry = "index.docker.io"
const dockerRegistryImageEndpoint = "registry-1.docker.io"

func init() {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		dockerHost = host
	}
}

type TemplateData struct {
	Name  string
	Image string
}

type DockerProvider struct {
	client   *dockerclient.Client
	dataPath string
	gateway  string
	logger   *util.Logger

	DockerClientProxy interface {
		PushImage(opts dockerclient.PushImageOptions, auth dockerclient.AuthConfiguration) error
		PullImage(opts dockerclient.PullImageOptions, auth dockerclient.AuthConfiguration) error
	}
	DockerConfigLocation string
}

type lookupImageResult struct {
	image *provider.ImageInfo
	byID  bool
}

func NewDockerProvider(logger *util.Logger, dataPath string) (*DockerProvider, error) {
	if logger == nil || dataPath == "" {
		return nil, errors.New("docker.NewDockerProvider: invalid argument")
	}

	client, err := dockerclient.NewClient(dockerHost)
	if err != nil {
		return nil, err
	}

	i := 0 // use counter to throttle logging
	for {  // check every second until docker is available
		if _, err := client.Version(); err == nil {
			break
		}
		i++
		if i%30 == 0 {
			i = 0
			logger.Warnf("DockerProvider: waiting for Docker to be available on %s", dockerHost)
		}
		time.Sleep(1 * time.Second)
	}

	out, err := exec.Command("ip", "-family", "inet", "addr", "show", "docker0").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Error determining docker gateway %v", err)
	}
	dockerGateway, err := gatewayFromIPOutput(out)
	if err != nil {
		return nil, fmt.Errorf("Error determining docker gateway: %v", err)
	}

	return &DockerProvider{
		client:            client,
		gateway:           dockerGateway,
		dataPath:          dataPath,
		logger:            logger,
		DockerClientProxy: client,
	}, nil
}

func (p *DockerProvider) VerifyToken(id, token string, permissions ...string) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	secret, err := p.readSharedSecret(nameFromContainer(container))
	if err != nil {
		return provider.ErrUnverifiable
	}
	err = hmactoken.Verify(permissions, token, secret)
	switch err {
	case hmactoken.ErrInvalidToken, hmactoken.ErrPermissionDenied:
		return provider.ErrTokenVerificationFailed
	case hmactoken.ErrTokenExpired:
		return provider.ErrTokenExpired
	default:
		return err
	}
}

func (p *DockerProvider) List() ([]*provider.ContainerInfo, error) {
	apicontainers, err := p.client.ListContainers(dockerclient.ListContainersOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}

	list := []*provider.ContainerInfo{}

	for _, apicontainer := range apicontainers {
		info, err := p.GetInfo(apicontainer.ID)
		if err != nil {
			return nil, err
		}
		list = append(list, info)
	}

	return list, nil
}

// CreateStart creates and starts a container.
// If the container cannot be started, the container info *and* a
// provider.StartFailedError is returned.
func (p *DockerProvider) CreateStart(name, imageName, secret, configJSON string) (*provider.ContainerInfo, error) {
	createdCnr, err := p.create(name, imageName, secret, configJSON)
	if err != nil {
		p.logger.Errorf("DockerProvider: failed to create container %s: %v", name, err)
		return nil, err
	}

	startedCnr, err := p.Start(createdCnr.ID, configJSON)
	if err != nil {
		// Return container info as well if container was created but failed
		// to start.
		p.logger.Errorf("DockerProvider: failed to start container %s: %v", name, err)
		info, _ := p.GetInfo(createdCnr.ID)
		return info, &provider.StartFailedError{Err: err}
	}

	return startedCnr, nil
}

func (p *DockerProvider) create(name, imageName, secret, configJSON string) (*provider.ContainerInfo, error) {
	if err := p.storeSharedSecret(name, secret); err != nil {
		return nil, err
	}

	if err := p.storeContainerName(name); err != nil {
		return nil, err
	}

	ak := p.containerAuthorizedKeys(name)
	if err := ak.Write([]string{}); err != nil {
		return nil, err
	}

	cfg, err := containerConfigFromJSON(configJSON)
	if err != nil {
		return nil, err
	}

	p.setDefaultContainerConfig(cfg)
	cfg.Hostname = name
	cfg.Image = imageName

	createConfig, err := p.mkContainerConfig(cfg, p.isNitrousImage(cfg.Image))
	if err != nil {
		return nil, err
	}

	container, err := p.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name:   name,
		Config: createConfig,
	})
	if err != nil {
		if err == dockerclient.ErrNoSuchImage {
			return nil, provider.ErrImageNotFound
		}
		return nil, err
	}
	return p.GetInfo(container.ID)
}

func (p *DockerProvider) Commit(id, imageName string) (*provider.ImageInfo, error) {
	repo, tag := parsers.ParseRepoAndTag(imageName)
	if tag == "" {
		tag = "latest"
	}

	opts := dockerclient.CommitContainerOptions{
		Container:  id,
		Repository: repo,
		Tag:        tag,
	}
	dockerImage, err := p.client.CommitContainer(opts)
	if err != nil {
		return nil, err
	}

	info, err := p.lookupImage(dockerImage.ID)
	if err != nil {
		return nil, err
	}
	return info.image, nil
}

func (p *DockerProvider) GetConfig(id string) (*provider.ContainerConfig, error) {
	cnr, err := p.inspectContainer(id)
	if err != nil {
		return nil, err
	}
	return p.configFromContainer(cnr), nil
}

func (p *DockerProvider) GetInfo(id string) (*provider.ContainerInfo, error) {
	container, err := p.inspectContainer(id)
	if err != nil {
		return nil, err
	}

	// get volumes created in /var/lib/freighter/volumes and mounted to this
	// container
	volumes := make(map[string]string)
	for vname, dest := range container.Volumes {
		if filepath.Dir(dest) == p.VolumesDir("") {
			volumes[vname] = filepath.Base(dest)
		}
	}

	return &provider.ContainerInfo{
		ID:         container.ID,
		Name:       nameFromContainer(container),
		Image:      container.Config.Image,
		ImageID:    container.Image,
		Host:       container.NetworkSettings.IPAddress,
		Gateway:    container.NetworkSettings.Gateway,
		Ports:      portsFromContainer(container),
		Env:        container.Config.Env,
		State:      stateFromContainer(container),
		Cmd:        container.Config.Cmd,
		Entrypoint: container.Config.Entrypoint,
		ExitCode:   container.State.ExitCode,
		User:       container.Config.User,
		StartedAt:  container.State.StartedAt,
		FinishedAt: container.State.FinishedAt,
		Volumes:    volumes,
	}, nil
}

func (p *DockerProvider) Start(id, configJSON string) (*provider.ContainerInfo, error) {
	container, err := p.inspectContainer(id)
	if err != nil {
		return nil, err
	}

	if err := p.storeContainerName(nameFromContainer(container)); err != nil {
		return nil, err
	}

	if !container.State.Running {
		cfg, err := containerConfigFromJSON(configJSON)
		if err != nil {
			return nil, err
		}

		p.setDefaultContainerConfig(cfg)
		cfg.Hostname = container.Config.Hostname
		cfg.Image = container.Config.Image

		startConfig, err := p.mkHostConfig(container, cfg, p.isNitrousImage(cfg.Image))
		if err != nil {
			return nil, err
		}
		p.logger.Info("DockerProvider: starting container", id)
		start := time.Now()
		if err := p.client.StartContainer(id, startConfig); err != nil {
			return nil, err
		}
		p.logger.TimeInfo(start, "DockerProvider: started container", id, "in")
	}

	return p.GetInfo(id)
}

func (p *DockerProvider) Stop(id string) (*provider.ContainerInfo, error) {
	container, err := p.inspectContainer(id)
	if err != nil {
		return nil, err
	}

	if container.State.Running {
		p.logger.Info("DockerProvider: stopping container", id)
		start := time.Now()

		err := p.client.StopContainer(id, 15)
		if err != nil {
			return nil, err
		}

		ch := make(chan error, 1)

		go func() {
			timeout := time.After(15 * time.Second)
			for {
				select {
				case <-timeout:
					ch <- provider.ErrTimeout
					return
				default:
					info, err := p.GetInfo(id)
					if err != nil {
						ch <- err
						return
					}
					if info.State == provider.ContainerStateStopped && info.Host == "" {
						p.logger.TimeInfo(start, "DockerProvider: stopped container", id)
						ch <- nil
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}()

		if err := <-ch; err != nil {
			return nil, err
		}
	}

	return p.GetInfo(id)
}

func (p *DockerProvider) Restart(id string) (*provider.ContainerInfo, error) {
	p.logger.Info("DockerProvider: restarting container", id)
	start := time.Now()
	// Stop and then start until https://github.com/docker/docker/issues/7693
	// is fixed.
	_, err := p.Stop(id)
	if err != nil {
		return nil, err
	}
	info, err := p.Start(id, "")
	if err != nil {
		return nil, err
	}
	p.logger.TimeInfo(start, "DockerProvider: restarted container", id, "in")
	return info, nil
}

func execForm(cmd, usr string) []string {
	if usr != "" {
		// When running command as a different user, we need:
		// 1. a login shell (so that /etc/profile, etc. are read)
		// 2. to preserve Docker env vars.
		return []string{
			"su", usr,
			"-c", cmd,
			"--login",                // Use a login shell so that /etc/profile, etc. are loaded.
			"--preserve-environment", // To preserve Docker env vars.
		}
	}
	return []string{"/bin/sh", "-c", cmd}
}

func (p *DockerProvider) Terminate(id string, removeSettings bool) (*provider.ContainerInfo, error) {
	p.logger.Info("DockerProvider: terminating container", id)
	terminationStart := time.Now()

	info, err := p.Stop(id)
	if err != nil {
		return nil, err
	}

	p.logger.Info("DockerProvider: removing container", id)
	start := time.Now()

	err = p.client.RemoveContainer(dockerclient.RemoveContainerOptions{ID: id})
	if err != nil {
		return nil, err
	}

	if removeSettings {
		if err := os.RemoveAll(p.ContainerSettingsDir(info.Name)); err != nil {
			return nil, err
		}
	}

	p.logger.TimeInfo(start, "DockerProvider: removed container", id, "in")

	info.Host = ""
	info.Gateway = ""
	info.State = ""
	info.Ports = nil

	p.logger.TimeInfo(terminationStart, "DockerProvider: terminated container", id, "in")
	return info, nil
}

func (p *DockerProvider) ListUserKeys(id string) (map[string]string, error) {
	container, err := p.inspectContainer(id)
	if err != nil {
		return nil, err
	}

	ak := p.containerAuthorizedKeys(nameFromContainer(container))
	keys, err := ak.Read()
	if err != nil {
		return nil, err
	}

	return keysToKeyMap(keys)
}

func (p *DockerProvider) AddUserKeys(id string, publicKeys []string) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	ak := p.containerAuthorizedKeys(nameFromContainer(container))
	return ak.Add(publicKeys)
}

func (p *DockerProvider) ClearUserKeys(id string) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	ak := p.containerAuthorizedKeys(nameFromContainer(container))
	return ak.Clear()
}

func (p *DockerProvider) RemoveUserKey(id string, keyFingerprint string) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	ak := p.containerAuthorizedKeys(nameFromContainer(container))
	return ak.Remove(keyFingerprint)
}

// at the moment there's only one key, but we're ready to return multiple
func (p *DockerProvider) ListContainerKeys(containerId string) ([]string, error) {
	container, err := p.inspectContainer(containerId)
	if err != nil {
		return nil, err
	}
	if !container.State.Running {
		return nil, provider.ErrContainerNotRunning
	}
	userName, exists := p.appUser(container)
	if !exists {
		return nil, provider.ErrPubKeyOpsNotSupported
	}
	if !p.isNitrousImage(container.Config.Image) {
		return nil, provider.ErrPubKeyOpsNotSupported
	}
	keyPath := fmt.Sprintf("/home/%s/.ssh/id_rsa.pub", userName)
	k, err := p.ReadFile(containerId, keyPath)
	if err != nil {
		return nil, err
	}
	k = strings.TrimSpace(k)
	return []string{k}, nil
}

// This is a copy of the Image struct in docker/image for unmarshalling JSON in
// /var/lib/docker/graph/<imageid>/json.
// FIXME This is fucking ugly though. But I don't know of a better way to edit
// only specific fields in /var/lib/docker/graph/<imageid>/json other than
// regexes and string replacement shenanigans.
type dockerImage struct {
	ID              string              `json:"id"`
	Parent          string              `json:"parent,omitempty"`
	Comment         string              `json:"comment,omitempty"`
	Created         time.Time           `json:"created"`
	Container       string              `json:"container,omitempty"`
	ContainerConfig dockerclient.Config `json:"container_config,omitempty"`
	DockerVersion   string              `json:"docker_version,omitempty"`
	Author          string              `json:"author,omitempty"`
	Config          dockerclient.Config `json:"config,omitempty"`
	Architecture    string              `json:"architecture,omitempty"`
	OS              string              `json:"os,omitempty"`
	Checksum        string              `json:"checksum"`
	Size            int64
}

type reconfigState int

const (
	stopped        reconfigState = iota + 1 // indicates old container has been stopped
	terminateError                          // indicates there was an error while terminating old container
	terminated                              // indicates old container has been terminated
	created                                 // indicates new container has been created
)

// Reconfig recreates a container from its original image with new config,
// since at this time docker container configs are frozen once set.
// Reconfig *always* returns the container so that callers know what the
// container is, since the container can be in various states.
//
// This is the current implementation:
//   1. Stop container.
//   2. Commit container to image.
//   3. Rewrite /var/lib/docker/graph/<imageid>/json with configJSON (leaving fields we don't care about unchanged).
//   4. Terminates the old container.
//   5. Create a new container from the image of the old container.
func (p *DockerProvider) Reconfig(id, configJSON string) (ret *provider.ContainerInfo, err error) {
	var (
		state           reconfigState
		cnName          string
		backupImageName string
		secret          string
	)
	defer func() {
		if err == nil {
			return
		}

		// Recover from errors.
		p.logger.Errorf("reconfig: failed to reconfigure %s: %v", id, err)

		switch state {
		case created:
			// Do nothing.
		case terminated:
			// Restore from backup image.
			if cn, err := p.CreateStart(cnName, backupImageName, secret, ""); err != nil {
				p.logger.Errorf("reconfig: failed to restore original container %s from %s after failure: %v",
					id, backupImageName, err)
			} else {
				ret = cn
			}
		case terminateError:
			p.logger.Infof("reconfig: error while terminating original container: %v", err)

			// When we run into an error terminating the old container, either:
			// 1. old container was actually terminated
			//    -> restore from backup image.
			// 2. old container was not terminated
			//    -> try to start old container, otherwise restore from backup image.
			// This sometimes happens when we run into this bug:
			//   https://github.com/docker/docker/issues/2714
			//   https://github.com/docker/docker/issues/3823

			// Handle old container actually terminated.
			if _, err := p.GetInfo(id); err != nil {
				// Restore from backup image.
				p.logger.Infof("reconfig: error getting container info: %v", err)

				if cn, err := p.CreateStart(cnName, backupImageName, secret, ""); err != nil {
					p.logger.Errorf("reconfig: failed to restore original container %s from %s after failure: %v",
						id, backupImageName, err)
				} else {
					ret = cn
				}
				return
			}

			// Handle old container not terminated.
			if cn, err := p.Start(id, ""); err != nil {
				// Restore from backup image.
				p.logger.Infof("reconfig: error starting original container %s: %v", id, err)

				if cn, err := p.CreateStart(cnName, backupImageName, secret, ""); err != nil {
					p.logger.Errorf("reconfig: failed to restore original container %s from %s after failure: %v",
						id, backupImageName, err)
				} else {
					ret = cn
				}
			} else {
				ret = cn
			}
		case stopped:
			if cn, err := p.Start(id, ""); err != nil {
				p.logger.Errorf("reconfig: failed to start original container %s after failure: %v", id, err)
			} else {
				ret = cn
			}
		}
	}()

	ret, err = p.Stop(id)
	if err != nil {
		if cn, e := p.GetInfo(id); e == nil {
			ret = cn
		}
		return ret, err
	}
	state = stopped
	cnName = ret.Name

	secret, err = p.readSharedSecret(cnName)
	if err != nil {
		return ret, err
	}

	ak := p.containerAuthorizedKeys(cnName)
	existingKeys, err := ak.Read()
	if err != nil {
		return ret, err
	}

	backupImageName = fmt.Sprintf("%s-backup-%d", cnName, time.Now().UnixNano())
	_, err = p.Commit(id, backupImageName)
	if err != nil {
		return ret, err
	}

	imageName := fmt.Sprintf("%s-reconfig-%d", cnName, time.Now().UnixNano())
	img, err := p.Commit(id, imageName)
	if err != nil {
		return ret, err
	}

	// Unmarshal /var/lib/docker/graph/<imageid>/json.
	// This assumes the docker daemon we're interacting with is running locally.
	jsonPath := fmt.Sprintf("/var/lib/docker/graph/%s/json", img.ID)
	b, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return ret, err
	}

	var graph dockerImage
	err = json.Unmarshal(b, &graph)
	if err != nil {
		return ret, err
	}

	// Reset configs that are additive so that we can replace them when creating
	// a container from this image.
	graph.Config.Volumes = nil
	graph.Config.PortSpecs = nil
	graph.Config.ExposedPorts = nil

	// Env is a special case because we rely on it to apply some defaults to
	// Nitrous-blessed images based on Nitrous-specific env vars.
	// FIXME This sucks because maintaining a list of env vars to preserve is
	// pretty fragile.
	newEnv := []string{}
	// Preserve Nitrous-specific env vars.
	for _, envKey := range preservedEnvVars {
		if val, ok := envValue(graph.Config.Env, envKey); ok {
			newEnv = append(newEnv, fmt.Sprintf("%s=%s", envKey, val))
		}
	}
	// Copy over all env vars from new config.
	cfg, err := containerConfigFromJSON(configJSON)
	if err != nil {
		return ret, err
	}
	for _, v := range cfg.Env {
		newEnv = append(newEnv, v)
	}
	graph.Config.Env = newEnv

	// Write new config back to /var/lib/docker/graph/<imageid>/json.
	f, err := os.OpenFile(jsonPath, os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return ret, err
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(graph)
	if err != nil {
		return ret, err
	}

	// Terminate the existing container.
	_, err = p.Terminate(id, false)
	if err != nil {
		state = terminateError
		return ret, err
	}
	state = terminated

	errf := "reconfig failed on %s during %s after termination: %s" // (id, step, error)
	newCn, err := p.create(cnName, img.Name, secret, configJSON)
	if err != nil {
		return ret, fmt.Errorf(errf, id, "create", err)
	}
	ret = newCn
	state = created

	err = p.AddUserKeys(newCn.ID, existingKeys)
	if err != nil {
		return ret, fmt.Errorf(errf, id, "public_keys", err)
	}

	startedCn, err := p.Start(newCn.ID, configJSON)
	if err != nil {
		return ret, &provider.StartFailedError{Err: err}
	}

	return startedCn, nil
}

// Logs gets stdout and stderr logs from a container. If successful, the logs
// are written to the given io.Writer.
func (p *DockerProvider) Logs(id string, w io.Writer, follow, timestamps bool, tail string) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	return p.client.Logs(dockerclient.LogsOptions{
		Container:    container.ID,
		Follow:       follow,
		Stdout:       true,
		Stderr:       true,
		Timestamps:   timestamps,
		Tail:         tail,
		OutputStream: w,
		ErrorStream:  w,
		RawTerminal:  true,
	})
}

func (p *DockerProvider) Login(username, password, email, registry string) error {
	args := []string{
		"login",
		"--username", username,
		"--password", password,
		"--email", email,
	}
	if registry != "" {
		args = append(args, registry)
	}
	return exec.Command("docker", args...).Run()
}

// Added to Docker with PR https://github.com/dotcloud/docker/pull/6719
func (p *DockerProvider) Logout(registry string) error {
	args := []string{
		"logout",
	}
	if registry != "" {
		args = append(args, registry)
	}
	return exec.Command("docker", args...).Run()
}

func (p *DockerProvider) IsNotFoundError(err error) bool {
	_, ok := err.(*dockerclient.NoSuchContainer)
	return ok
}

func (p *DockerProvider) Export(id string, output io.Writer) error {
	defer func(start time.Time) {
		p.logger.Infof("Export container %s took %v", id, time.Now().Sub(start))
	}(time.Now())
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}
	opts := dockerclient.ExportContainerOptions{
		ID:           container.ID,
		OutputStream: output,
	}
	return p.client.ExportContainer(opts)
}

func (p *DockerProvider) ListImages() ([]*provider.ImageInfo, error) {
	dockerImages, err := p.client.ListImages(false)
	if err != nil {
		return nil, err
	}

	list := []*provider.ImageInfo{}
	for _, dockerImage := range dockerImages {
		for _, name := range dockerImage.RepoTags {
			repo, tag := parsers.ParseRepoAndTag(name)
			info := &provider.ImageInfo{
				ID:          dockerImage.ID,
				Name:        repo,
				Tag:         tag,
				CreatedAt:   dockerImage.Created,
				ActualSize:  dockerImage.Size,
				VirtualSize: dockerImage.VirtualSize,
			}
			list = append(list, info)
		}
	}

	return list, nil
}

func (p *DockerProvider) BuildImage(o provider.BuildImageOptions) error {
	opts := dockerclient.BuildImageOptions{
		Name:                o.Name,
		InputStream:         o.InputStream,
		OutputStream:        o.OutputStream,
		RawJSONStream:       o.RawJSONStream,
		Remote:              o.Remote,
		RmTmpContainer:      true,
		ForceRmTmpContainer: true,
	}

	return p.client.BuildImage(opts)
}

func (p *DockerProvider) RemoveImage(imageIDOrName string) error {
	lookupResult, err := p.lookupImage(imageIDOrName)
	if err != nil {
		return err
	}

	image, useID := lookupResult.image, lookupResult.byID
	var removeID string
	if useID {
		removeID = image.ID
	} else {
		removeID = strings.Join([]string{image.Name, image.Tag}, ":")
	}
	err = p.client.RemoveImage(removeID)
	if err == nil {
		return nil
	}

	// TODO: This can be removed once this https://github.com/docker/docker/pull/7963/files
	// has been released
	if !useID {
		_, lookupErr := p.lookupImage(imageIDOrName)
		if lookupErr == provider.ErrImageNotFound {
			tagErr := p.client.TagImage(image.ID, dockerclient.TagImageOptions{Repo: image.Name, Tag: image.Tag})
			if tagErr != nil {
				return tagErr
			}
		}
	}

	if strings.Contains(err.Error(), "tagged in multiple repositories") {
		return provider.ErrImageShared
	}

	if strings.Contains(err.Error(), "Conflict, cannot delete") {
		// TODO: this is retarded, we should patch go-dockerclient to report a proper error
		// if status code == 409
		return provider.ErrImageInUse
	}

	return err
}

// PullImage pulls the image specified with `location` and writes the pull's
// output into the provided writer
func (p *DockerProvider) PullImage(location string, output io.Writer, rawJSONStream bool) error {
	registry := parsers.ParseRegistry(location)
	auth, err := p.fetchAuthenticationInfoBestMatch(registry)
	if err != nil {
		return err
	}

	repository, tag := parsers.ParseRepoAndTag(location)
	if tag == "" {
		tag = "latest"
	}

	pullOpts := dockerclient.PullImageOptions{
		Repository:    repository,
		Tag:           tag,
		OutputStream:  output,
		RawJSONStream: rawJSONStream,
	}
	return p.client.PullImage(pullOpts, auth)
}

// PushImage pushes an image identified by its ID or name to a destination
// repository.
//
// If destination repository is "", the image is simply pushed.
//
// As a side effect, the image will be tagged with the destination repository.
//
// Examples:
//   push rails to chuyeow/rails
//   push rails to quay.io/chuyeow/railsbox
//   push quay.io/chuyeow/rails
//   push quay.io/chuyeow/rails to tsuru.io/nitrous/railsbox
func (p *DockerProvider) PushImage(imageIDOrName, dest string, output io.Writer, rawJSONStream bool) error {
	lookupResult, err := p.lookupImage(imageIDOrName)
	if err != nil {
		return err
	}

	image := lookupResult.image

	// If destination repository is blank, we just want to push the image.
	if dest == "" {
		dest = strings.Join([]string{image.Name, image.Tag}, ":")
	}

	registry := parsers.ParseRegistry(dest)
	auth, err := p.fetchAuthenticationInfoBestMatch(registry)
	if err != nil {
		return err
	}
	if auth == (dockerclient.AuthConfiguration{}) {
		// docker push requires logging in to the registry first.
		return provider.ErrImagePushAuthRequired
	}

	if err := p.dockerTag(image.ID, dest, true); err != nil {
		return err
	}

	repository, tag := parsers.ParseRepoAndTag(dest)
	// while PushImageOptions contains a Registry attribute, it does not work
	// docker push XXXXXX calls the API with the full name (registry and repository) as the image name
	pushOpts := dockerclient.PushImageOptions{
		Name:          repository,
		Tag:           tag,
		OutputStream:  output,
		RawJSONStream: rawJSONStream,
	}

	return p.DockerClientProxy.PushImage(pushOpts, auth)
}

func (p *DockerProvider) TagImage(imageIDOrName, newName string, force bool) error {
	lookupResult, err := p.lookupImage(imageIDOrName)
	if err != nil {
		return err
	}

	image := lookupResult.image

	repository, tag := parsers.ParseRepoAndTag(newName)
	if tag == "" {
		tag = "latest"
	}
	finalName := strings.Join([]string{repository, tag}, ":")

	if !force {
		// check for collision
		_, err := p.client.InspectImage(finalName)
		if err != dockerclient.ErrNoSuchImage {
			return provider.ErrImageExists
		}
	}

	return p.dockerTag(image.ID, finalName, true)
}

// InspectImage returns information about a single image ( which can be
// identified by either its ID or its tag )
func (p *DockerProvider) InspectImage(name string, remoteFallback bool) (*provider.ImageInfo, error) {
	lookupResult, err := p.lookupImage(name)

	var image *provider.ImageInfo
	if err == provider.ErrImageNotFound && remoteFallback {
		return p.inspectRemoteImage(name)
	}

	if err != nil {
		return nil, err
	}

	image = lookupResult.image
	additional, err := p.inspectLocalImage(image.ID)
	if err != nil {
		return nil, err
	}
	image.Config = additional.Config

	return image, nil
}

func (p *DockerProvider) AttachToContainer(id string, in io.Reader, out io.Writer, noStdin bool) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	return p.client.AttachToContainer(dockerclient.AttachToContainerOptions{
		Container:    container.ID,
		InputStream:  in,
		OutputStream: out,
		ErrorStream:  out,
		Stream:       true,
		Stdin:        !noStdin,
		Stdout:       true,
		Stderr:       true,
		RawTerminal:  true,
	})
}

func (p *DockerProvider) ResizeContainerTTY(id string, height, width int) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	return p.client.ResizeContainerTTY(container.ID, height, width)
}

func (p *DockerProvider) FileInfo(id string, location string) (provider.FileInfo, error) {
	container, err := p.inspectContainer(id)
	if err != nil {
		return provider.FileInfo{}, err
	}
	info, err := cops.GetFileInfo(container.ID, location)
	if err != nil {
		return provider.FileInfo{}, err
	}
	return provider.FileInfo{
		IsDir: info.IsDir,
	}, nil
}

// CopyFile copies file/directory from a docker container. The output is
// written into the provided io.Writer as a tar stream for a directory and
// file content for a file.
func (p *DockerProvider) CopyFile(id string, location string, out io.Writer) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}
	info, err := cops.GetFileInfo(id, location)
	if err != nil {
		return err
	}
	if !info.IsDir {
		return cops.ReadFromContainer(id, location, out)
	}
	return p.client.CopyFromContainer(dockerclient.CopyFromContainerOptions{
		OutputStream: out,
		Container:    container.ID,
		Resource:     location,
	})
}

func (p *DockerProvider) ReadFile(id string, location string) (string, error) {
	var b bytes.Buffer
	if err := p.CopyFile(id, location, &b); err != nil {
		return "", err
	}
	return string(b.Bytes()), nil
}

// CopyFromHost copies files from host into a container
// TODO: docker should also provide this functionality so we dont have to
// rely on cops/cfs for this
func (p *DockerProvider) CopyFromHost(id string, location string, input map[string]io.Reader) error {
	container, err := p.inspectContainer(id)
	if err != nil {
		return err
	}

	// for each uploaded file, create a new file in the specified location
	// and copy the parts over
	for filename, r := range input {
		dest := filepath.Join(location, filename)
		if err := cops.CopyToContainer(container.ID, dest, r); err != nil {
			return err
		}
	}
	return nil
}

func (p *DockerProvider) Version() (string, error) {
	env, err := p.client.Version()
	if err != nil {
		return "", err
	}
	version := env.Get("Version")
	if version == "" {
		return "", errors.New("no Version returned by p.client.Version()")
	}
	return version, nil
}

type authconfig struct {
	Auth  string `json:"auth"`
	Email string `json:"email"`
}

func (p *DockerProvider) fetchAuthenticationInfoBestMatch(registry string) (dockerclient.AuthConfiguration, error) {
	if registry == "" {
		registry = "index.docker.io"
	}

	// if we dont have the auth configuration, use unauthenticated request
	authPath, err := p.dockerConfigLocation()
	if err != nil {
		p.logger.Warnf("failed to find authentication location, err: %v", err)
		return dockerclient.AuthConfiguration{}, nil
	}

	fileContents, err := ioutil.ReadFile(authPath)
	if err != nil {
		p.logger.Warnf("failed to find authentication details, err: %v", err)
		return dockerclient.AuthConfiguration{}, nil
	}

	// got the auth contents here, parse it and get the correct configuration
	// out
	var endpointMap map[string]authconfig
	err = json.Unmarshal(fileContents, &endpointMap)
	if err != nil {
		return dockerclient.AuthConfiguration{}, err
	}

	for endpoint, authconfig := range endpointMap {
		if strings.Contains(endpoint, registry) {
			if authconfig.Auth == "" {
				return dockerclient.AuthConfiguration{}, nil
			}
			decoded, err := base64.StdEncoding.DecodeString(authconfig.Auth)
			if err != nil {
				return dockerclient.AuthConfiguration{}, err
			}
			authParams := strings.Split(string(decoded), ":")
			if len(authParams) != 2 {
				return dockerclient.AuthConfiguration{}, provider.ErrImageAuthMisconfigured
			}
			user, password := authParams[0], authParams[1]
			return dockerclient.AuthConfiguration{
				Username: user,
				Password: password,
				Email:    authconfig.Email,
			}, nil
		}
	}

	return dockerclient.AuthConfiguration{}, nil
}

func (p *DockerProvider) dockerConfigLocation() (string, error) {
	if len(p.DockerConfigLocation) != 0 {
		return p.DockerConfigLocation, nil
	}

	// Use $HOME as the root path when fetching .dockercfg file.
	// As suggested in - https://github.com/nitrous-io/nitrous/issues/134
	configPath := path.Join(os.Getenv("HOME"), ".dockercfg")

	_, err := os.Stat(configPath)
	if err != nil {
		return "", err
	}
	return configPath, nil
}

func (p *DockerProvider) lookupImage(imageIDOrName string) (lookupImageResult, error) {
	// look up the image to test its validity
	images, err := p.ListImages()
	if err != nil {
		return lookupImageResult{}, err
	}

	repository, tag := parsers.ParseRepoAndTag(imageIDOrName)
	if tag == "" {
		tag = "latest"
	}
	for _, image := range images {
		if image.ID == imageIDOrName {
			return lookupImageResult{image: image, byID: true}, nil
		} else {
			if repository == image.Name && tag == image.Tag {
				return lookupImageResult{image: image}, nil
			}
		}
	}

	return lookupImageResult{}, provider.ErrImageNotFound
}

func (p *DockerProvider) inspectLocalImage(location string) (*provider.ImageInfo, error) {
	img, err := p.client.InspectImage(location)
	if err == dockerclient.ErrNoSuchImage {
		return nil, provider.ErrImageNotFound
	} else if err != nil {
		return nil, err
	}

	config := buildDockerConfig(img.Config, img.ID)
	info := &provider.ImageInfo{
		ID:         img.ID,
		CreatedAt:  img.Created.Unix(),
		ActualSize: img.Size,
		Config:     config,
	}
	return info, nil
}

func (p *DockerProvider) inspectRemoteImage(location string) (*provider.ImageInfo, error) {
	registry := parsers.ParseRegistry(location)
	auth, err := p.fetchAuthenticationInfoBestMatch(registry)
	if err != nil {
		return nil, err
	}

	if registry == "" {
		registry = defaultDockerRegistry
	}

	repo, tag := parsers.ParseRepoAndTag(location)
	repo = strings.Replace(repo, registry+"/", "", 1)
	if tag == "" {
		tag = "latest"
	}

	initReqLoc := fmt.Sprintf("https://%s/v1/repositories/%s/images", registry, repo)
	resp, err := p.performRegistryRequest(initReqLoc, auth, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dockerSignature := resp.Header.Get("X-Docker-Token")

	var idList []struct {
		Checksum string `json:"checksum"`
		Id       string `json:"id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&idList)
	if err != nil {
		return nil, err
	}

	tagListLoc := fmt.Sprintf("https://%s/v1/repositories/%s/tags", registry, repo)
	resp, err = p.performRegistryRequest(tagListLoc, auth, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tagList []struct {
		Layer string `json:"layer"`
		Name  string `json:"name"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tagList)
	if err != nil {
		return nil, err
	}

	var expectedId string
	for _, tagItem := range tagList {
		if tag != tagItem.Name {
			continue
		}
		for _, idItem := range idList {
			if strings.HasPrefix(idItem.Id, tagItem.Layer) {
				expectedId = idItem.Id
			}
		}
	}

	if expectedId == "" {
		return nil, provider.ErrImageNotFound
	}

	imageInspectHost := registry
	if registry == defaultDockerRegistry {
		imageInspectHost = dockerRegistryImageEndpoint
	}

	imageInfoLoc := fmt.Sprintf("https://%s/v1/images/%s/json", imageInspectHost, expectedId)
	resp, err = p.performRegistryRequest(imageInfoLoc, dockerclient.AuthConfiguration{}, dockerSignature)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var imageInfo dockerclient.ImagePre012
	err = json.NewDecoder(resp.Body).Decode(&imageInfo)
	if err != nil {
		return nil, err
	}

	imageConfig := buildDockerConfig(imageInfo.Config, imageInfo.ID)
	info := &provider.ImageInfo{
		ID:         imageInfo.ID,
		Name:       repo,
		Tag:        tag,
		CreatedAt:  imageInfo.Created.Unix(),
		ActualSize: imageInfo.Size,
		Config:     imageConfig,
	}

	return info, nil
}

func (p *DockerProvider) performRegistryRequest(url string, auth dockerclient.AuthConfiguration, signature string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Docker-Token", "true")
	req.Header.Set("Accept", "application/json")
	if auth.Username != "" {
		authDetails := fmt.Sprintf("%s:%s", auth.Username, auth.Password)
		req.Header.Set("Authorization", base64.StdEncoding.EncodeToString([]byte(authDetails)))
	}

	if signature != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Token %v", signature))
	}

	cl := p.client
	resp, err := cl.HTTPClient.Do(req)
	if err != nil {
		return resp, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return resp, provider.ErrImageNotFound
	case http.StatusUnauthorized:
		return resp, provider.ErrImageAuthNotAllowed
	default:
		body, err := ioutil.ReadAll(resp.Body)
		bodyStr := ""
		if err == nil {
			bodyStr = string(body)
		}
		return resp, errors.New(fmt.Sprintf("Unknown error: (%d) %s", resp.StatusCode, bodyStr))
	}

	return resp, err
}

func (p *DockerProvider) ContainerSettingsDir(containerName string) string {
	return path.Join(p.dataPath, "containers", containerName)
}

func (p *DockerProvider) storeSharedSecret(containerName string, secret string) error {
	p.logger.Info("DockerProvider: storing shared secret for", containerName)
	dir := p.ContainerSettingsDir(containerName)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	filename := path.Join(dir, "secret")
	err = ioutil.WriteFile(filename, []byte(secret), 0600)
	if err != nil {
		return err
	}
	return nil
}

func (p *DockerProvider) storeContainerName(containerName string) error {
	p.logger.Infof("DockerProvider: storing %s for %s", nitrousSlugFilename, containerName)
	dir := p.ContainerSettingsDir(containerName)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	filename := path.Join(dir, nitrousSlugFilename)
	err = ioutil.WriteFile(filename, []byte(containerName), 0755)
	if err != nil {
		return err
	}
	return nil
}

func (p *DockerProvider) readSharedSecret(containerName string) (string, error) {
	filename := path.Join(p.ContainerSettingsDir(containerName), "secret")
	secret, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(secret)), nil
}

func (p *DockerProvider) containerAuthorizedKeys(containerName string) *AuthorizedKeys {
	return NewAuthorizedKeys(
		path.Join(p.ContainerSettingsDir(containerName), "ssh", "authorized_keys"),
	)
}

func (p *DockerProvider) inspectContainer(id string) (*dockerclient.Container, error) {
	return p.client.InspectContainer(id)
}

func (p *DockerProvider) dockerTag(image string, newName string, force bool) error {
	repo, tag := parsers.ParseRepoAndTag(newName)
	return p.client.TagImage(image, dockerclient.TagImageOptions{
		Repo:  repo,
		Tag:   tag,
		Force: force,
	})
}

func (p *DockerProvider) isNitrousImage(fullName string) bool {
	imageInfo, err := p.client.InspectImage(fullName)
	if err != nil {
		return false
	}

	if imageInfo.Config == nil {
		return false
	}

	_, ok := envValue(imageInfo.Config.Env, nitrousEnvImage)
	return ok
}

// returns false if not nitrous user
func (p *DockerProvider) appUser(container *dockerclient.Container) (string, bool) {
	return envValue(container.Config.Env, nitrousEnvUser)
}

func (p *DockerProvider) appPID(container *dockerclient.Container, cmd string) (int, error) {
	usr, _ := p.appUser(container)
	fullCmd := strings.Join(execForm(cmd, usr), " ")
	nsenterCmd := fmt.Sprintf(`nsenter-exec --nspid %d -- %s`, container.State.Pid, fullCmd)

	pgrepCmd := exec.Command("pgrep", "--full", "--exact", "--newest", nsenterCmd)
	out, err := pgrepCmd.CombinedOutput()
	if err != nil {
		p.logger.Errorf("pgrep for nsenter process failed, err: %v, output: %s", err, out)
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		p.logger.Errorf("pgrep did not return a proper pid, err: %v, output: %s", err, out)
		return 0, err
	}

	return pid, nil
}

// returns false if envVar doesn't exist in env
func envValue(env []string, envVar string) (string, bool) {
	for _, val := range env {
		split := strings.SplitN(val, "=", 2)
		if len(split) == 2 && split[0] == envVar {
			return split[1], true
		}
	}
	return "", false
}

func containsEnvVar(env []string, envVar string) bool {
	for _, val := range env {
		split := strings.SplitN(val, "=", 2)
		if len(split) == 2 && split[0] == envVar {
			return true
		}
	}
	return false
}

func (p *DockerProvider) setDefaultContainerConfig(c *provider.ContainerConfig) {
	c.Env = append(c.Env, fmt.Sprintf("__HOST=%s", p.gateway))
}

func buildDockerConfig(config *dockerclient.Config, imageId string) *provider.DockerConfig {
	if config == nil {
		return nil
	}

	imageConfig := &provider.DockerConfig{
		Env:          config.Env,
		Cmd:          config.Cmd,
		Volumes:      config.Volumes,
		PortSpecs:    config.PortSpecs,
		ExposedPorts: make(map[string]struct{}),
		Image:        imageId,
	}

	for port, str := range config.ExposedPorts {
		imageConfig.ExposedPorts[string(port)] = str
	}

	return imageConfig
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

// portsFromContainer returns a mapping from container port to a slice of host
// ports. The result means different things depending on container state.
// For running containers, the actual "live" port bindings are returned.
// For stopped containers, the *configured* port bindings are returned and
// will look like this: "22" -> [] to indicate that there is a port binding for
// port 22.
func portsFromContainer(container *dockerclient.Container) map[string][]string {
	ports := make(map[string][]string)

	// If container is running, the actual "live" port bindings are set in
	// NetworkSettings.Ports by Docker.
	if container.State.Running {
		for port, bindings := range container.NetworkSettings.Ports {
			containerPort := port.Port()
			for _, binding := range bindings {
				ports[containerPort] = append(ports[containerPort], binding.HostPort)
			}
		}
		return ports
	}

	// Otherwise, container is stopped so we have to merge HostConfig and
	// ExposedPorts to determine the configured port bindings.
	for port, bindings := range container.HostConfig.PortBindings {
		containerPort := port.Port()
		for _, binding := range bindings {
			ports[containerPort] = append(ports[containerPort], binding.HostPort)
		}
	}
	for port := range container.Config.ExposedPorts {
		containerPort := port.Port()
		if _, ok := ports[containerPort]; !ok {
			ports[containerPort] = []string{}
		}
	}
	return ports
}

func containerConfigFromJSON(jsonStr string) (*provider.ContainerConfig, error) {
	cfg := &provider.ContainerConfig{}
	if jsonStr == "" {
		return cfg, nil
	}
	err := json.NewDecoder(strings.NewReader(jsonStr)).Decode(&cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// gatewayFromIPOutput takes the output of `ip addr show docker0` (do not turn
// off inet family via options) and returns the docker gateway ip
func gatewayFromIPOutput(output []byte) (string, error) {
	// output looks like
	// 2: docker0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default
	//     inet 10.1.42.1/16 scope global docker0
	//        valid_lft forever preferred_lft forever
	re := regexp.MustCompile(`inet ([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})/`)
	match := re.FindSubmatch(output)
	if match == nil || len(match) != 2 {
		return "", errors.New("no inet ip address found")
	}
	return string(match[1]), nil
}
