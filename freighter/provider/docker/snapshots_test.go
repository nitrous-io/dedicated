package docker_test

import (
	"os"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	. "github.com/nitrous-io/dedicated/freighter/test"
	"github.com/nitrous-io/dedicated/freighter/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker snapshots", func() {
	var (
		datadir   string
		container string
		prov      *docker.DockerProvider
		info      *provider.ContainerInfo
		secret    = "s3cr3t"
	)

	BeforeEach(func() {
		var err error

		datadir, err = TempDir()
		Expect(err).To(BeNil())
		logger := &util.Logger{Level: util.LogLevelQuiet}
		prov, err = docker.NewDockerProvider(logger, datadir)
		Expect(err).To(BeNil())

		PullImage(NitrousImage)
		container = RandomName("freightertest")
		info, err = prov.CreateStart(container, NitrousImage, secret, "")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		prov.Terminate(info.ID, true)
		os.RemoveAll(datadir)
	})

	Describe(".CreateSnapshot()", func() {
		It("creates a snapshot", func() {
			snap, err := prov.CreateSnapshot()
			Expect(err).To(BeNil())
			Expect(snap.Size).ToNot(Equal(0))
			Expect(snap.Url).ToNot(Equal(""))
		})
	})
})
