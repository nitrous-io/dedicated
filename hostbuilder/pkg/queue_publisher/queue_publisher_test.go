package queue_publisher_test

import (
	"fmt"
	"testing"

	"gopkg.in/redis.v2"

	"github.com/nitrous-io/dedicated/hostbuilder/pkg/queue_publisher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const redisTestDb int64 = 12
const redisHost string = "localhost"
const redisPort string = "6379"

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/queue_publisher")
}

var _ = Describe("pkg/queue_publisher", func() {
	var qp *queue_publisher.QueuePublisher
	var redisClient *redis.Client

	BeforeEach(func() {
		redisClient = redis.NewTCPClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
			Password: "",
			DB:       12,
		})
		// Make sure redis connection is established
		Expect(redisClient.Ping().Err()).To(BeNil())
	})

	AfterEach(func() {
		redisClient.FlushDb()
	})

	Describe("Publish()", func() {
		BeforeEach(func() {
			qp = queue_publisher.NewQueue(redisHost, redisPort, redisTestDb, "test")
			err := qp.Reset()
			fmt.Println(err)
			Expect(err).To(BeNil())
		})

		It("publishes a message to queue", func() {
			Expect(qp.Length()).To(Equal(int64(0)))
			err := qp.Publish("create", "host", "1", "")
			Expect(err).To(BeNil())
			Expect(qp.Length()).To(Equal(int64(1)))
		})
	})
})
