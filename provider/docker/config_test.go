package docker_test

import (
	"path/filepath"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/freighter/provider/docker"
	. "github.com/nitrous-io/freighter/test"
	"github.com/nitrous-io/freighter/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker config", func() {
	var (
		prov      *docker.DockerProvider
		namespace = "freightertest"
		datadir   string
	)

	BeforeEach(func() {
		var err error
		datadir, err = TempDir()
		Expect(err).To(BeNil())
		logger := &util.Logger{Level: util.LogLevelQuiet}
		prov, err = docker.NewDockerProvider(logger, datadir, namespace)
		Expect(err).To(BeNil())
	})

	Describe("CreateConfigs", func() {
		var (
			cfg     *dockerclient.Config
			hostCfg *dockerclient.HostConfig
		)

		Context("defaults (no config passed)", func() {
			BeforeEach(func() {
				var err error
				cfg, hostCfg, err = prov.CreateConfigs("php-box", "nitrous-io/php", nil)
				Expect(err).To(BeNil())
			})

			It("has hostname and image", func() {
				Expect(cfg.Hostname).To(Equal("php-box"))
				Expect(cfg.Image).To(Equal("nitrous-io/php"))
			})

			It("has ssh volume and bind", func() {
				Expect(cfg.Volumes).To(HaveLen(1))
				_, ok := cfg.Volumes["/etc/ssh/freighter"]
				Expect(ok).To(BeTrue())
				Expect(hostCfg.Binds).To(HaveLen(1))
				Expect(hostCfg.Binds[0]).To(Equal(filepath.Join(
					datadir, "containers", "php-box", "ssh:/etc/ssh/freighter:ro")))
			})

			It("has no env", func() {
				Expect(cfg.Env).To(BeEmpty())
			})

			It("exposes 22/tcp", func() {
				_, ok := cfg.ExposedPorts["22/tcp"]
				Expect(ok).To(BeTrue())
			})

			It("publishes all ports", func() {
				Expect(hostCfg.PublishAllPorts).To(Equal(true))
			})
		})

		Context("with PortBindings config", func() {
			BeforeEach(func() {
				var err error
				cfg, hostCfg, err = prov.CreateConfigs("php-box", "nitrous-io/php",
					&docker.ContainerConfig{
						PortBindings: []string{"0.0.0.0::4000", "3000:8080", "127.0.0.1:5000:5001"},
					})
				Expect(err).To(BeNil())
			})

			It("parses portspecs", func() {
				Expect(cfg.ExposedPorts).To(HaveLen(4))
				for _, p := range []dockerclient.Port{"4000", "8080", "5001", "22/tcp"} {
					Expect(cfg.ExposedPorts[p]).To(Equal(struct{}{}))
				}
				Expect(hostCfg.PortBindings).To(HaveLen(3))
				portBinds := []struct {
					conPort  string
					hostIP   string
					hostPort string
				}{
					{"4000", "0.0.0.0", ""},
					{"8080", "127.0.0.1", "3000"},
					{"5001", "127.0.0.1", "5000"},
				}
				for _, pb := range portBinds {
					pbs := hostCfg.PortBindings[dockerclient.Port(pb.conPort)]
					Expect(pbs).To(HaveLen(1))
					Expect(pbs[0]).To(Equal(
						dockerclient.PortBinding{
							HostIp: pb.hostIP, HostPort: pb.hostPort}))
				}
			})
		})
	})

	Describe("StartConfig", func() {
		var (
			container *dockerclient.Container
			hostCfg   *dockerclient.HostConfig
		)

		BeforeEach(func() {
			container = &dockerclient.Container{
				HostConfig: &dockerclient.HostConfig{
					PortBindings: map[dockerclient.Port][]dockerclient.PortBinding{
						dockerclient.Port("22"): []dockerclient.PortBinding{},
					},
				},
			}
		})

		Context("defaults (no config passed)", func() {
			BeforeEach(func() {
				var err error
				hostCfg, err = prov.StartConfig(container, nil)
				Expect(err).To(BeNil())
			})

			It("preserves existing port bindings", func() {
				Expect(hostCfg.PortBindings).To(HaveLen(1))
				Expect(hostCfg.PortBindings[dockerclient.Port("22")]).To(Equal([]dockerclient.PortBinding{}))
			})
		})
	})
})
