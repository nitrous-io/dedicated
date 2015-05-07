package queue_test

import (
	"testing"

	"gopkg.in/redis.v2"

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
		if qp != nil {
			qp.Delete()
		}
		redisClient.Close()
	})

	Describe("QueuePublisher", func() {
		Describe("Publish()", func() {
			BeforeEach(func() {
				qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				err := qp.ResetInput()
				Expect(err).To(BeNil())
			})

			It("publishes a message to queue", func() {
				Expect(qp.GetInputLength()).To(Equal(int64(0)))
				err := qp.Publish("create", "image.created", "host", "1", []byte{})
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
			qp.Publish("create", "image.created", "host", "1", []byte{})
		})

		AfterEach(func() {
			if qp != nil {
				qp.Delete()
			}
		})

		Describe("Consume", func() {
			It("consumes a message from queue", func() {
				m, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(m.Action).To(Equal("create"))
				Expect(m.Event).To(Equal("image.created"))
				Expect(m.ResourceName).To(Equal("host"))
				Expect(m.ResourceID).To(Equal("1"))
				Expect(m.Payload).To(Equal([]byte{}))
			})
		})
	})
})
