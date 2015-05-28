package server_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/event"
	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/pkg/queue"
	redistest "github.com/nitrous-io/dedicated/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/redis.v2"
)

var _ = Describe("HostQueueHandler", func() {
	var (
		redisClient *redis.Client
		qp          *queue.QueuePublisher
		qc          *queue.QueueConsumer
		db          *db.Database
		handler     func(m *queue.Message) error
		h           *host.Host
		u           *user.User
		svr         *server.Server
		queueInfo   *hh.QueueInfo
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB)
		Expect(err).To(BeNil())
		qp = queue.NewQueue(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, "host")
		Expect(err).To(BeNil())
		qc, err = queue.CreateConsumer(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, "host")
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

		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
		handler = svr.HostQueueHandler()
	})

	AfterEach(func() {
		if qp != nil {
			qp.Queue.Delete()
		}
		if redisClient != nil {
			redisClient.Close()
		}
	})

	Context("host is being created", func() {
		var message *queue.Message

		BeforeEach(func() {
			queueInfo = &hh.QueueInfo{
				ID:      h.ID,
				Slug:    h.Slug,
				UserID:  u.ID,
				Message: "creating a host",
			}
			payload, err := json.Marshal(queueInfo)
			Expect(err).To(BeNil())
			message = &queue.Message{
				UUID:         "uuid-1234",
				Action:       "create",
				Event:        string(event.HostCreating),
				ResourceName: "host",
				ResourceID:   "host-1234",
				Payload:      payload,
			}
		})

		It("publishes the event", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			err := handler(message)
			Expect(err).To(BeNil())

			evtCh := make(chan *controllers.Event)
			go func() {
				var evt *controllers.Event
				for msg := range ch { // Ignore events we don't care about.
					evt = msg.(*controllers.Event)
					if evt.Name == controllers.EventName(event.HostCreating) {
						evtCh <- evt
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timeout while getting image created event")
			case evt := <-evtCh:
				Expect(evt.Name).To(Equal(controllers.EventName(event.HostCreating)))
				evtData, ok := evt.Data.(*controllers.ProgressEvent)
				Expect(ok).To(BeTrue())
				Expect(evtData.Progress).To(Equal(fmt.Sprintf(`{"stream":"%s"}`, queueInfo.Message)))
			}
		})
	})

	Context("a host is created", func() {
		var message *queue.Message

		BeforeEach(func() {
			queueInfo = &hh.QueueInfo{
				ID:     h.ID,
				Slug:   h.Slug,
				UserID: u.ID,
				Host: &hh.Info{
					Provider:       "docker",
					ID:             "host-1234",
					Region:         "somewhere",
					Name:           "hostname-1234",
					IPAddress:      "1.2.3.4",
					FreighterdPort: 62345,
					State:          state.Running,
				},
			}
			payload, err := json.Marshal(queueInfo)
			Expect(err).To(BeNil())
			message = &queue.Message{
				UUID:         "uuid-1234",
				Action:       "create",
				Event:        string(event.HostCreated),
				ResourceName: "host",
				ResourceID:   "host-1234",
				Payload:      payload,
			}
		})

		It("publishes the event", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			err := handler(message)
			Expect(err).To(BeNil())

			evtCh := make(chan *controllers.Event)
			go func() {
				var evt *controllers.Event
				for msg := range ch { // Ignore events we don't care about.
					evt = msg.(*controllers.Event)
					if evt.Name == controllers.EventName(event.HostCreated) {
						evtCh <- evt
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timeout while getting image created event")
			case evt := <-evtCh:
				Expect(evt.Name).To(Equal(controllers.EventName(event.HostCreated)))
				evtData, ok := evt.Data.(map[string]*serializers.Host)
				Expect(ok).To(BeTrue())
				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())
				Expect(evtData["host"]).To(Equal(serializers.SerializeHost(updatedHost)))
			}
		})

		It("updates the host", func() {
			err := handler(message)
			Expect(err).To(BeNil())

			updatedHost, err := host.Find(db, u, h.Slug)
			Expect(err).To(BeNil())

			Expect(updatedHost.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(updatedHost.ProviderIdentifier.String).To(Equal("host-1234"))
			Expect(updatedHost.RegionIdentifier.String).To(Equal("somewhere"))
			Expect(updatedHost.FreighterdPort.Int64).To(Equal(int64(62345)))
			Expect(updatedHost.State).To(Equal(host.StateRunning))
			Expect(updatedHost.HourlyPrice).To(Equal(int64(745)))
			Expect(updatedHost.Free).To(BeFalse())
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

			err = handler(message)
			Expect(err).To(BeNil())

			updatedHost, err := host.Find(db, u, h.Slug)
			Expect(err).To(BeNil())

			Expect(updatedHost.IPAddress.String).To(Equal("4.5.6.7"))
			Expect(updatedHost.ProviderIdentifier.String).To(Equal("host-3456"))
			Expect(updatedHost.RegionIdentifier.String).To(Equal("here"))
			Expect(updatedHost.FreighterdPort.Int64).To(Equal(int64(12345)))
			Expect(updatedHost.State).To(Equal(host.StateRunning))
			Expect(updatedHost.HourlyPrice).To(Equal(int64(1000000)))
			Expect(updatedHost.Free).To(BeFalse())
		})
	})

	Context("failed to create a host", func() {
		var message *queue.Message

		BeforeEach(func() {
			queueInfo = &hh.QueueInfo{
				ID:      h.ID,
				Slug:    h.Slug,
				UserID:  u.ID,
				Message: "Failed to create a host",
				Host: &hh.Info{
					Provider:       "docker",
					ID:             "host-1234",
					Region:         "somewhere",
					Name:           "hostname-1234",
					IPAddress:      "1.2.3.4",
					FreighterdPort: 62345,
					State:          state.Running,
				},
			}
			payload, err := json.Marshal(queueInfo)
			Expect(err).To(BeNil())
			message = &queue.Message{
				UUID:         "uuid-1234",
				Action:       "create",
				Event:        string(event.HostCreateFailed),
				ResourceName: "host",
				ResourceID:   "host-1234",
				Payload:      payload,
			}
		})

		It("sets the host as an orphan", func() {
			err := handler(message)
			Expect(err).To(BeNil())

			updatedHost, err := host.Find(db, u, h.Slug)
			Expect(err).To(BeNil())

			Expect(updatedHost.State).To(Equal(host.StateOrphan))
		})

		It("publishes the event", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			err := handler(message)
			Expect(err).To(BeNil())

			evtCh := make(chan *controllers.Event)
			go func() {
				var evt *controllers.Event
				for msg := range ch { // Ignore events we don't care about.
					evt = msg.(*controllers.Event)
					if evt.Name == controllers.EventName(event.HostCreateFailed) {
						evtCh <- evt
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timeout while getting image created event")
			case evt := <-evtCh:
				Expect(evt.Name).To(Equal(controllers.EventName(event.HostCreateFailed)))
			}
		})
	})
})
