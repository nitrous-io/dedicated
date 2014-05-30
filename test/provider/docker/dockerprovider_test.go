package dockertest

import (
	//. "github.com/nitrous-io/freighter/test"

	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/nitrous-io/freighter/datastore"
	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/provider/docker"
	. "github.com/nitrous-io/freighter/test"
	"github.com/nitrous-io/freighter/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DockerProvider", func() {
	var (
		store  *datastore.Datastore
		dbFile string
		prov   *docker.DockerProvider
	)

	BeforeEach(func() {
		var err error
		dbFile = TempFile("sqlite3")
		store, err = datastore.NewSqliteDatastore(dbFile)
		Expect(err).To(BeNil())

		err = store.Set("port:65001", "0")
		Expect(err).To(BeNil())

		datadir, err := TempDir()
		Expect(err).To(BeNil())
		prov, err = docker.NewDockerProvider(store, datadir)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if store != nil && store.Conn != nil {
			store.Conn.Close()
		}
		os.Remove(dbFile)
	})

	Describe(".Create()", func() {
		var (
			name         string
			id           string
			info         *provider.ContainerInfo
			pubkey1      = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCxCeR/jJna6xXELDV8HF51QQPbj++JNDE4W/nqLyhzRTEDDnAfyp8gXW/EXXos8w5XXLak6eUDXE/QI6o8blX39RYoT47u45v4b2scN0ZiS5McmESo193KwWH3gXHbnHWywzPy7+GrRF3nHm/uidvp0tqh4ywqIZujaMjST1sIOQ== test@example.com"
			pubkey2      string
			fingerprint1 = "f7:de:4d:6f:54:6b:b9:d5:78:a9:62:15:e2:ad:ec:bb"
			fingerprint2 string
		)

		BeforeEach(func() {
			name = RandomName("test")
			userPubkey, err := ioutil.ReadFile(path.Join(os.Getenv("HOME"), ".ssh", "id_rsa.pub"))
			Expect(err).To(BeNil())
			pubkey2 = strings.TrimSpace(string(userPubkey))
			fingerprint2, err = util.Md5Fingerprint(pubkey2)
			Expect(err).To(BeNil())

			info, err = prov.Create(name, "ubuntu-upstart:trusty", []string{pubkey1, pubkey2})
			Expect(err).To(BeNil())
			bid, err := exec.Command("docker", "inspect", "-f", "{{.ID}}", name).CombinedOutput()
			Expect(err).To(BeNil())
			id = strings.TrimSpace(string(bid))
		})

		AfterEach(func() {
			exec.Command("docker", "stop", name).Run()
			exec.Command("docker", "rm", name).Run()
			exec.Command("docker", "rmi", "image-", name).Run()
		})

		expectSSHToWork := func() {
			time.Sleep(1 * time.Second)
			out, err := exec.Command("ssh", "action@localhost", "-p", "65001", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "echo success").CombinedOutput()
			Expect(out).To(ContainSubstring("success"))
			Expect(err).To(BeNil())
		}

		It("creates a container successfully", func() {
			Expect(info.Id).To(Equal(id))
			Expect(info.Name).To(Equal(name))
			Expect(info.Image).To(Equal("image-" + name))
			Expect(info.Port).To(Equal(uint16(65001)))
			Expect(info.State).To(Equal(provider.ContainerStateRunning))
			portUsed, err := store.Get("port:65001")
			Expect(err).To(BeNil())
			Expect(portUsed).To(Equal("1"))
			expectSSHToWork()
		})

		Context("after container is created", func() {
			Describe(".GetInfo()", func() {
				It("returns container info", func() {
					info, err := prov.GetInfo(id)
					Expect(err).To(BeNil())
					Expect(info.Id).To(Equal(id))
					Expect(info.Name).To(Equal(name))
					Expect(info.Image).To(Equal("image-" + name))
					Expect(info.Port).To(Equal(uint16(65001)))
					Expect(info.State).To(Equal(provider.ContainerStateRunning))
				})
			})

			Describe(".Start()", func() {
				expectContainerToBeRunning := func() {
					out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
					Expect(err).To(BeNil())
					Expect(out).To(ContainSubstring("true"))
					expectSSHToWork()
				}

				JustBeforeEach(func() {
					err := prov.Start(id)
					Expect(err).To(BeNil())
				})

				Context("when the container is running", func() {
					It("does nothing", func() {
						expectContainerToBeRunning()
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("starts the container", func() {
						expectContainerToBeRunning()
					})
				})
			})

			Describe(".Stop()", func() {
				expectContainerToBeStopped := func() {
					out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
					Expect(err).To(BeNil())
					Expect(out).To(ContainSubstring("false"))
				}

				JustBeforeEach(func() {
					err := prov.Stop(id)
					Expect(err).To(BeNil())
				})

				Context("when the container is running", func() {
					It("stops the container", func() {
						expectContainerToBeStopped()
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("does nothing", func() {
						expectContainerToBeStopped()
					})
				})
			})

			Describe(".Restart()", func() {
				Context("when the container is running", func() {
					It("restarts the container", func() {
						out, err := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						prevStartTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(out)))
						Expect(err).To(BeNil())
						err = prov.Restart(id)
						Expect(err).To(BeNil())
						out, err = exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						Expect(out).To(ContainSubstring("true"))
						out, err = exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						newStartTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(out)))
						Expect(prevStartTime.Before(newStartTime)).To(BeTrue())
						expectSSHToWork()
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("starts the container", func() {
						err := prov.Restart(id)
						Expect(err).To(BeNil())
						out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
						Expect(err).To(BeNil())
						Expect(out).To(ContainSubstring("true"))
						expectSSHToWork()
					})
				})
			})

			Describe(".Terminate()", func() {
				expectContainerToBeTerminatedAndPortReleased := func() {
					out, err := exec.Command("docker", "inspect", id).CombinedOutput()
					Expect(err).NotTo(BeNil())
					Expect(out).To(ContainSubstring("No such image or container"))
					portUsed, err := store.Get("port:65001")
					Expect(err).To(BeNil())
					Expect(portUsed).To(Equal("0"))
				}

				JustBeforeEach(func() {
					err := prov.Terminate(id)
					Expect(err).To(BeNil())
				})

				Context("when the container is running", func() {
					It("terminates the container", func() {
						expectContainerToBeTerminatedAndPortReleased()
					})
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					It("terminates the container", func() {
						expectContainerToBeTerminatedAndPortReleased()
					})
				})
			})

			Describe(".ListPublicKeys()", func() {
				itWorks := func() {
					It("returns list of pub keys and the corresponding fingerprints", func() {
						pubkeys, err := prov.ListPublicKeys(id)
						Expect(err).To(BeNil())
						Expect(pubkeys).To(Equal(map[string]string{
							fingerprint1: pubkey1,
							fingerprint2: pubkey2,
						}))
					})
				}

				Context("when the container is running", func() {
					itWorks()
				})

				Context("when the container is stopped", func() {
					BeforeEach(func() {
						err := exec.Command("docker", "stop", id).Run()
						Expect(err).To(BeNil())
					})

					itWorks()
				})
			})
		})
	})
})
