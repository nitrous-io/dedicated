package queue_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

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
		output, err := exec.Command("service", "redis_6379", "start").CombinedOutput()
		Expect(err).To(BeNil(), "failed to start redis")
		if !strings.Contains(string(output), "already running") {
			time.Sleep(100 * time.Millisecond)
		}

		redisClient, err = test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
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
				msg := &queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				}
				err := qp.Publish(msg)
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})
		})

		Describe("PublishRetry()", func() {
			var msg *queue.Message
			BeforeEach(func() {
				qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				err := qp.ResetInput()
				Expect(err).To(BeNil())
				msg = &queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				}
			})

			It("ignore ratelimit error from redis library", func() {
				err := test.RunWhileRestartingRedis(func() {
					err := qp.PublishRetry(msg, -1, 5*time.Millisecond)
					Expect(err).To(BeNil())
				})
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})

			It("retries when failed to send msg to redis", func() {
				err := test.RunWhileRestartingRedis(func() {
					err := qp.PublishRetry(msg, 11, 50*time.Millisecond)
					Expect(err).To(BeNil())
				})
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})

			It("retries until it successes when retry param is -1", func() {
				err := test.RunWhileRestartingRedis(func() {
					err := qp.PublishRetry(msg, -1, 50*time.Millisecond)
					Expect(err).To(BeNil())
				})
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
			msg := &queue.Message{
				UUID:         "1234",
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "1",
			}
			err = qp.Publish(msg)
			Expect(err).To(BeNil())
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
				Expect(m.UUID).To(Equal("1234"))
				Expect(m.Action).To(Equal("create"))
				Expect(m.Event).To(Equal("image.created"))
				Expect(m.ResourceName).To(Equal("host"))
				Expect(m.ResourceID).To(Equal("1"))
				Expect(m.Payload).To(BeEmpty())
			})
		})
	})

	Describe("Message", func() {
		Describe("Ack", func() {
			var qc *queue.QueueConsumer
			var err error
			var msg *queue.Message
			BeforeEach(func() {
				qc, err = queue.CreateConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				Expect(err).To(BeNil())
				qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				err := qp.ResetInput()
				Expect(err).To(BeNil())
				err = qp.Publish(&queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				})
				Expect(err).To(BeNil())
				err = qp.Publish(&queue.Message{
					UUID:         "4567",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "2",
				})
				Expect(err).To(BeNil())
				msg, err = qc.Consume()
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				if qp != nil {
					qp.Delete()
				}
			})

			It("sends ask for the message to consume next message successfully", func() {
				msg.Ack(50 * time.Millisecond)
				nextMsg, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(nextMsg.UUID).To(Equal("4567"))
			})

			It("retries when it failes to send a ACK", func() {
				err := test.RunWhileRestartingRedis(func() {
					msg.Ack(50 * time.Millisecond)
				})
				Expect(err).To(BeNil())
				nextMsg, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(nextMsg.UUID).To(Equal("4567"))
			})
		})
	})
})
