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

		prov, err = docker.NewDockerProvider(store)
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
			name   string
			pubkey []byte
			info   *provider.ContainerInfo
		)

		BeforeEach(func() {
			name = RandomName("test")
			var err error
			pubkey, err = ioutil.ReadFile(path.Join(os.Getenv("HOME"), ".ssh", "id_rsa.pub"))
			Expect(err).To(BeNil())
			info, err = prov.Create(name, "ubuntu-upstart:trusty", []string{string(pubkey)})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			exec.Command("docker", "stop", name).Run()
			exec.Command("docker", "rm", name).Run()
		})

		It("creates a container successfully", func() {
			id, err := exec.Command("docker", "inspect", "-f", "{{.ID}}", name).CombinedOutput()
			Expect(err).To(BeNil())
			Expect(info.Id).To(Equal(strings.TrimSpace(string(id))))
			Expect(info.Name).To(Equal(name))
			Expect(info.Port).To(Equal(uint16(65001)))
			Expect(info.State).To(Equal(provider.ContainerStateRunning))
			time.Sleep(1 * time.Second)
			err = exec.Command("ssh", "action@localhost", "-p", "65001", "-o", "StrictHostKeyChecking=no", "echo 1").Run()
			Expect(err).To(BeNil())
		})

		Describe(".GetInfo()", func() {
			It("returns container info", func() {
				info, err := prov.GetInfo(name)
				Expect(err).To(BeNil())
				id, err := exec.Command("docker", "inspect", "-f", "{{.ID}}", name).CombinedOutput()
				Expect(err).To(BeNil())
				Expect(info.Id).To(Equal(strings.TrimSpace(string(id))))
				Expect(info.Name).To(Equal(name))
				Expect(info.Port).To(Equal(uint16(65001)))
				Expect(info.State).To(Equal(provider.ContainerStateRunning))
			})
		})
	})
})
