package docker_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/host/docker"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/redis.v2"

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
	})

	Describe("CreateWithQueue", func() {
		var h host.Host
		var info *host.Info
		var qp queue.Publisher
		var qc *queue.QueueConsumer
		var queueInfo *host.QueueInfo

		var redisClient *redis.Client
		BeforeEach(func() {
			var err error
			redisClient, err = test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			Expect(err).To(BeNil())
			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "host")
			Expect(err).To(BeNil())
			qc, err = qp.CreateConsumer()
			Expect(err).To(BeNil())

			h, err = docker.NewHost(
				map[string]string{
					"uuid":                "uuid-111",
					"endpoint":            server.URL(),
					"freighterd_location": "/bin/true",
				},
				`{"name":"test-123","image":"ubuntu","public_keys":["ssh-rsa 3456"]}`,
			)
			Expect(err).To(BeNil())
			queueInfo = &host.QueueInfo{
				ID:     123,
				Slug:   "go-124",
				UserID: 125,
			}
		})

		AfterEach(func() {
			if qp != nil {
				qp.Delete()
			}
			if redisClient != nil {
				redisClient.Close()
			}
		})

		It("returns host info", func() {
			info, err = h.CreateWithQueue(qp, queueInfo)
			Expect(err).To(BeNil())

			Expect(info.Provider).To(Equal("docker"))
			Expect(info.Name).To(Equal("test-123"))
			Expect(len(info.ID)).To(BeNumerically(">", 0))
			Expect(info.IPAddress).To(Equal("127.0.0.1"))
		})

		It("sends messages to the host queue", func() {
			info, err = h.CreateWithQueue(qp, queueInfo)
			Expect(err).To(BeNil())

			Expect(qp.GetInputLength()).To(Equal(int64(3)))
			events := [3]event.Event{
				event.HostCreating,
				event.HostCreating,
				event.HostCreated,
			}

			var queueInfos [3]*host.QueueInfo
			for index, event := range events {
				m, err := qc.Consume()
				m.Ack(10 * time.Millisecond)
				Expect(err).To(BeNil())
				Expect(m.UUID).To(Equal("uuid-111"))
				Expect(m.Event).To(Equal(string(event)))
				Expect(m.Action).To(Equal("create"))
				Expect(m.ResourceName).To(Equal("host"))

				var queueInfo host.QueueInfo
				err = json.Unmarshal(m.Payload, &queueInfo)
				Expect(err).To(BeNil())
				Expect(queueInfo.ID).To(Equal(int64(123)))
				Expect(queueInfo.Slug).To(Equal("go-124"))
				Expect(queueInfo.UserID).To(Equal(int64(125)))
				queueInfos[index] = &queueInfo
			}
			Expect(queueInfos[0].Message).ToNot(BeZero())
			Expect(queueInfos[0].Host).To(BeZero())

			Expect(queueInfos[1].Message).ToNot(BeZero())
			Expect(queueInfos[1].Host).To(BeZero())

			Expect(queueInfos[2].Message).To(BeZero())
			Expect(queueInfos[2].Host).To(Equal(info))
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
