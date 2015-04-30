package queue_test

import (
	"encoding/json"
	"testing"

	"gopkg.in/redis.v2"

	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/queue")
}

var _ = Describe("pkg/queue", func() {
	var qp *queue.QueuePublisher
	var redisClient *redis.Client

	BeforeEach(func() {
		var err error
		redisClient, err = test.ConnectToRedisClient()
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		redisClient.Close()
	})

	Describe("QueuePublisher", func() {
		Describe("Publish()", func() {
			BeforeEach(func() {
				qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				err := qp.ResetInput()
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				qp.Close()
			})

			It("publishes a message to queue", func() {
				Expect(qp.GetInputLength()).To(Equal(int64(0)))
				err := qp.Publish("create", "image.created", "host", "1", "")
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})
		})
	})

	Describe("QueueConsumer", func() {
		var qc *queue.QueueConsumer
		var err error

		BeforeEach(func() {
			qc, err = queue.CreateConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			Expect(err).To(BeNil())
			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err := qp.ResetInput()
			Expect(err).To(BeNil())
			qp.Publish("create", "image.created", "host", "1", "{}")
		})

		AfterEach(func() {
			qp.Close()
		})

		Describe("Consume", func() {
			It("consumes a message from queue", func() {
				m, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(m.Action).To(Equal("create"))
				Expect(m.Event).To(Equal("image.created"))
				Expect(m.ResourceName).To(Equal("host"))
				Expect(m.ResourceID).To(Equal("1"))
				Expect(m.Payload).To(Equal("{}"))
			})
		})
	})

	Describe("HostQueuePublisher", func() {
		var hqp *queue.HostQueuePublisher
		var err error

		BeforeEach(func() {
			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err = qp.ResetInput()
			Expect(err).To(BeNil())
			hqp = qp.NewHostQueuePublisher(123, "test-123", 124)
		})

		AfterEach(func() {
			qp.Close()
		})

		Describe("Publish", func() {
			It("publishes a message", func() {
				Expect(qp.GetInputLength()).To(Equal(int64(0)))
				err := hqp.Publish("create", "image.created", "host", "1")
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})

			It("publishes a message with host info", func() {
				hqp.Provider = "docker"
				hqp.ID = "Host-1234"
				hqp.Region = "Somewhere"
				hqp.Name = "Host-4567"
				hqp.IPAddress = "1.2.3.4"
				hqp.FreighterdPort = 61234
				hqp.State = state.Running

				err := hqp.Publish("create", "image.created", "host", "1")
				Expect(err).To(BeNil())

				qc, err := queue.CreateConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				Expect(err).To(BeNil())

				m, err := qc.Consume()
				Expect(err).To(BeNil())

				var hostInfo queue.HostQueuePublisher
				err = json.Unmarshal([]byte(m.Payload), &hostInfo)
				Expect(err).To(BeNil())
				Expect(hostInfo.HostID).To(Equal(int64(123)))
				Expect(hostInfo.Slug).To(Equal("test-123"))
				Expect(hostInfo.UserID).To(Equal(int64(124)))
				Expect(hostInfo.Provider).To(Equal("docker"))
				Expect(hostInfo.ID).To(Equal("Host-1234"))
				Expect(hostInfo.Region).To(Equal("Somewhere"))
				Expect(hostInfo.Name).To(Equal("Host-4567"))
				Expect(hostInfo.IPAddress).To(Equal("1.2.3.4"))
				Expect(hostInfo.FreighterdPort).To(Equal(uint16(61234)))
				Expect(hostInfo.State).To(Equal(state.Running))
			})
		})
	})
})
