package docker_test

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/host/docker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	dockertest "github.com/fsouza/go-dockerclient/testing"
)

func TestDocker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/docker")
}

var _ = Describe("host/docker", func() {
	var server *dockertest.DockerServer
	var requests = []*http.Request{}
	var err error

	BeforeEach(func() {
		server, err = dockertest.NewServer("127.0.0.1:0", func(r *http.Request) {
			requests = append(requests, r)
		})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		requests = []*http.Request{}
		server.Stop()
	})

	Describe("Create", func() {
		Context("Successful Creation", func() {
			var h host.Host
			var err error
			var info *host.Info

			BeforeEach(func() {
				h, err = docker.NewHost(
					map[string]string{"endpoint": server.URL()},
					`{"name":"test-123","image":"ubuntu","public_keys":["ssh-rsa 3456"]}`,
				)
				Expect(err).To(BeNil())
				info, err = h.Create(ioutil.Discard)
			})

			It("should successfully create the host", func() {
				Expect(err).To(BeNil())
			})

			It("should create a new image", func() {
				r := requests[0]
				Expect(r.URL.Path).To(Equal("/build"))
				Expect(r.URL.Query().Get("t")).To(Equal("registry.nitrous.io/test-123"))
			})

			It("should create a new container", func() {
				r := requests[1]
				Expect(r.URL.Path).To(Equal("/containers/create"))
				Expect(r.URL.Query().Get("name")).To(Equal("test-123"))
			})

			It("should start the container", func() {
				r := requests[2]
				Expect(r.URL.Path).To(Equal("/containers/" + info.ID + "/start"))
			})

			It("should get the container info and return its IP address", func() {
				r := requests[3]
				Expect(r.URL.Path).To(Equal("/containers/" + info.ID + "/json"))
				Expect(info.Provider).To(Equal("docker"))
				Expect(len(info.ID)).To(BeNumerically(">", 0))
				Expect(info.IPAddress).To(Equal("127.0.0.1"))
			})
		})
	})
})
