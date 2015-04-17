package docker_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/host/docker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	dockerclient "github.com/fsouza/go-dockerclient"
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
		server, err = dockertest.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
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
					map[string]string{
						"endpoint":            server.URL(),
						"freighterd_location": "/bin/true",
					},
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
				Expect(info.Name).To(Equal("test-123"))
				Expect(len(info.ID)).To(BeNumerically(">", 0))
				Expect(info.IPAddress).To(Equal("127.0.0.1"))
			})
		})

		Describe("Start", func() {
			var h host.Host
			var err error
			var info *host.Info

			Context("With docker location", func() {
				BeforeEach(func() {
					h, err = docker.NewHost(
						map[string]string{
							"endpoint":            server.URL(),
							"freighterd_location": "/bin/true",
							"docker_location":     "/bin/true",
						},
						`{"name":"test-123","image":"ubuntu","public_keys":["ssh-rsa 3456"]}`,
					)
					Expect(err).To(BeNil())
					info, err = h.Create(ioutil.Discard)
					Expect(err).To(BeNil())
				})

				It("should start the container", func() {
					server.CustomHandler("/containers/"+info.ID+"/start", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body dockerclient.HostConfig
						defer r.Body.Close()
						err := json.NewDecoder(r.Body).Decode(&body)
						Expect(err).To(BeNil())
						Expect(body.Binds).To(HaveLen(2))
						Expect(body.Binds[0]).To(Equal("/bin/true:/freighterd"))
						Expect(body.Binds[1]).To(Equal("/bin/true:/usr/bin/docker"))
					}))
					info, err = h.Start()
					Expect(err).To(BeNil())
				})
			})

			Context("Without docker location", func() {
				BeforeEach(func() {
					h, err = docker.NewHost(
						map[string]string{
							"endpoint":            server.URL(),
							"freighterd_location": "/bin/true",
						},
						`{"name":"test-123","image":"ubuntu","public_keys":["ssh-rsa 3456"]}`,
					)
					Expect(err).To(BeNil())
					info, err = h.Create(ioutil.Discard)
					Expect(err).To(BeNil())
				})

				It("should start the container", func() {
					server.CustomHandler("/containers/"+info.ID+"/start", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body dockerclient.HostConfig
						defer r.Body.Close()
						err := json.NewDecoder(r.Body).Decode(&body)
						Expect(err).To(BeNil())
						Expect(body.Binds).To(HaveLen(1))
						Expect(body.Binds[0]).To(Equal("/bin/true:/freighterd"))
					}))
					info, err = h.Start()
					Expect(err).To(BeNil())
				})
			})
		})
	})
})
