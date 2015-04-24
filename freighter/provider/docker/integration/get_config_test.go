package integration

import (
	"io/ioutil"

	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	"github.com/nitrous-io/dedicated/freighter/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetConfig", func() {
	It("retrieves port bindings for running and stopped containers", func() {
		datadir, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil())

		prov, err := docker.NewDockerProvider(&util.Logger{Level: util.LogLevelQuiet},
			datadir)
		Expect(err).To(BeNil())

		name := test.RandomName("freightertest")
		configJSON := `{
			"PortBindings": ["private:1234:2345/udp", "public:2345:3456/tcp", "public::3456", "private::8080"]
		}`
		info, err := prov.CreateStart(name, "busybox:latest", "secret", configJSON)
		defer test.CleanUpContainer(name)
		Expect(err).To(BeNil())

		config, err := prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		Expect(config.PortBindings).To(ContainElement("private:1234:2345/udp"))
		Expect(config.PortBindings).To(ContainElement("public:2345:3456/tcp"))
		Expect(config.PortBindings).To(ContainElement(MatchRegexp(`public:\d+:3456`)))
		Expect(config.PortBindings).To(ContainElement(MatchRegexp(`private:\d+:8080`)))

		Expect(config.ConfigPortBindings).To(ConsistOf([]string{
			"private:1234:2345/udp", "public:2345:3456/tcp", "public::3456/tcp", "private::8080/tcp",
		}))

		info, err = prov.Stop(name)
		Expect(err).To(BeNil())

		config, err = prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		Expect(config.PortBindings).To(ConsistOf([]string{
			"private:1234:2345/udp", "public:2345:3456/tcp", "public::3456/tcp", "private::8080/tcp",
		}))

		Expect(config.ConfigPortBindings).To(ConsistOf([]string{
			"private:1234:2345/udp", "public:2345:3456/tcp", "public::3456/tcp", "private::8080/tcp",
		}))
	})
})
