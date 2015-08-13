package docker_test

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/provider/docker/parsers"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"github.com/nitrous-io/nitrous-common/hmactoken"
	"github.com/onsi/gomega/ghttp"
	"golang.org/x/crypto/ssh"

	. "github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() { logrus.SetLevel(logrus.PanicLevel) }

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "provider/docker")
}

const (
	busyboxImage = "busybox:latest"
)

var _ = BeforeSuite(func() {
	var (
		start time.Time
		out   []byte
		err   error
		cmd   *exec.Cmd
	)

	// load the stored ubuntu-bare image so we dont have to pull it from the internet
	// nitrousio/ubuntu-bare image from /root/ubuntu-bare.tar.gz
	start = time.Now()
	fmt.Print("build nitrousio/ubuntu-bare:latest...")
	out, err = exec.Command("docker", "load", "-i", "/root/ubuntu-bare.tar.gz").CombinedOutput()
	Expect(err).To(BeNil(), "failed to load nitrousio/ubuntu-bare, output: %s", string(out))
	fmt.Printf(" took %s\n", time.Now().Sub(start))

	// build nitrous/ubuntu based on nitrosio/ubuntu-bare, but with "top" cmd
	start = time.Now()
	fmt.Print("build nitrous/ubuntu:latest...")
	cmd = exec.Command("docker", "build", "-t", "nitrousio/ubuntu-test:latest", "-")
	cmd.Stdin = bytes.NewBufferString(`
	FROM nitrousio/ubuntu-bare:latest
	CMD ["top"]
	`)
	out, err = cmd.CombinedOutput()
	Expect(err).To(BeNil(), "failed to build nitrous/ubuntu, output: %s", string(out))
	fmt.Printf(" took %s\n", time.Now().Sub(start))

	// build base, which is busybox with /sh as CMD
	start = time.Now()
	fmt.Print("build base:latest...")
	cmd = exec.Command("docker", "build", "-t", "base:latest", ".")
	cmd.Dir = "/docker-busybox"
	out, err = cmd.CombinedOutput()
	Expect(err).To(BeNil(), "failed to build base, output: %s", string(out))
	fmt.Printf(" took %s\n", time.Now().Sub(start))

	// build busybox, which is busybox duh but with top as CMD
	start = time.Now()
	fmt.Print("build busybox:latest...")
	cmd = exec.Command("docker", "build", "-t", "busybox:latest", "-")
	cmd.Stdin = bytes.NewBufferString(`
	FROM base:latest
	CMD ["top"]
	`)
	out, err = cmd.CombinedOutput()
	Expect(err).To(BeNil(), "failed to build busybox, output: %s", string(out))
	fmt.Printf(" took %s\n", time.Now().Sub(start))

	// replace NitrousImage to not having to run /sbin/init as pid 1
	NitrousImage = "nitrousio/ubuntu-test:latest"
})

var _ = AfterSuite(func() {
	NitrousImage = "nitrousio/ubuntu-bare:latest"
})

