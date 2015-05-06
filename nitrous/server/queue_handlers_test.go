package server_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/pkg/queue"
	redis_test "github.com/nitrous-io/dedicated/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server")
}

var _ = Describe("HostQueueHandler", func() {
	var (
		hqp     *queue.HostQueuePublisher
		qp      *queue.QueuePublisher
		qc      *queue.QueueConsumer
		db      *db.Database
		handler func(qc *queue.QueueConsumer, m *queue.Message) error
		h       *host.Host
		u       *user.User
		svr     *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		_, err = redis_test.ConnectToRedisClient()
		Expect(err).To(BeNil())

		qp = queue.NewQueue(redis_test.RedisHost, redis_test.RedisPort, redis_test.RedisTestDb, "host")
		Expect(err).To(BeNil())
		qc, err = qp.CreateConsumer()
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		hs, err := test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:  "host-123",
				Size:  sql.NullString{"micro", true},
				State: host.StateBorn,
			},
			U: u,
		})
		Expect(err).To(BeNil())
		h = hs.H

		hqp = qp.NewHostQueuePublisher(h.ID, h.Slug, u.ID)
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
		handler = svr.HostQueueHandler()
	})

	AfterEach(func() {
		qp.Close()
	})

	Context("an image is created on creating host", func() {
		var message *queue.Message

		BeforeEach(func() {
			hostinfo, err := json.Marshal(&hqp)
			Expect(err).To(BeNil())
			message = &queue.Message{
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "host-1234",
				Payload:      hostinfo,
			}
		})

		It("publishes events", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			err := handler(qc, message)
			Expect(err).To(BeNil())

			evtCh := make(chan *controllers.Event)
			go func() {
				var evt *controllers.Event
				for msg := range ch { // Ignore events we don't care about.
					evt = msg.(*controllers.Event)
					if evt.Name == controllers.EventImageCreated {
						evtCh <- evt
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timeout while getting image created event")
			case evt := <-evtCh:
				Expect(evt.Name).To(Equal(controllers.EventImageCreated))
				evtData, ok := evt.Data.(map[string]*serializers.Host)
				Expect(ok).To(BeTrue())
				Expect(evtData["host"]).To(Equal(serializers.SerializeHost(h)))
			}
		})
	})

	Context("a host is created", func() {
		var message *queue.Message

		BeforeEach(func() {
			hqp.Provider = "docker"
			hqp.ID = "host-1234"
			hqp.Region = "somewhere"
			hqp.Name = "hostname-1234"
			hqp.IPAddress = "1.2.3.4"
			hqp.FreighterdPort = 62345
			hqp.State = state.Running

			hostinfo, err := json.Marshal(&hqp)
			Expect(err).To(BeNil())

			message = &queue.Message{
				Action:       "create",
				Event:        "host.created",
				ResourceName: "host",
				ResourceID:   "host-1234",
				Payload:      hostinfo,
			}
		})

		It("updates the host", func() {
			err := handler(qc, message)
			Expect(err).To(BeNil())

			updatedHost, err := host.Find(db, u, h.Slug)
			Expect(err).To(BeNil())

			Expect(updatedHost.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(updatedHost.ProviderIdentifier.String).To(Equal("host-1234"))
			Expect(updatedHost.RegionIdentifier.String).To(Equal("somewhere"))
			Expect(updatedHost.FreighterdPort.Int64).To(Equal(int64(62345)))
			Expect(updatedHost.State).To(Equal(host.StateRunning))
			Expect(updatedHost.HourlyPrice).To(Equal(int64(745)))
		})

		It("does not update when the host is running", func() {
			h.State = host.StateRunning
			h.IPAddress = sql.NullString{"4.5.6.7", true}
			h.ProviderIdentifier = sql.NullString{"host-3456", true}
			h.RegionIdentifier = sql.NullString{"here", true}
			h.FreighterdPort = sql.NullInt64{12345, true}
			h.HourlyPrice = 1000000

			err := h.Update(db)
			Expect(err).To(BeNil())

			err = handler(qc, message)
			Expect(err).To(BeNil())

			err = handler(qc, message)
			Expect(err).To(BeNil())

			updatedHost, err := host.Find(db, u, h.Slug)
			Expect(err).To(BeNil())

			Expect(updatedHost.IPAddress.String).To(Equal("4.5.6.7"))
			Expect(updatedHost.ProviderIdentifier.String).To(Equal("host-3456"))
			Expect(updatedHost.RegionIdentifier.String).To(Equal("here"))
			Expect(updatedHost.FreighterdPort.Int64).To(Equal(int64(12345)))
			Expect(updatedHost.State).To(Equal(host.StateRunning))
			Expect(updatedHost.HourlyPrice).To(Equal(int64(1000000)))
		})
	})
})
