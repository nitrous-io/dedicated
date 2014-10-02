package integration

import (
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	"github.com/nitrous-io/dedicated/freighter/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("InspectImage", func() {
	It("returns image info", func() {
		logger := &util.Logger{Level: util.LogLevelQuiet}
		data, _ := test.TempDir()
		prov, err := docker.NewDockerProvider(logger, data)
		Expect(err).To(BeNil(), "failed to initialize docker provider")

		info, err := prov.InspectImage("scratch")
		Expect(err).To(BeNil(), "failed to get scratch image data")
		Expect(info.Name).To(Equal("scratch"), "invalid name")
		Expect(info.Tag).To(Equal("latest"), "invalid tag")
	})

	It("returns error when fail to find matching image", func() {
		logger := &util.Logger{Level: util.LogLevelQuiet}
		data, _ := test.TempDir()
		prov, err := docker.NewDockerProvider(logger, data)
		Expect(err).To(BeNil(), "failed to initialize docker provider")

		_, err = prov.InspectImage("missingimage")
		Expect(err).ToNot(BeNil(), "failed to return error for missing image")
	})
})