var _ = Describe("provider/docker", func() {
	var (
		prov    *docker.DockerProvider
		secret  = "s3cr3t"
		datadir string
	)

	BeforeEach(func() {
		var err error
		datadir = os.Getenv("FREIGHTERD_BASE")

		if datadir == "" {
			datadir, err = TempDir()
			Expect(err).To(BeNil())
		}

		prov, err = docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())
	})

	doInspect := func(id, format string) string {
		out, err := exec.Command("docker", "inspect", "-f", format, id).CombinedOutput()
		Expect(err).To(BeNil(), "docker inspect -f %v %v failed with output: %v", format, id, string(out))
		return strings.TrimSpace(string(out))
	}

	inspectID := func(name string) string {
		return doInspect(name, "{{.Id}}")
	}

	inspectNetworkSettingsPort := func(id, port string) string {
		return doInspect(id, fmt.Sprintf(`{{(index (index .NetworkSettings.Ports "%s") 0).HostPort}}`, port))
	}

	inspectIP := func(id string) string {
		return doInspect(id, "{{.NetworkSettings.IPAddress}}")
	}

	inspectGateway := func(id string) string {
		return doInspect(id, "{{.NetworkSettings.Gateway}}")
	}

	inspectEnv := func(id string) string {
		return doInspect(id, "{{.Config.Env}}")
	}

	inspectUser := func(id string) string {
		return doInspect(id, "{{.Config.User}}")
	}

	expectExposedPort := func(id, port string) {
		out := doInspect(id, fmt.Sprintf(`{{(index .Config.ExposedPorts "%s")}}`, port))
		Expect(out).NotTo(ContainSubstring("<no value>"),
			"Port %s not exposed. docker inspect .Config.ExposedPorts output: %v", port, out)
	}

	type configInfo struct {
		Cmd        []string
		Entrypoint []string
	}

	type stateInfo struct {
		ExitCode   int
		StartedAt  time.Time
		FinishedAt time.Time
	}

	type containerInfo struct {
		Config configInfo
		State  stateInfo
	}

	inspectContainerInfo := func(id string) *containerInfo {
		out := doInspect(id, `{ "Config":{{json .Config}}, "State":{{json .State}} }`)
		var v containerInfo
		err := json.Unmarshal([]byte(out), &v)
		Expect(err).To(BeNil())
		return &v
	}

	createContainer := func(tag string) string {
		out, err := exec.Command("docker", "run", "-d", tag, "/bin/bash").CombinedOutput()
		Expect(err).To(BeNil(), "Failed to create container, output: %s", string(out))

		return string(out)
	}

	Describe(".NewDockerProvider()", func() {
		It("populates provisioner gateway", func() {
			// use different command so as not to test exact same codepath
			out, err := exec.Command("ifconfig", "docker0").CombinedOutput()
			Expect(err).To(BeNil())
			re := regexp.MustCompile(`inet addr:([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})`)
			match := re.FindSubmatch(out)
			Expect(match).To(HaveLen(2), "unexpected ifconfig output: %s", out)

			p, err := docker.NewDockerProvider(datadir)
			Expect(err).To(BeNil())
			Expect(p.Gateway()).To(Equal(string(match[1])))
		})
	})

	Describe(".CreateStart()", func() {
		It("creates a running container", func() {
			name := RandomName("freightertest")
			info, err := prov.CreateStart(name, "nitrousio/ubuntu-test:latest", secret, "")
			Expect(err).To(BeNil())
			defer CleanUpContainer(name)
			id := inspectID(name)
			gateway := inspectGateway(id)

			Expect(info.ID).To(Equal(id))
			Expect(info.Name).To(Equal(name))
			Expect(info.Image).To(Equal(NitrousImage))
			Expect(info.ImageID).To(Equal(inspectID(NitrousImage)))
			Expect(info.Host).To(Equal(inspectIP(id)))
			Expect(info.Gateway).To(Equal(gateway))
			Expect(info.State).To(Equal(provider.ContainerStateRunning))
			sshPort := inspectNetworkSettingsPort(id, "22/tcp")
			Expect(info.Ports["22"]).To(ContainElement(sshPort))
			expectExposedPort(info.ID, "22/tcp")
			Expect(inspectEnv(id)).To(ContainSubstring(fmt.Sprintf("__HOST=%s", gateway)))

			c := inspectContainerInfo(id)
			Expect(info.Cmd).To(Equal(c.Config.Cmd))
			Expect(info.Entrypoint).To(Equal(c.Config.Entrypoint))
			Expect(info.ExitCode).To(Equal(c.State.ExitCode))
			Expect(info.StartedAt).To(Equal(c.State.StartedAt))
			Expect(info.FinishedAt).To(Equal(c.State.FinishedAt))

			settingsDir := prov.ContainerSettingsDir(name)
			_, err = os.Stat(settingsDir)
			Expect(err).To(BeNil()) // container settings dir exists

			authKeysFile := path.Join(settingsDir, "ssh", "authorized_keys")
			_, err = os.Stat(authKeysFile)
			Expect(err).To(BeNil()) // authorized_keys file exists

			containerSlugFile := path.Join(settingsDir, ".nitrous-container-slug")
			_, err = os.Stat(containerSlugFile)
			Expect(err).To(BeNil()) // .nitrous-container-slug file exists
			s, err := ioutil.ReadFile(containerSlugFile)
			Expect(string(s)).To(Equal(name))

			sharedSecretFile := path.Join(settingsDir, "secret")
			fi, err := os.Stat(sharedSecretFile)
			Expect(err).To(BeNil())                        // shared secret file exists
			Expect(fi.Mode()).To(Equal(os.FileMode(0600))) // shared secret file has perm 0600
			s, err = ioutil.ReadFile(sharedSecretFile)
			Expect(string(s)).To(Equal(secret))
		})

		Context("after a container is created", func() {
			var (
				name string
				id   string
				info *provider.ContainerInfo

				// set to false for tests which handle their own termination
				terminateAfterTest = true
			)

			BeforeEach(func() {
				var err error
				name = RandomName("freightertest")
				info, err = prov.CreateStart(name, NitrousImage, secret, "")
				Expect(err).To(BeNil())
				id = inspectID(name)
			})

			AfterEach(func() {
				if terminateAfterTest {
					CleanUpContainer(name)
				}
			})

			Describe(".List()", func() {
				It("returns a list of all containers", func() {
					// Create another container.
					container2Name := RandomName("freightertest2")
					container2Info, err := prov.CreateStart(container2Name, NitrousImage, secret, "")
					Expect(err).To(BeNil())
					defer CleanUpContainer(container2Name)

					list, err := prov.List()
					Expect(err).To(BeNil())
					Expect(list).To(HaveLen(2))

					var c1, c2 *provider.ContainerInfo
					for _, c := range list {
						if c.ID == id {
							c2 = c
						} else if c.ID == container2Info.ID {
							c1 = c
						}
					}

					imageID := inspectID(NitrousImage)

					Expect(c1.ID).To(Equal(container2Info.ID))
					Expect(c1.Name).To(Equal(container2Name))
					Expect(c1.Image).To(Equal(NitrousImage))
					Expect(c1.ImageID).To(Equal(imageID))
					Expect(c1.Host).To(Equal(inspectIP(container2Info.ID)))
					Expect(c1.Gateway).To(Equal(inspectGateway(container2Info.ID)))
					Expect(c1.State).To(Equal(provider.ContainerStateRunning))
					sshPort := inspectNetworkSettingsPort(container2Info.ID, "22/tcp")
					Expect(c1.Ports["22"]).To(ContainElement(sshPort))
					expectExposedPort(c1.ID, "22/tcp")

					c := inspectContainerInfo(container2Info.ID)
					Expect(c1.Cmd).To(Equal(c.Config.Cmd))
					Expect(c1.Entrypoint).To(Equal(c.Config.Entrypoint))
					Expect(c1.ExitCode).To(Equal(c.State.ExitCode))
					Expect(c1.StartedAt).To(Equal(c.State.StartedAt))
					Expect(c1.FinishedAt).To(Equal(c.State.FinishedAt))

					Expect(c2.ID).To(Equal(id))
					Expect(c2.Name).To(Equal(name))
					Expect(c2.Image).To(Equal(NitrousImage))
					Expect(c2.ImageID).To(Equal(imageID))
					Expect(c2.Host).To(Equal(inspectIP(id)))
					Expect(c2.Gateway).To(Equal(inspectGateway(id)))
					Expect(c2.State).To(Equal(provider.ContainerStateRunning))
					sshPort = inspectNetworkSettingsPort(id, "22/tcp")
					Expect(c2.Ports["22"]).To(ContainElement(sshPort))
					expectExposedPort(c2.ID, "22/tcp")

					c = inspectContainerInfo(id)
					Expect(c2.Cmd).To(Equal(c.Config.Cmd))
					Expect(c2.Entrypoint).To(Equal(c.Config.Entrypoint))
					Expect(c2.ExitCode).To(Equal(c.State.ExitCode))
					Expect(c2.StartedAt).To(Equal(c.State.StartedAt))
					Expect(c2.FinishedAt).To(Equal(c.State.FinishedAt))
				})
			})

			Describe(".GetConfig()", func() {
				It("returns container config", func() {
					config, err := prov.GetConfig(id)
					Expect(err).To(BeNil())
					// not all config checked against -- elided to avoid test
					// fragility wrt image changes
					Expect(config).NotTo(BeNil())
					Expect(config.Env).To(ContainElement(fmt.Sprintf("__HOST=%s", inspectGateway(id))))
					Expect(config.Mounts).To(HaveLen(2))
					Expect(strings.HasSuffix(config.Mounts[0], "/nitrous/ssh:ro")).To(BeTrue())
					Expect(strings.HasSuffix(config.Mounts[1], "/.nitrous-container-slug:ro")).To(BeTrue())
					Expect(*config.Privileged).To(BeFalse())
					Expect(*config.PublishAll).To(BeTrue()) // Defaults to true for Nitrous-blessed images.
				})
			})

			Describe(".Start()", func() {
				expectContainerToBeRunning := func() {
					out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
					Expect(err).To(BeNil())
					Expect(out).To(ContainSubstring("true"))
				}

				expectContainerInfoToHaveBeenReturned := func(info *provider.ContainerInfo) {
					Expect(info).NotTo(BeNil())
					Expect(info.ID).To(Equal(id))
					Expect(info.Name).To(Equal(name))
					Expect(info.Image).To(Equal(NitrousImage))
					Expect(info.ImageID).To(Equal(inspectID(NitrousImage)))
					Expect(info.Host).To(Equal(inspectIP(id)))
					Expect(info.Gateway).To(Equal(inspectGateway(id)))
					Expect(info.State).To(Equal(provider.ContainerStateRunning))
					sshPort := inspectNetworkSettingsPort(id, "22/tcp")
					Expect(info.Ports["22"]).To(ContainElement(sshPort))
					expectExposedPort(info.ID, "22/tcp")

					c := inspectContainerInfo(id)
					Expect(info.Cmd).To(Equal(c.Config.Cmd))
					Expect(info.Entrypoint).To(Equal(c.Config.Entrypoint))
					Expect(info.ExitCode).To(Equal(c.State.ExitCode))
					Expect(info.StartedAt).To(Equal(c.State.StartedAt))
					Expect(info.FinishedAt).To(Equal(c.State.FinishedAt))
				}

				Context("when the container is running", func() {
					It("does nothing", func() {
						info, err := prov.Start(id, "")
						Expect(err).To(BeNil())
						expectContainerToBeRunning()
						expectContainerInfoToHaveBeenReturned(info)
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("starts the container", func() {
						info, err := prov.Start(id, "")
						Expect(err).To(BeNil())
						expectContainerToBeRunning()
						expectContainerInfoToHaveBeenReturned(info)
					})
				})

				It("injects a '<gateway IP> host' entry to the container's /etc/hosts file", func() {
					_, err := prov.Start(id, "")
					Expect(err).To(BeNil())
					out, err := exec.Command("docker", "exec", id, "/bin/cat", "/etc/hosts").CombinedOutput()
					Expect(err).To(BeNil())
					Expect(string(out)).To(ContainSubstring(fmt.Sprintf("%s\thost", inspectGateway(id))))
				})

				It("provides RO access to the container's name", func() {
					_, err := prov.Start(id, "")
					Expect(err).To(BeNil())

					out, err := exec.Command("docker", "exec", id, "/bin/rm", "/.nitrous-container-slug").CombinedOutput()
					Expect(err).ToNot(BeNil())
					Expect(string(out)).To(ContainSubstring("cannot remove"))

					out, err = exec.Command("docker", "exec", id, "/bin/cat", "/.nitrous-container-slug").CombinedOutput()
					Expect(err).To(BeNil())
					Expect(string(out)).To(Equal(name))
				})
			})

			Describe(".Stop()", func() {
				var info *provider.ContainerInfo

				expectContainerToBeStopped := func() {
					out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
					Expect(err).To(BeNil())
					Expect(out).To(ContainSubstring("false"))
				}

				expectContainerInfoToHaveBeenReturned := func() {
					Expect(info).NotTo(BeNil())
					Expect(info.ID).To(Equal(id))
					Expect(info.Host).To(Equal(""))
					Expect(info.Gateway).To(Equal(""))
					Expect(info.State).To(Equal(provider.ContainerStateStopped))
					hostPorts, ok := info.Ports["22"]
					Expect(ok).To(BeTrue())
					Expect(hostPorts).To(HaveLen(0))
					Expect(info.Name).To(Equal(name))
					Expect(info.Image).To(Equal(NitrousImage))
					Expect(info.ImageID).To(Equal(inspectID(NitrousImage)))

					c := inspectContainerInfo(id)
					Expect(info.Cmd).To(Equal(c.Config.Cmd))
					Expect(info.Entrypoint).To(Equal(c.Config.Entrypoint))
					Expect(info.ExitCode).To(Equal(c.State.ExitCode))
					Expect(info.StartedAt).To(Equal(c.State.StartedAt))
					Expect(info.FinishedAt).To(Equal(c.State.FinishedAt))
				}

				JustBeforeEach(func() {
					var err error
					info, err = prov.Stop(id)
					Expect(err).To(BeNil())
				})

				Context("when the container is running", func() {
					It("stops the container", func() {
						expectContainerToBeStopped()
						expectContainerInfoToHaveBeenReturned()
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						_, err := prov.Stop(id)
						Expect(err).To(BeNil())
					})

					It("does nothing", func() {
						expectContainerToBeStopped()
						expectContainerInfoToHaveBeenReturned()
					})
				})
			})

			Describe(".Restart()", func() {
				expectContainerInfoToHaveBeenReturned := func(info *provider.ContainerInfo) {
					Expect(info.ID).To(Equal(id))
					Expect(info.Name).To(Equal(name))
					Expect(info.Image).To(Equal(NitrousImage))
					Expect(info.ImageID).To(Equal(inspectID(NitrousImage)))
					Expect(info.Gateway).To(Equal(inspectGateway(id)))
					Expect(info.Host).To(Equal(inspectIP(id)))
					Expect(info.State).To(Equal(provider.ContainerStateRunning))
					sshPort := inspectNetworkSettingsPort(id, "22/tcp")
					Expect(info.Ports["22"]).To(ContainElement(sshPort))
					expectExposedPort(info.ID, "22/tcp")

					c := inspectContainerInfo(id)
					Expect(info.Cmd).To(Equal(c.Config.Cmd))
					Expect(info.Entrypoint).To(Equal(c.Config.Entrypoint))
					Expect(info.ExitCode).To(Equal(c.State.ExitCode))
					Expect(info.StartedAt).To(Equal(c.State.StartedAt))
					Expect(info.FinishedAt).To(Equal(c.State.FinishedAt))
				}

				Context("when the container is running", func() {
					It("restarts the container", func() {
						startedAt1, err := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", id).CombinedOutput()
						Expect(err).To(BeNil())

						info, err := prov.Restart(id)
						Expect(err).To(BeNil())
						expectContainerInfoToHaveBeenReturned(info)

						out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						Expect(out).To(ContainSubstring("true"))

						// check that the started at time changed when container restarts
						startedAt2, err := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						Expect(startedAt2).ToNot(Equal(startedAt1))
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("starts the container", func() {
						info, err := prov.Restart(id)
						Expect(err).To(BeNil())
						expectContainerInfoToHaveBeenReturned(info)
						out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						Expect(out).To(ContainSubstring("true"))
					})
				})
			})

			Describe(".Terminate()", func() {
				expectContainerToBeTerminatedAndPortReleased := func(info *provider.ContainerInfo) {
					out, err := exec.Command("docker", "inspect", id).CombinedOutput()
					Expect(err).NotTo(BeNil())
					Expect(out).To(ContainSubstring("No such image or container"))
					settingsDir := prov.ContainerSettingsDir(info.Name)
					_, err = os.Stat(settingsDir)
					Expect(os.IsNotExist(err)).To(BeTrue(), "Terminate did not remove settings dir %s", settingsDir)
				}

				expectContainerInfoToHaveBeenReturned := func(info *provider.ContainerInfo, origInfo *containerInfo) {
					Expect(info).NotTo(BeNil())
					Expect(info.ID).To(Equal(id))
					Expect(info.Name).To(Equal(name))
					Expect(info.Image).To(Equal(NitrousImage))
					Expect(info.ImageID).To(Equal(inspectID(NitrousImage)))
					Expect(info.Host).To(Equal(""))
					Expect(info.Gateway).To(Equal(""))
					Expect(info.Ports).To(BeEmpty())
					Expect(info.State).To(Equal(""))

					Expect(info.Cmd).To(Equal(origInfo.Config.Cmd))
					Expect(info.Entrypoint).To(Equal(origInfo.Config.Entrypoint))
					Expect(info.StartedAt).To(Equal(origInfo.State.StartedAt))
					Expect(info.FinishedAt.IsZero()).To(BeFalse())
				}

				BeforeEach(func() {
					terminateAfterTest = false
				})

				Context("when the container is running", func() {
					It("terminates the container", func() {
						// Get container info before terminating.
						origInfo := inspectContainerInfo(id)

						info, err := prov.Terminate(id, true)
						Expect(err).To(BeNil())
						expectContainerToBeTerminatedAndPortReleased(info)
						expectContainerInfoToHaveBeenReturned(info, origInfo)
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("terminates the container", func() {
						// Get container info before terminating.
						origInfo := inspectContainerInfo(id)

						info, err := prov.Terminate(id, true)
						Expect(err).To(BeNil())
						expectContainerToBeTerminatedAndPortReleased(info)
						expectContainerInfoToHaveBeenReturned(info, origInfo)
					})
				})
			})

			Describe(".Commit()", func() {
				It("creates an image", func() {
					image, err := prov.Commit(id, "committed")
					Expect(err).To(BeNil())
					defer RemoveImages([]string{image.ID})
					Expect(image).NotTo(BeNil())
					Expect(image.Name).To(Equal("committed"))
					Expect(image.Tag).To(Equal("latest"))
					out, err := exec.Command("docker", "images", "--no-trunc").CombinedOutput()
					Expect(err).To(BeNil())
					Expect(out).To(ContainSubstring(image.ID))
				})

				It("creates an image with a custom tag", func() {
					image, err := prov.Commit(id, "archived:deprecated")
					Expect(err).To(BeNil())
					defer RemoveImages([]string{image.ID})
					Expect(image).NotTo(BeNil())
					Expect(image.Name).To(Equal("archived"))
					Expect(image.Tag).To(Equal("deprecated"))
					out, err := exec.Command("docker", "images", "--no-trunc").CombinedOutput()
					Expect(err).To(BeNil())
					Expect(out).To(ContainSubstring(image.ID))
				})
			})

			Describe("AttachToContainer()", func() {
				It("attaches to container's stdin and stdout streams", func() {
					var stdout bytes.Buffer
					r := strings.NewReader("send value")
					errCh := make(chan error)
					go func() {
						errCh <- prov.AttachToContainer(id, r, &stdout, false)
					}()

					// FIXME Ideally we don't have to stop a container but a bug
					// in either go-dockerclient or Docker causes
					// AttachToContainer() to not return even after the output
					// stream is closed for writing.
					// This happens to containers that do not write any output.
					err := exec.Command("docker", "stop", id).Run()
					Expect(err).To(BeNil())

					Expect(<-errCh).To(BeNil())
				})
			})

			Describe("Logs()", func() {
				It("writes the container's logs", func() {
					// FIXME Ideally this should test against a container with
					// actual log output and probably exercise the follow,
					// timestamps and tail arguments.
					err := prov.Logs(id, ioutil.Discard, false, false, "")
					Expect(err).To(BeNil())
				})
			})

			Describe("VerifyToken()", func() {
				var correctToken string

				BeforeEach(func() {
					var err error
					correctToken, err = hmactoken.Generate([]string{"attach"}, secret)
					Expect(err).To(BeNil())
				})

				It("doesn't return an error when verifying a correct token", func() {
					err := prov.VerifyToken(id, correctToken, "attach")
					Expect(err).To(BeNil())
				})

				It("returns ErrUnverifiable when the container's shared secret cannot be read", func() {
					secretFile := path.Join(prov.ContainerSettingsDir(name), "secret")
					newpath := path.Join(prov.ContainerSettingsDir(name), "somewhere")
					err := os.Rename(secretFile, newpath)
					Expect(err).To(BeNil())
					defer os.Rename(newpath, secretFile)

					err = prov.VerifyToken(id, correctToken, "attach")
					Expect(err).To(MatchError(provider.ErrUnverifiable))
				})

				It("returns ErrTokenVerificationFailed when token is invalid", func() {
					invalidToken, err := hmactoken.Generate([]string{"attach"}, "wrong secret")
					Expect(err).To(BeNil())

					err = prov.VerifyToken(id, invalidToken, "attach")
					Expect(err).To(MatchError(provider.ErrTokenVerificationFailed))
				})

				It("returns ErrTokenVerificationFailed when token has incorrect permissions", func() {
					invalidToken, err := hmactoken.Generate([]string{"admin"}, secret)
					Expect(err).To(BeNil())

					err = prov.VerifyToken(id, invalidToken, "attach")
					Expect(err).To(MatchError(provider.ErrTokenVerificationFailed))
				})

				It("returns ErrTokenExpired when token has expired", func() {
					hmactoken.DefaultValidityPeriod = 0
					expiredToken, err := hmactoken.Generate([]string{"attach"}, secret)
					Expect(err).To(BeNil())

					err = prov.VerifyToken(id, expiredToken, "attach")
					Expect(err).To(MatchError(provider.ErrTokenExpired))
				})
			})
		})

		Describe("a container with ssh enabled", func() {
			var (
				name string
				id   string
				info *provider.ContainerInfo

				pubkey1      = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCxCeR/jJna6xXELDV8HF51QQPbj++JNDE4W/nqLyhzRTEDDnAfyp8gXW/EXXos8w5XXLak6eUDXE/QI6o8blX39RYoT47u45v4b2scN0ZiS5McmESo193KwWH3gXHbnHWywzPy7+GrRF3nHm/uidvp0tqh4ywqIZujaMjST1sIOQ== test@example.com"
				pubkey2      = strings.TrimSpace(string(TestSSHPubKey))
				fingerprint1 = "f7:de:4d:6f:54:6b:b9:d5:78:a9:62:15:e2:ad:ec:bb"
				fingerprint2 = "d7:39:a8:61:5a:fe:4f:57:48:68:18:1e:d7:a2:a6:eb"

				// set to false for tests which handle their own termination
				terminateAfterTest = true
			)

			BeforeEach(func() {
				var err error
				name = RandomName("freightertest")
				info, err = prov.CreateStart(name, "nitrousio/ubuntu-bare:latest", secret, "")
				Expect(err).To(BeNil())
				id = inspectID(name)
			})

			AfterEach(func() {
				if terminateAfterTest {
					CleanUpContainer(name)
				}
			})

			It("allows ssh access only with keys present", func() {
				hostname := "localhost"
				if ip := os.Getenv("DOCKER_IP"); ip != "" {
					hostname = ip
				}
				address := fmt.Sprintf("%s:%s", hostname,
					inspectNetworkSettingsPort(id, "22/tcp"))
				pk, err := ssh.ParseRawPrivateKey([]byte(TestSSHPrivKey))
				Expect(err).To(BeNil())
				privKey, ok := pk.(*rsa.PrivateKey)
				Expect(ok).To(BeTrue())

				// without keys
				sshutil.MaxAttempts = 1
				ch := make(chan struct{})
				_, err = sshutil.WaitForSSH(
					address,
					DfileEnvUser,
					privKey,
					500*time.Millisecond,
					ch,
					nil,
				)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("unable to authenticate"))

				// with keys
				err = prov.AddUserKeys(id, []string{pubkey1, pubkey2})
				Expect(err).To(BeNil())

				sshutil.MaxAttempts = 10
				_, err = sshutil.WaitForSSH(
					address,
					DfileEnvUser,
					privKey,
					500*time.Millisecond,
					ch,
					nil,
				)
				Expect(err).To(BeNil())
			})

			Describe(".ListUserKeys()", func() {
				It("returns list of pub keys and the corresponding fingerprints", func() {
					err := prov.AddUserKeys(id, []string{pubkey1, pubkey2})
					Expect(err).To(BeNil())

					// when container is running
					pubkeys, err := prov.ListUserKeys(id)
					Expect(err).To(BeNil())
					Expect(pubkeys).To(Equal(map[string]string{
						fingerprint1: pubkey1,
						fingerprint2: pubkey2,
					}))

					// when container is stopped
					err = exec.Command("docker", "stop", "--time=1", id).Run()
					Expect(err).To(BeNil())
					pubkeys, err = prov.ListUserKeys(id)
					Expect(err).To(BeNil())
					Expect(pubkeys).To(Equal(map[string]string{
						fingerprint1: pubkey1,
						fingerprint2: pubkey2,
					}))
				})
			})
		})
	})

	Describe(".GetInfo()", func() {
		It("returns container info", func() {
			name := RandomName("freightertest")
			configJSON := ""
			info, err := prov.CreateStart(name, NitrousImage, secret, configJSON)
			Expect(err).To(BeNil())
			defer CleanUpContainer(name)

			id := info.ID

			info, err = prov.GetInfo(id)
			Expect(err).To(BeNil())
			Expect(info.ID).To(Equal(id))
			Expect(info.Name).To(Equal(name))
			Expect(info.Image).To(Equal(NitrousImage))
			Expect(info.ImageID).To(Equal(inspectID(NitrousImage)))
			Expect(info.Host).To(Equal(inspectIP(id)))
			Expect(info.Gateway).To(Equal(inspectGateway(id)))
			Expect(info.User).To(Equal(inspectUser(id)))
			Expect(info.State).To(Equal(provider.ContainerStateRunning))
			Expect(fmt.Sprintf("[%s]", strings.Join(info.Env, " "))).To(Equal(inspectEnv(id)))
			sshPort := inspectNetworkSettingsPort(id, "22/tcp")
			Expect(info.Ports["22"]).To(ContainElement(sshPort))

			c := inspectContainerInfo(id)
			Expect(info.Cmd).To(Equal(c.Config.Cmd))
			Expect(info.Entrypoint).To(Equal(c.Config.Entrypoint))
			Expect(info.ExitCode).To(Equal(c.State.ExitCode))
			Expect(info.StartedAt).To(Equal(c.State.StartedAt))
			Expect(info.FinishedAt).To(Equal(c.State.FinishedAt))
		})
	})

	Describe(".ListImages()", func() {
		imageSize := func(id string) int64 {
			sizeInspect := doInspect(id, "{{.Size}}")
			parsedSize, err := strconv.ParseInt(sizeInspect, 10, 64)
			Expect(err).To(BeNil())
			return parsedSize
		}

		Context("with one image", func() {
			var (
				publicImages []string
			)

			BeforeEach(func() {
				publicImages = CreateImages(1, "basic", false, "")
			})

			AfterEach(func() {
				RemoveImages(publicImages)
			})

			It("should fetch basic info about an Image", func() {
				imageList, err := prov.ListImages()
				Expect(err).To(BeNil())

				i1 := imageList[0]

				Expect(i1.Name).To(Equal("basic"))
				Expect(i1.Tag).To(Equal("n1"))
				Expect(i1.ActualSize).To(Equal(imageSize(i1.ID)))
			})
		})

		Context("when the same image is under multiple repositories", func() {
			var (
				createdImages []string
			)

			BeforeEach(func() {
				createdImages = CreateImages(2, "multiple", true, "")
			})

			AfterEach(func() {
				RemoveImages(createdImages)
			})

			It("should break them out into separate entries", func() {
				images, err := prov.ListImages()
				Expect(err).To(BeNil())

				var i1, i2 *provider.ImageInfo
				for _, i := range images {
					if i.Name == "multiple" && i.Tag == "n1" {
						i1 = i
					}
					if i.Name == "multiple" && i.Tag == "n2" {
						i2 = i
					}
				}

				Expect(i1).NotTo(BeNil())
				Expect(i2).NotTo(BeNil())
				Expect(i1.ID).To(Equal(i2.ID))
			})
		})
	})

	Describe(".BuildImage()", func() {
		It("streams output from Docker", func() {
			prov, err := docker.NewDockerProviderWithHost(
				datadir,
				"tcp://127.0.0.1:1234", // TCP address: so that go-dockerclient uses HTTP.
			)
			Expect(err).To(BeNil())

			body := `
				{"stream":"Step 0 : FROM ubuntu:latest\n"}
				{"stream":" ---\u003e 4300eb9d3c8d\n"}
				{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
				{"stream":" ---\u003e Using cache\n"}
				{"stream":" ---\u003e 3a3ed758c370\n"}
				{"stream":"Step 2 : CMD /usr/bin/top\n"}
				{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
				{"stream":" ---\u003e 4b6188aebe39\n"}
				{"stream":"Removing intermediate container 36b1479cc2e4\n"}
				{"stream":"Successfully built 4b6188aebe39\n"}
			`
			// Replace dockerclient's HTTP client's RoundTripper with our fake
			// response.
			fakeRT := &FakeRoundTripper{
				message: body,
				status:  http.StatusOK,
				header: map[string]string{
					"Content-Type": "application/json",
				},
			}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: fakeRT}

			in := bytes.NewBufferString(`build context tarball contents`)
			out := bytes.NewBuffer(nil)
			opts := provider.BuildImageOptions{
				Name:          "myimage",
				InputStream:   in,
				OutputStream:  out,
				RawJSONStream: true,
			}
			err = prov.BuildImage(opts)
			Expect(err).To(BeNil())
			Expect(out.String()).To(Equal(body))

			req := fakeRT.requests[0]
			b, _ := ioutil.ReadAll(req.Body)
			Expect(string(b)).To(Equal(`build context tarball contents`))
			assertBuildImageOptions(req, dockerclient.BuildImageOptions{
				Name:                "myimage",
				RmTmpContainer:      true,
				ForceRmTmpContainer: true,
				RawJSONStream:       true,
			})
		})
	})

	Describe(".RemoveImage()", func() {
		Context("when an ID is provided", func() {
			It("should be deleted", func() {
				images := CreateImages(1, "removable", false, "")

				id := doInspect(images[0], "{{.Id}}")
				Expect(id).ToNot(BeEmpty())
				err := prov.RemoveImage(id)
				Expect(err).To(BeNil())

				out, _ := exec.Command("docker", "inspect", "-f", "{{.Id}}", images[0]).CombinedOutput()
				Expect(out).To(ContainSubstring("Error: No such image or container:"))
			})

			Context("with running containers using that ID", func() {
				It("should not delete but preserve the tag of the image", func() {
					i1 := CreateImages(1, "multremovable", true, "")
					defer RemoveImages(i1)

					id1 := doInspect(i1[0], "{{.Id}}")
					Expect(id1).ToNot(BeEmpty())

					containerId := strings.TrimRight(createContainer(i1[0]), "\n")
					Expect(containerId).ToNot(BeEmpty())

					err := prov.RemoveImage(i1[0])
					Expect(err).To(Equal(provider.ErrImageInUse))

					id1 = doInspect(i1[0], "{{.Id}}")
					Expect(id1).ToNot(BeEmpty())

					CleanUpContainer(containerId)
				})
			})

			Context("with multiple images with the same ID", func() {
				It("should not allow for deletion by ID", func() {
					i1 := CreateImages(1, "multremovable", true, "")
					defer RemoveImages(i1)
					i2 := CreateImages(1, "multremovable2", true, "")
					defer RemoveImages(i2)

					id1 := doInspect(i1[0], "{{.Id}}")
					id2 := doInspect(i2[0], "{{.Id}}")
					Expect(id1).ToNot(BeEmpty())
					Expect(id1).To(Equal(id2))

					err := prov.RemoveImage(id1)
					Expect(err).To(Equal(provider.ErrImageShared))

					id1 = doInspect(i1[0], "{{.Id}}")
					Expect(id1).ToNot(BeEmpty())
				})
			})
		})

		Context("when a Name is provided", func() {
			It("should be deleted", func() {
				images := CreateImages(1, "removable", false, "")

				err := prov.RemoveImage(images[0])
				Expect(err).To(BeNil())

				out, _ := exec.Command("docker", "inspect", "-f", "{{.Id}}", images[0]).CombinedOutput()
				Expect(out).To(ContainSubstring("Error: No such image or container:"))
			})
		})
	})

	Describe(".PullImage()", func() {
		var (
			tempAuthFile *os.File
			authFileSrc  string
		)

		BeforeEach(func() {
			var err error
			tempAuthFile, err = ioutil.TempFile("", "cfg")
			Expect(err).To(BeNil())

			/* credentials are
			test:1234xxcc
			testo:Password1
			magic:abc34x1
			*/
			authFileSrc = `
			{
				"https://index.docker.io/v1/": { "auth": "dGVzdDoxMjM0eHhjYwo=", "email": "docker@callumj.com" },
				"quay.io": { "auth": "dGVzdG86UGFzc3dvcmQxCg==", "email": "quay@callumj.com" },
				"cloud.inc": { "auth": "bWFnaWM6YWJjMzR4MQo=", "email": "cloud@callumj.com" },
				"magic.io": {}
			}
			`

			_, err = tempAuthFile.WriteString(authFileSrc)
			Expect(err).To(BeNil())
			err = tempAuthFile.Close()
			Expect(err).To(BeNil())

			// the tests uses FakeRoundTripper so we inject a random tcp address here
			// instead of using the unix socket so go-dockerclient can inherit
			// the FakeRoundTripper
			prov, err = docker.NewDockerProviderWithHost(datadir, "tcp://127.0.0.1:1234")
			Expect(err).To(BeNil())
			// FIXME: remove this stuff
			prov.DockerConfigLocation = tempAuthFile.Name()
		})

		AfterEach(func() {
			os.Remove(tempAuthFile.Name())
		})

		It("handles the new docker config.json registry auth format", func() {
			txt := fmt.Sprintf(`{ "auths": %s }`, authFileSrc)
			newAuthFile, err := ioutil.TempFile("", "newcfg")
			Expect(err).To(BeNil())
			_, err = newAuthFile.WriteString(txt)
			Expect(err).To(BeNil())
			err = newAuthFile.Close()
			Expect(err).To(BeNil())
			prov.DockerConfigLocation = newAuthFile.Name()

			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			err = prov.PullImage("nitrousio/nodejs", ioutil.Discard, true)
			Expect(err).To(BeNil())

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "nitrousio/nodejs",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{
				Username: "test",
				Password: "1234xxcc\n",
				Email:    "docker@callumj.com",
			})
		})

		It("streams the docker output", func() {
			body := `
			{"status":"Pulling..."}
			{"status":"Pulling", "progress":"1 B/ 100 B", "progressDetail":{"current":1, "total":100}}
			`
			rt := &FakeRoundTripper{
				message: body,
				status:  http.StatusOK,
				header: map[string]string{
					"Content-Type": "application/json",
				},
			}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}
			stdout := bytes.NewBuffer(nil)

			err := prov.PullImage("nitrousio/nodejs", stdout, true)
			Expect(err).To(BeNil())
			Expect(stdout.String()).To(Equal(body), "unexpected stream output")
		})

		It("should allow for importing by default hub", func() {
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			err := prov.PullImage("nitrousio/nodejs", ioutil.Discard, true)
			Expect(err).To(BeNil())

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "nitrousio/nodejs",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{
				Username: "test",
				Password: "1234xxcc\n",
				Email:    "docker@callumj.com",
			})
		})

		It("should allow for importing by a tagged image", func() {
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			err := prov.PullImage("ubuntu:10.04", ioutil.Discard, true)
			Expect(err).To(BeNil())

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "ubuntu",
				Tag:        "10.04",
			})
		})

		It("should allow for importing by simple remote registry", func() {
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			err := prov.PullImage("quay.io/ubuntu", ioutil.Discard, true)
			Expect(err).To(BeNil())

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "quay.io/ubuntu",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{
				Username: "testo",
				Password: "Password1\n",
				Email:    "quay@callumj.com",
			})
		})

		It("should allow for importing by remote registry", func() {
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			err := prov.PullImage("quay.io/quay/ubuntu", ioutil.Discard, true)
			Expect(err).To(BeNil())

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "quay.io/quay/ubuntu",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{
				Username: "testo",
				Password: "Password1\n",
				Email:    "quay@callumj.com",
			})
		})

		It("should allow even when auth credentials could not be located", func() {
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			prov.PullImage("cloudy.io/nodejs", ioutil.Discard, true)

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "cloudy.io/nodejs",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{})
		})

		It("should allow even a malformed config", func() {
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			prov.PullImage("magic.io/quay/ubuntu", ioutil.Discard, true)

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "magic.io/quay/ubuntu",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{})
		})

		It("should allow to pull without credentials when we dont have a auth configuration", func() {
			os.Remove(tempAuthFile.Name())
			rt := &FakeRoundTripper{message: "", status: http.StatusOK}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			prov.PullImage("cloudy.io/nodejs", ioutil.Discard, true)

			r := rt.requests[0]
			assertPullOptions(r, dockerclient.PullImageOptions{
				Repository: "cloudy.io/nodejs",
			})
			assertRegistryAuth(r, dockerclient.AuthConfiguration{})
		})

		It("should surface PullImage errors", func() {
			rt := &FakeRoundTripper{message: `{"error":"Sorry!"}`, status: http.StatusInternalServerError}
			cl := prov.Client()
			cl.HTTPClient = &http.Client{Transport: rt}

			stdout := bytes.NewBuffer(nil)
			err := prov.PullImage("quay/ubuntu", stdout, false)

			dockerErr, ok := err.(*dockerclient.Error)
			Expect(ok).To(BeTrue(), "invalid error type %q", err)
			Expect(dockerErr.Status).To(Equal(http.StatusInternalServerError), "invalid http status")
			Expect(dockerErr.Message).To(Equal(`{"error":"Sorry!"}`), "invalid error message")
		})
	})

	Describe(".TagImage()", func() {

		verifyImageExistence := func(name string) {
			parts := strings.Split(name, ":")
			Expect(parts).To(HaveLen(2))
			repo, tag := parts[0], parts[1]

			knownImages, err := prov.ListImages()
			Expect(err).To(BeNil())
			var targetImage *provider.ImageInfo
			for _, image := range knownImages {

				if image.Name == repo && image.Tag == tag {
					targetImage = image
					break
				}
			}

			Expect(targetImage).ToNot(BeNil())
			err = prov.RemoveImage(name)
			Expect(err).To(BeNil())
		}

		It("should allow for tagging images without tags", func() {
			created := CreateImages(1, "withoutly", false, "")
			err := prov.TagImage(created[0], "withoutly2", false)
			Expect(err).To(BeNil())

			verifyImageExistence("withoutly2:latest")
			RemoveImages(created)
		})

		It("should allow for tagging images with tags", func() {
			created := CreateImages(1, "tagly", false, "")
			err := prov.TagImage(created[0], "tagly2:1907", false)
			Expect(err).To(BeNil())

			verifyImageExistence("tagly2:1907")
			RemoveImages(created)
		})

		Context("image tag already exists", func() {
			It("should not override existing tags with no force", func() {
				created := CreateImages(1, "nooverridely", false, "")
				err := prov.TagImage(created[0], "nooverridely:n1", false)
				Expect(err).To(Equal(provider.ErrImageExists))
				RemoveImages(created)
			})

			It("should override existing tags with force", func() {
				target := CreateImages(1, "overridely", false, "")
				destination := CreateImages(1, "overridelyreplacey", false, "")
				curId := doInspect(destination[0], "{{.Id}}")

				err := prov.TagImage(target[0], "overridelyreplacey:n1", true)
				Expect(err).To(BeNil())

				newId := inspectID("overridelyreplacey:n1")
				Expect(newId).ToNot(Equal(curId))
				RemoveImages(target)
				RemoveImages(destination)
			})
		})
	})

	Describe(".PushImage()", func() {
		It("should not push if the image is not accessible", func() {
			err := prov.PushImage("invalid-image-name", "dont-care", nil, true)
			Expect(err).To(Equal(provider.ErrImageNotFound))
		})

		Context("with an dockercfg authentication file", func() {
			var (
				mockProxy    *MockDockerProxy
				tempAuthFile *os.File
				srcImage     string
			)

			BeforeEach(func() {
				var err error

				images := CreateImages(1, "namely", false, "")
				srcImage = images[0]

				mockProxy = &MockDockerProxy{}
				tempAuthFile, err = ioutil.TempFile("", "cfg")
				Expect(err).To(BeNil())

				/* credentials are
				test:1234xxcc
				testo:Password1
				magic:abc34x1
				*/
				authFileSrc := `
				{
				  "https://index.docker.io/v1/": {
				    "auth": "dGVzdDoxMjM0eHhjYwo=",
				    "email": "docker@callumj.com"
				  },
				  "quay.io": {
				    "auth": "dGVzdG86UGFzc3dvcmQxCg==",
				    "email": "quay@callumj.com"
				  },
				  "cloud.inc": {
				    "auth": "bWFnaWM6YWJjMzR4MQo=",
				    "email": "cloud@callumj.com"
				  },
				  "magic.io": {}
				}
				`

				_, err = tempAuthFile.WriteString(authFileSrc)
				Expect(err).To(BeNil())
				err = tempAuthFile.Close()
				Expect(err).To(BeNil())

				prov.DockerClientProxy = mockProxy
				prov.DockerConfigLocation = tempAuthFile.Name()
			})

			AfterEach(func() {
				RemoveImages([]string{srcImage})
				os.Remove(tempAuthFile.Name())
			})

			It("should support pushing to the default Docker registry endpoint", func() {
				output := bytes.NewBuffer(nil)
				err := prov.PushImage(srcImage, "callumj/test1:n1", output, true)
				defer RemoveImages([]string{"callumj/test1:n1"})
				Expect(err).To(BeNil())

				Expect(mockProxy.PushImageCalled).To(BeTrue())
				Expect(mockProxy.PushImageOpts.Name).To(Equal("callumj/test1"))
				Expect(mockProxy.PushImageOpts.Tag).To(Equal("n1"))
				Expect(mockProxy.PushImageOpts.OutputStream).To(Equal(output))
				Expect(mockProxy.PushImageOpts.RawJSONStream).To(Equal(true))
				Expect(mockProxy.PushImageAuth.Username).To(Equal("test"))
				Expect(mockProxy.PushImageAuth.Password).To(ContainSubstring("1234xxcc"))
				Expect(mockProxy.PushImageAuth.Email).To(Equal("docker@callumj.com"))
			})

			It("does not set tag if not specified", func() {
				err := prov.PushImage(srcImage, "callumj/test1", nil, true)
				defer RemoveImages([]string{"callumj/test1"})
				Expect(err).To(BeNil())

				Expect(mockProxy.PushImageCalled).To(BeTrue())
				Expect(mockProxy.PushImageOpts.Tag).To(Equal(""))
			})

			It("sets tag if specified", func() {
				err := prov.PushImage(srcImage, "callumj/test1:latest", nil, true)
				defer RemoveImages([]string{"callumj/test1:latest"})
				Expect(err).To(BeNil())

				Expect(mockProxy.PushImageCalled).To(BeTrue())
				Expect(mockProxy.PushImageOpts.Tag).To(Equal("latest"))
			})

			It("should support pushing to an alternative registry endpoint", func() {
				err := prov.PushImage(srcImage, "quay.io/callumj/test1:tag", nil, true)
				defer RemoveImages([]string{"quay.io/callumj/test1:tag"})
				Expect(err).To(BeNil())

				Expect(mockProxy.PushImageCalled).To(BeTrue())
				Expect(mockProxy.PushImageOpts.Name).To(Equal("quay.io/callumj/test1"))
				Expect(mockProxy.PushImageOpts.Tag).To(Equal("tag"))
				Expect(mockProxy.PushImageAuth.Username).To(Equal("testo"))
				Expect(mockProxy.PushImageAuth.Password).To(ContainSubstring("Password1"))
				Expect(mockProxy.PushImageAuth.Email).To(Equal("quay@callumj.com"))
			})

			It("pushes the image if destination repository name is not specified", func() {
				name := srcImage
				repo, tag := parsers.ParseRepoAndTag(name)

				err := prov.PushImage(name, "", nil, true)
				Expect(err).To(BeNil())

				Expect(mockProxy.PushImageCalled).To(BeTrue())
				Expect(mockProxy.PushImageOpts.Name).To(Equal(repo))
				Expect(mockProxy.PushImageOpts.Tag).To(Equal(tag))
			})

			It("tags the image with the destination repository name", func() {
				err := prov.PushImage(srcImage, "callumj/test1:mytag", nil, true)
				defer RemoveImages([]string{"callumj/test1:mytag"})
				Expect(err).To(BeNil())

				srcID := doInspect(srcImage, "{{.Id}}")
				destID := doInspect("callumj/test1:mytag", "{{.Id}}")
				Expect(destID).NotTo(BeEmpty())
				Expect(destID).To(Equal(srcID))
			})

			Context("when destination repository already exists locally", func() {
				var (
					existingImageName = "test/existing"
					existingImageID   string
				)

				BeforeEach(func() {
					err := exec.Command("docker", "tag", "-f", NitrousImage, existingImageName).Run()
					Expect(err).To(BeNil())
					o, err := exec.Command("docker", "inspect", "-f", "{{.Id}}", existingImageName).CombinedOutput()
					existingImageID = strings.TrimSpace(string(o))
					Expect(err).To(BeNil())
					Expect(existingImageID).NotTo(BeEmpty())
				})

				AfterEach(func() {
					RemoveImages([]string{existingImageName})
				})

				It("overrides the local image by tagging it", func() {
					err := prov.PushImage(srcImage, existingImageName, nil, true)
					Expect(err).To(BeNil())

					Expect(mockProxy.PushImageCalled).To(BeTrue())
					Expect(mockProxy.PushImageOpts.Name).To(Equal(existingImageName))
					Expect(mockProxy.PushImageOpts.Tag).To(Equal(""))

					newID := doInspect(existingImageName, "{{.Id}}")
					Expect(newID).NotTo(Equal(existingImageID))
				})
			})

			It("returns an error when auth credentials could not be located", func() {
				err := prov.PushImage(srcImage, "thingyquay.io/callumj/test1", nil, true)
				Expect(err).To(Equal(provider.ErrImagePushAuthRequired))
				Expect(mockProxy.PushImageCalled).To(BeFalse())
			})

			It("returns an error when auth config is malformed", func() {
				err := prov.PushImage(srcImage, "magic.io/callumj/test1", nil, true)
				Expect(err).To(Equal(provider.ErrImagePushAuthRequired))
				Expect(mockProxy.PushImageCalled).To(BeFalse())
			})

			It("should surface PushImage errors", func() {
				e := errors.New("Sorry!")
				mockProxy.PushImageError = e
				err := prov.PushImage(srcImage, "callumj/test1", nil, true)
				defer RemoveImages([]string{"callumj/test1"})
				Expect(err).To(Equal(e))
				Expect(mockProxy.PushImageCalled).To(BeTrue())
			})
		})
	})

	Describe("ContainerSettingsDir()", func() {
		It("is in subdirectory 'containers' in base freighter dir", func() {
			d := prov.ContainerSettingsDir("containername")
			Expect(d).To(Equal(fmt.Sprintf("%s/containers/containername", datadir)))
		})
	})

	Describe("IsNitrousImage()", func() {
		cases := []struct {
			dockerFile string
			expected   bool
		}{
			{`FROM busybox
				MAINTAINER callum@action.io
				ENV No WAY
				ENV NITROUS_IMAGE_VERSION 1.1`, true},
			{`FROM busybox
				MAINTAINER callum@action.io
				ENV NITROUS_IMAGE_VERSION 1.1.1.1`, true},
			{`FROM busybox
				MAINTAINER callum@action.io
				ENV NOTROUS_IMAGE_VERSION 0.1`, false},
			{`FROM busybox
				MAINTAINER callum@action.io`, false},
			{`FROM busybox
				MAINTAINER callum@action.io
				ENV SOME THING`, false},
		}

		for _, c2 := range cases {
			c := c2
			It(fmt.Sprintf("is %v for Dockerfile '%s'", c.expected, c.dockerFile), func() {
				imageList := CreateImages(1, "isnitrous", false, c.dockerFile)
				defer RemoveImages(imageList)
				Expect(prov.IsNitrousImage(imageList[0])).To(Equal(c.expected))
			})
		}

		It("should be true when an image inherits", func() {
			baseDocker := `FROM busybox
			MAINTAINER callum@action.io
			ENV No WAY
			ENV NITROUS_IMAGE_VERSION 1.1`
			createdBase := CreateImages(1, "isnitbase", false, baseDocker)
			defer RemoveImages(createdBase)
			childDocker := fmt.Sprintf(`FROM %v
				MAINTAINER callum@callumj.com`, createdBase[0])
			createdChild := CreateImages(1, "isnitchild", false, childDocker)
			defer RemoveImages(createdChild)

			Expect(prov.IsNitrousImage(createdChild[0])).To(BeTrue())
		})

		It("should be true when an image overrides", func() {
			baseDocker := `FROM busybox
			MAINTAINER callum@action.io
			ENV No WAY
			ENV NITROUS_IMAGE_VERSION 1.1`
			createdBase := CreateImages(1, "isnitbase", false, baseDocker)
			defer RemoveImages(createdBase)
			childDocker := fmt.Sprintf(`FROM %v
				MAINTAINER callum@callumj.com
				ENV NITROUS_IMAGE_VERSION 1.2`, createdBase[0])
			createdChild := CreateImages(1, "isnitchild", false, childDocker)
			defer RemoveImages(createdChild)

			Expect(prov.IsNitrousImage(createdChild[0])).To(BeTrue())
		})
	})

	Describe("InspectRemoteImage()", func() {
		Context("with an alternative repository", func() {
			var server *ghttp.Server
			var host string
			var prevParseRegistry func(string) string

			BeforeEach(func() {
				server = ghttp.NewTLSServer()
				parsedURL, err := url.Parse(server.URL())
				Expect(err).To(BeNil())
				host = parsedURL.Host

				certs := x509.NewCertPool()
				ok := certs.AppendCertsFromPEM(GoLocalhostCert)
				Expect(ok).To(BeTrue())

				cl := prov.Client()
				cl.HTTPClient = &http.Client{Transport: &http.Transport{
					TLSClientConfig: &tls.Config{RootCAs: certs},
				}}

				prevParseRegistry = parsers.ParseRegistry
				parsers.ParseRegistry = func(repos string) string {
					if strings.HasPrefix(repos, host) {
						return host
					}

					return prevParseRegistry(repos)
				}
			})

			AfterEach(func() {
				server.Close()
				parsers.ParseRegistry = prevParseRegistry
			})

			It("should return not found if the image cannot be found", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/images"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusNotFound, ""),
					),
				)

				_, err := prov.InspectRemoteImage(fmt.Sprintf("%s/centos", host))
				Expect(err).ToNot(BeNil())
				Expect(err).To(MatchError(provider.ErrImageNotFound))
			})

			Context("registry requires authentication", func() {
				It("should return authentication failure if the credentials are not provided", func() {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/v1/repositories/centos/images"),
							ghttp.RespondWith(http.StatusUnauthorized, ""),
						),
					)

					_, err := prov.InspectRemoteImage(fmt.Sprintf("%s/centos", host))
					Expect(err).ToNot(BeNil())
					Expect(err).To(MatchError(provider.ErrImageAuthNotAllowed))
				})

				It("should pass through authentication configuration if available", func() {
					authFileSrc := `
					{
					  "%s": {
					    "auth": "Y2FsbHVtajpQYXNzd29yZDEK",
					    "email": "docker@callumj.com"
					  }
					}
					`
					tempAuthFile, err := ioutil.TempFile("", "cfg")
					Expect(err).To(BeNil())
					defer os.Remove(tempAuthFile.Name())
					_, err = tempAuthFile.WriteString(fmt.Sprintf(authFileSrc, host))
					Expect(err).To(BeNil())
					err = tempAuthFile.Close()
					Expect(err).To(BeNil())

					prov.DockerConfigLocation = tempAuthFile.Name()

					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/v1/repositories/centos/images"),
							ghttp.VerifyHeader(http.Header{
								"Authorization": []string{"Y2FsbHVtajpQYXNzd29yZDEK"},
							}),
							ghttp.RespondWith(http.StatusNotFound, ""),
						),
					)

					_, err = prov.InspectRemoteImage(fmt.Sprintf("%s/centos", host))
					Expect(err).ToNot(BeNil())
					Expect(err).To(MatchError(provider.ErrImageNotFound))
				})
			})

			It("should return not found if it cannot find the corresponding tag", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/images"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusOK, `[
							{"checksum":"xcb23xansdjsad722sads", "id": "d1"},
							{"checksum":"xcb23b72ddasdbh423sd", "id": "d2"}
						]`),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/tags"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusOK, `[
							{"layer":"xcb23x", "name": "v1"},
							{"layer":"xcb23x", "name": "v2"}
						]`),
					),
				)

				_, err := prov.InspectRemoteImage(fmt.Sprintf("%s/centos:v3", host))
				Expect(err).ToNot(BeNil())
				Expect(err).To(MatchError(provider.ErrImageNotFound))
			})

			It("should subsequently query the image ID if the tag is found, sending the Docker Token", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/images"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusOK, `[
							{"checksum":"", "id": "xcb23xansdjsad722sads"},
							{"checksum":"", "id": "xcb23b72ddasdbh423sd"}
						]`, http.Header{"X-Docker-Token": []string{`signature=X23x8X,repository="centos",permission="read"`}}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/tags"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusOK, `[
							{"layer":"xcb23x", "name": "v1"},
							{"layer":"xcb23b", "name": "v3"}
						]`),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/xcb23b72ddasdbh423sd/json"),
						ghttp.VerifyHeader(http.Header{
							"Authorization": []string{`Token signature=X23x8X,repository="centos",permission="read"`},
						}),
						ghttp.RespondWith(http.StatusNotFound, ""),
					),
				)

				_, err := prov.InspectRemoteImage(fmt.Sprintf("%s/centos:v3", host))
				Expect(err).ToNot(BeNil())
				Expect(err).To(MatchError(provider.ErrImageNotFound))
			})

			It("should be able to decode the JSON", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/images"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusOK, `[
							{"checksum":"", "id": "xcb23xansdjsad722sads"},
							{"checksum":"", "id": "xcb23b72ddasdbh423sd"}
						]`, http.Header{"X-Docker-Token": []string{`signature=X23x8X,repository="centos",permission="read"`}}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/repositories/centos/tags"),
						ghttp.VerifyHeader(http.Header{
							"X-Docker-Token": []string{"true"},
							"Accept":         []string{"application/json"},
						}),
						ghttp.RespondWith(http.StatusOK, `[
							{"layer":"xcb23x", "name": "v1"},
							{"layer":"xcb23b", "name": "v3"}
						]`),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/v1/images/xcb23b72ddasdbh423sd/json"),
						ghttp.VerifyHeader(http.Header{
							"Authorization": []string{`Token signature=X23x8X,repository="centos",permission="read"`},
						}),
						ghttp.RespondWith(http.StatusOK, dockerRemoteInspectBody),
					),
				)

				img, err := prov.InspectRemoteImage(fmt.Sprintf("%s/centos:v3", host))
				Expect(err).To(BeNil())
				Expect(img.ID).To(Equal("179fcfebcf3e4bc4ec59e94023ba18160e303e528ac91c371c581222ed981a6f"))
				Expect(img.ActualSize).To(Equal(int64(200667946)))
				Expect(img.Name).To(Equal("centos"))
				Expect(img.Tag).To(Equal("v3"))
				Expect(img.Config).ToNot(BeNil())
				Expect(img.Config.Image).To(Equal(img.ID))
				Expect(img.Config.PortSpecs).To(Equal([]string{"5432:5432"}))
				Expect(img.Config.ExposedPorts).To(HaveKey("22/tcp"))
				Expect(img.Config.Env).To(Equal([]string{
					"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
					"container=docker",
					"NITROUS_IMAGE_VERSION=1",
				}))
				Expect(img.Config.Cmd).To(Equal([]string{"/sbin/init"}))
				Expect(img.Config.Volumes).To(HaveKey("/home:/tmp"))
			})
		})
	})

	Describe("gatewayFromIPOutput", func() {
		It("returns the correct gateway", func() {
			cases := []struct {
				gateway string
				output  string
			}{
				{"10.1.42.1", // positive example, with -family inet
					`2: docker0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default
    inet 10.1.42.1/16 scope global docker0
    valid_lft forever preferred_lft forever`,
				},
				{"172.17.42.1", // positive example, with multiple families
					`2: docker0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default
    link/ether 56:84:7a:fe:97:99 brd ff:ff:ff:ff:ff:ff
    inet 172.17.42.1/16 scope global docker0
        valid_lft forever preferred_lft forever
    inet6 fe80::5484:7aff:fefe:9799/64 scope link
        valid_lft forever preferred_lft forever`,
				},
				{"", ``},
				{"", // no inet
					`3: docker0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500
   inet6 fe80::5484:7aff:fefe:9799/64 scope link
   valid_lft forever preferred_lft forever`,
				},
			}

			for _, c := range cases {
				gateway, err := docker.GatewayFromIPOutput([]byte(c.output))
				if c.gateway == "" {
					Expect(err).To(MatchError("no inet ip address found"))
				} else {
					Expect(gateway).To(Equal(c.gateway), "got wrong gateway '%s' for output %s", c.gateway, c.output)
				}
			}
		})
	})

})

// FakeRoundTripper implements a fake Transport layer that records the executed
// requests and provide a fake response
//
// This is more lightweight than implementing a full HTTP server and should be
// approriate for some tests
type FakeRoundTripper struct {
	message  string
	status   int
	header   map[string]string
	requests []*http.Request
}

func (rt *FakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	body := strings.NewReader(rt.message)
	rt.requests = append(rt.requests, r)
	res := &http.Response{
		StatusCode: rt.status,
		Body:       ioutil.NopCloser(body),
		Header:     make(http.Header),
	}
	for k, v := range rt.header {
		res.Header.Set(k, v)
	}
	return res, nil
}

func (rt *FakeRoundTripper) Reset() {
	rt.requests = nil
}

func (rt *FakeRoundTripper) CancelRequest(req *http.Request) {
	// No-op. Satisfy http.Transport.CancelRequest() that's needed when
	// setting timeout on a http.Client.
}

// parses base64-encoded auth information string
func parseRegistryAuth(s string) (dockerclient.AuthConfiguration, error) {
	a := dockerclient.AuthConfiguration{}
	b, err := base64.URLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return a, err
	}
	return a, nil
}

// asserts that docker API receives the correct auth information
// refers to https://docs.docker.com/reference/api/docker_remote_api_v1.13/#22-images
// for more documentation
func assertRegistryAuth(r *http.Request, expected dockerclient.AuthConfiguration) {
	auth, err := parseRegistryAuth(r.Header.Get("X-Registry-Auth"))
	Expect(err).To(BeNil(), "failed to decode auth data in header")
	Expect(auth).To(Equal(expected), "invalid auth configuration")
}

// asserts that docker API receives the correct pull information
// refers to https://docs.docker.com/reference/api/docker_remote_api_v1.13/#22-images
// for more documentation
func assertPullOptions(r *http.Request, expected dockerclient.PullImageOptions) {
	// if we dont expect any tag, by default it only pulls the latest tag
	// https://github.com/nitrous-io/dedicated/freighter/commit/f3af226068ddd6eb697a58be38b2acb521e8e0b4
	if expected.Tag == "" {
		expected.Tag = "latest"
	}
	repo := r.FormValue("fromImage")
	registry := r.FormValue("registry")
	tag := r.FormValue("tag")
	Expect(repo).To(Equal(expected.Repository), "invalid PullImage repository")
	Expect(registry).To(Equal(expected.Registry), "invalid PullImage registry")
	Expect(tag).To(Equal(expected.Tag), "invalid PullImage tag")
}

func assertBuildImageOptions(r *http.Request, expected dockerclient.BuildImageOptions) {
	name := r.FormValue("t")
	rm := r.FormValue("rm")
	forcerm := r.FormValue("forcerm")
	Expect(name).To(Equal(expected.Name))
	if expected.RmTmpContainer {
		Expect(rm).To(Equal("1"))
	}
	if expected.ForceRmTmpContainer {
		Expect(forcerm).To(Equal("1"))
	}
}

const dockerRemoteInspectBody = `{
    "Size": 200667946,
    "architecture": "amd64",
    "author": "Nitrous.IO <hello@nitrous.io>",
    "config": {
        "AttachStderr": false,
        "AttachStdin": false,
        "AttachStdout": false,
        "Cmd": [
            "/sbin/init"
        ],
        "CpuShares": 0,
        "Cpuset": "",
        "Domainname": "",
        "Entrypoint": null,
        "Env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "container=docker",
            "NITROUS_IMAGE_VERSION=1"
        ],
        "ExposedPorts": {
            "22/tcp": {}
        },
        "Hostname": "265bbe22abe7",
        "Image": "2a234d7d7ee172d42288269c27e5b564ca9cd73862db6b63ce69fa5406fa110c",
        "Memory": 0,
        "MemorySwap": 0,
        "NetworkDisabled": false,
        "OnBuild": [],
        "OpenStdin": false,
        "PortSpecs": ["5432:5432"],
        "StdinOnce": false,
        "Tty": false,
        "User": "",
        "Volumes": {
            "/home:/tmp": {}
        },
        "WorkingDir": ""
    },
    "container": "22efabc557c8f04a8e22030430a034930ac142598cde45f4da74cf475c000a42",
    "created": "2014-10-04T00:00:24.079051046Z",
    "docker_version": "1.2.0",
    "id": "179fcfebcf3e4bc4ec59e94023ba18160e303e528ac91c371c581222ed981a6f",
    "os": "linux",
    "parent": "2a234d7d7ee172d42288269c27e5b564ca9cd73862db6b63ce69fa5406fa110c"
}
`
