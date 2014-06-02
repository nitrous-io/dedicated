package dockertest

import (
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
		store     *datastore.Datastore
		dbFile    string
		prov      *docker.DockerProvider
		namespace = "freightertest"
	)

	BeforeEach(func() {
		var err error
		dbFile = TempFile("sqlite3")
		store, err = datastore.NewSqliteDatastore(dbFile)
		Expect(err).To(BeNil())

		err = store.Set("port:65001", "0")
		Expect(err).To(BeNil())
		err = store.Set("port:65002", "0")
		Expect(err).To(BeNil())

		datadir, err := TempDir()
		Expect(err).To(BeNil())
		prov, err = docker.NewDockerProvider(store, datadir, namespace)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if store != nil && store.Conn != nil {
			store.Conn.Close()
		}
		os.Remove(dbFile)
	})

	cleanUpContainer := func(name string) {
		exec.Command("docker", "stop", name).Run()
		exec.Command("docker", "rm", name).Run()
		exec.Command("docker", "rmi", namespace+"."+name).Run()
	}

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
			name = RandomName("freightertest")
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
			cleanUpContainer(name)
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
			Expect(info.Image).To(Equal(namespace + "." + name))
			Expect(info.Port).To(Equal(uint16(65001)))
			Expect(info.State).To(Equal(provider.ContainerStateRunning))
			portUsed, err := store.Get("port:65001")
			Expect(err).To(BeNil())
			Expect(portUsed).To(Equal("1"))
			expectSSHToWork()
		})

		Context("after container is created", func() {
			Describe(".List()", func() {
				var (
					container2name string
					container2info *provider.ContainerInfo
				)

				BeforeEach(func() {
					container2name = RandomName("freightertest2")
					// shut down first container
					err := exec.Command("docker", "stop", id).Run()
					Expect(err).To(BeNil())

					// create another container
					container2info, err = prov.Create(container2name, "ubuntu-upstart:trusty", []string{})
					Expect(err).To(BeNil())
				})

				AfterEach(func() {
					cleanUpContainer(container2name)
				})

				It("returns a list of all containers, running or stopped", func() {
					list, err := prov.List()
					Expect(err).To(BeNil())

					Expect(list).To(HaveLen(2))
					Expect(list).To(ContainElement(&provider.ContainerInfo{
						Id:    id,
						Name:  name,
						Image: namespace + "." + name,
						Port:  uint16(65001),
						State: provider.ContainerStateStopped,
					}))

					Expect(list).To(ContainElement(&provider.ContainerInfo{
						Id:    container2info.Id,
						Name:  container2name,
						Image: namespace + "." + container2name,
						Port:  uint16(65002),
						State: provider.ContainerStateRunning,
					}))
				})
			})

			Describe(".GetInfo()", func() {
				It("returns container info", func() {
					info, err := prov.GetInfo(id)
					Expect(err).To(BeNil())
					Expect(info.Id).To(Equal(id))
					Expect(info.Name).To(Equal(name))
					Expect(info.Image).To(Equal(namespace + "." + name))
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

			Describe(".AddPublicKeys()", func() {
				pubkey3 := "ssh-dss AAAAB3NzaC1kc3MAAACBAKPiMSg6vvWZAK1a+Vj+yPH0kRpCDUqhNCyPJd8yIfvs8c+uGgnXkDG6vW3gamyxkFilxgLvqW89NnvmcBRoAXkrT5e8uiciLJ2CqJuqZPHFNf3T298xFleh/43X5dxgS6CdODzrkreFe8XD1R1DozMgChU8wGVpQNQWGKU/x7xXAAAAFQCa8OP3WB4V2mNA/8cN7fj4gxDbLwAAAIEAhxHz0xu/EAWvq+nZ5CXnWZ2g++t1IEJcJWnTTIQakU+aZV0DxQ4hesrFiXllBX/oUTgr8ZlqYgqTy4+lHpXIhesNy3k9OjjU8/dYm9zZZw3TWIL64dAEp9L0xNZ7MFgOVvDwpjNqjUrbDCIujk7iZ+J/hFFoTqRxSeciX+cbusQAAACBAIcGybaGJ2Z4E21jR0iD05kNhJm97CJHavhH6fgxkeecVdXU2+k/DjLHTz7UfhbAtg6XqE0s5PVUgh0P/stQRGyosTmasYE3hTwwOXYQ2k/HLqM/ZGwEXW2kLSpi2MROowQf25wQrWmJBjTQ8mjG5jm0KOCX2jFf0QN3qTzVovSc vagrant@ubuntu-14"

				Context("with valid public keys", func() {
					var (
						pubkey4      = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBIC/WqXhoJC0FSaocOrx/uyneeqH32CUNbva4L6PdY7mIZ99mTTpdREowMo1T3pvY46JxwL6X3stuhZmIYNJEc0="
						fingerprint3 = "9b:e8:01:40:ff:35:c5:12:05:dc:cd:17:3e:a9:55:53"
						fingerprint4 = "2a:43:73:52:6c:21:b5:5f:cc:cd:73:21:2b:05:50:de"
					)

					itWorks := func() {
						It("adds given pub keys to the authorized_keys file for the container", func() {
							err := prov.AddPublicKeys(id, []string{pubkey3, pubkey4, pubkey1}) // dupe key should be ignored
							Expect(err).To(BeNil())
							pubkeys, err := prov.ListPublicKeys(id)
							Expect(err).To(BeNil())
							Expect(pubkeys).To(Equal(map[string]string{
								fingerprint1: pubkey1,
								fingerprint2: pubkey2,
								fingerprint3: pubkey3,
								fingerprint4: pubkey4,
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

				Context("with an invalid public key in the mix", func() {
					pubkey4 := "ssh-rsa S4!#AFLJKN*F@NF12f"

					itDoesNotWork := func() {
						It("does not add given pub keys to the authorized_keys file for the container", func() {
							err := prov.AddPublicKeys(id, []string{pubkey3, pubkey4, pubkey1})
							Expect(err).To(MatchError(util.ErrInvalidPublicKey))
							pubkeys, err := prov.ListPublicKeys(id)
							Expect(err).To(BeNil())
							Expect(pubkeys).To(Equal(map[string]string{
								fingerprint1: pubkey1,
								fingerprint2: pubkey2,
							}))
						})
					}

					Context("when the container is running", func() {
						itDoesNotWork()
					})

					Context("when the container is stopped", func() {
						BeforeEach(func() {
							err := exec.Command("docker", "stop", id).Run()
							Expect(err).To(BeNil())
						})

						itDoesNotWork()
					})
				})
			})

			Describe(".RemovePublicKeys()", func() {
				Context("when a pub key with a given fingerprint is found", func() {
					itWorks := func() {
						It("removes the matching pub key from the authorized_keys file", func() {
							err := prov.RemovePublicKey(id, fingerprint1)
							Expect(err).To(BeNil())
							pubkeys, err := prov.ListPublicKeys(id)
							Expect(err).To(BeNil())
							Expect(pubkeys).To(Equal(map[string]string{
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

				Context("when a pub key with a given fingerprint is not found", func() {
					itDoesNotWork := func() {
						It("does nothing", func() {
							err := prov.RemovePublicKey(id, "11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11")
							Expect(err).To(BeNil())
							pubkeys, err := prov.ListPublicKeys(id)
							Expect(err).To(BeNil())
							Expect(pubkeys).To(Equal(map[string]string{
								fingerprint1: pubkey1,
								fingerprint2: pubkey2,
							}))
						})
					}

					Context("when the container is running", func() {
						itDoesNotWork()
					})

					Context("when the container is stopped", func() {
						BeforeEach(func() {
							err := exec.Command("docker", "stop", id).Run()
							Expect(err).To(BeNil())
						})

						itDoesNotWork()
					})
				})
			})
		})
	})
})
