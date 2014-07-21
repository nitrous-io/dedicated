package docker_test

import (
	"encoding/json"

	"github.com/nitrous-io/freighter/provider/docker"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker config", func() {
	Describe("Docker config marshaling", func() {
		var p *docker.DockerProvider

		BeforeEach(func() {
			p = &docker.DockerProvider{}
		})

		It("handles all options", func() {
			j, err := json.Marshal(&docker.ContainerConfig{
				Env:          []string{"LANG=C"},
				Volumes:      []string{"/music"},
				VolumesFrom:  []string{"somecontainer"},
				ExposedPorts: []string{"3000/tcp"},
				PortBindings: map[string]string{
					"3000/tcp": ":1234",
				},
			})
			Expect(err).To(BeNil())
			Expect(string(j)).To(Equal(`{"Config":{"Env":["LANG=C"],"ExposedPorts":{"3000/tcp":{}},"Volumes":{"/music":{}}},"HostConfig":{"PortBindings":{"3000/tcp":[{"HostIp":"127.0.0.1","HostPort":"1234"}]},"VolumesFrom":["somecontainer"]}}`))
		})

		It("returns error on invalid port binding", func() {
			_, err := json.Marshal(&docker.ContainerConfig{
				PortBindings: map[string]string{
					"3000/tcp": "123:456:789",
				},
			})
			Expect(err.Error()).To(ContainSubstring("parsing port binding"))
		})

		It("handles missing Config", func() {
			j, err := json.Marshal(&docker.ContainerConfig{
				VolumesFrom: []string{"somecontainer"},
			})
			Expect(err).To(BeNil())
			Expect(string(j)).To(Equal(`{"Config":{},"HostConfig":{"VolumesFrom":["somecontainer"]}}`))
		})

		It("handles missing HostConfig", func() {
			j, err := json.Marshal(&docker.ContainerConfig{
				Env: []string{"LANG=C"},
			})
			Expect(err).To(BeNil())
			Expect(string(j)).To(Equal(`{"Config":{"Env":["LANG=C"]},"HostConfig":{}}`))
		})
	})
})
