package server_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/event"
	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/pkg/queue"
	redistest "github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/redis.v2"
)

var _ = Describe("HostQueueHandler", func() {
	var (
		redisClient *redis.Client
		qp          queue.Publisher
		qc          queue.Consumer
		db          *db.Database
		handler     func(m *queue.Message) error
		h           *host.Host
		u           *user.User
		svr         *server.Server
		queueInfo   *hh.QueueInfo
		hs          *test.HostStack
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB)
		Expect(err).To(BeNil())
		qp = queue.NewQueue(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, "host")
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
		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:  "host-123",
				Size:  sql.NullString{"micro", true},
				State: host.StateBorn,
			},
			U: u,
		})
		Expect(err).To(BeNil())
		h = hs.H

		svr = server.NewServer("", db, nil)
		handler = svr.HostQueueHandler()
	})

	AfterEach(func() {
		if qp != nil {
			qp.Delete()
		}
		if redisClient != nil {
			redisClient.Close()
		}
	})

	Describe("Host Create Message", func() {
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

			It("updates even if the host is running", func() {
				h.State = host.StateRunning
				h.IPAddress = sql.NullString{"4.5.6.7", true}
				h.ProviderIdentifier = sql.NullString{"host-3456", true}
				h.RegionIdentifier = sql.NullString{"here", true}
				h.FreighterdPort = sql.NullInt64{12345, true}
				h.HourlyPrice = 1000000

				err := h.UpdateWithPrice(db)
				Expect(err).To(BeNil())

				err = handler(message)
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

			Context("when the host plan requires subscription", func() {
				var (
					fsub *test.FakeSubscriptionProvider
					rt   *test.FakeRoundTripper
				)

				BeforeEach(func() {
					fsub = &test.FakeSubscriptionProvider{}
					svr.SubscriptionProvider = fsub

					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					rt = &test.FakeRoundTripper{
						Message: `{"freighterd_port":65333,"ip_address":"127.0.0.1"}`,
						Status:  http.StatusOK,
					}
					hb.Client.Client.Transport = rt
					hostcontrol.HostbuilderClient = hb
				})

				It("terminates the host if the subscribe fails", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					_, err := db.Exec(`UPDATE plans SET stripe_plan_id = 'plan-id' WHERE id = $1;`, hs.P.ID)
					Expect(err).To(BeNil())

					fsub.SubscribeError = errors.New("I failed")

					err = handler(message)
					Expect(err).To(MatchError("Failed to create subscription, please try again."))
					Expect(rt.Requests).To(HaveLen(1))

					Expect(fsub.SubscribeCalled).To(BeTrue())
					Expect(fsub.SubscribeHost.Slug).To(HavePrefix("host-123"))
					Expect(fsub.SubscribePlan.Slug).To(Equal(hs.P.Slug))
					Expect(fsub.SubscribeUser.ID).To(Equal(u.ID))

					evt, err := test.WaitForEvent(ch, controllers.EventHostCreateFailed, 250*time.Millisecond)
					Expect(err).To(BeNil())
					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(s).To(Equal("Failed to create subscription, please try again."))

					hosts, _ := host.FindAllByUser(db, u.ID)
					Expect(hosts).To(HaveLen(0))
				})

				It("terminates the host if the card is declined", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					_, err := db.Exec(`UPDATE plans SET stripe_plan_id = 'plan-id' WHERE id = $1;`, hs.P.ID)
					Expect(err).To(BeNil())

					fsub.SubscribeError = subscriptions.ErrCardDeclined

					err = handler(message)
					Expect(err).To(MatchError(subscriptions.ErrCardDeclined.Error()))
					Expect(rt.Requests).To(HaveLen(1))

					Expect(fsub.SubscribeCalled).To(BeTrue())
					Expect(fsub.SubscribeHost.Slug).To(HavePrefix("host-123"))
					Expect(fsub.SubscribePlan.Slug).To(Equal(hs.P.Slug))
					Expect(fsub.SubscribeUser.ID).To(Equal(u.ID))

					evt, err := test.WaitForEvent(ch, controllers.EventHostCreateFailed, 250*time.Millisecond)
					Expect(err).To(BeNil())
					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(s).To(Equal(subscriptions.ErrCardDeclined.Error()))

					hosts, _ := host.FindAllByUser(db, u.ID)
					Expect(hosts).To(HaveLen(0))
				})

				It("terminates the host if the card is explired", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					_, err := db.Exec(`UPDATE plans SET stripe_plan_id = 'plan-id' WHERE id = $1;`, hs.P.ID)
					Expect(err).To(BeNil())

					fsub.SubscribeError = subscriptions.ErrCardExpired

					err = handler(message)
					Expect(err).To(MatchError(subscriptions.ErrCardExpired.Error()))
					Expect(rt.Requests).To(HaveLen(1))

					Expect(fsub.SubscribeCalled).To(BeTrue())
					Expect(fsub.SubscribeHost.Slug).To(HavePrefix("host-123"))
					Expect(fsub.SubscribePlan.Slug).To(Equal(hs.P.Slug))
					Expect(fsub.SubscribeUser.ID).To(Equal(u.ID))

					evt, err := test.WaitForEvent(ch, controllers.EventHostCreateFailed, 250*time.Millisecond)
					Expect(err).To(BeNil())
					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(s).To(Equal(subscriptions.ErrCardExpired.Error()))

					hosts, _ := host.FindAllByUser(db, u.ID)
					Expect(hosts).To(HaveLen(0))
				})

				It("continues creating if the subscribe succeeds", func() {
					_, err := db.Exec(`UPDATE plans SET stripe_plan_id = 'plan-id' WHERE id = $1;`, hs.P.ID)
					Expect(err).To(BeNil())

					fsub.SubscribeError = nil
					fsub.SubscribeReturn = &subscription.Subscription{}

					err = handler(message)
					Expect(err).To(BeNil())
					Expect(rt.Requests).To(HaveLen(0))

					Expect(fsub.SubscribeCalled).To(BeTrue())
					Expect(fsub.SubscribeHost.Slug).To(HavePrefix("host-123"))
					Expect(fsub.SubscribePlan.Slug).To(Equal(hs.P.Slug))
					Expect(fsub.SubscribeUser.ID).To(Equal(u.ID))

					hosts, err := host.FindAllByUser(db, u.ID)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					Expect(hosts[0].Slug).To(HavePrefix("host-123"))
				})
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

				err = h.UpdateWithPrice(db)
				Expect(err).To(BeNil())
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

	Describe("Stop Host Message", func() {
		var (
			message      *queue.Message
			requester    *user.User
			collaborator *user.User
			c1           *container.Container
			c2           *container.Container
		)

		BeforeEach(func() {
			h.State = host.StateStopping
			err := h.Update(db)
			Expect(err).To(BeNil())
			requester = &user.User{
				Username: "requester",
				Password: "123456",
				Email:    "requester@example.com",
			}
			err = test.WrapInTransaction(db, requester.Insert)
			Expect(err).To(BeNil())

			c1 = &container.Container{
				CID:    "beefface",
				Slug:   "rails-box",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: h.ID,
			}
			err = c1.Insert(db)
			Expect(err).To(BeNil())
			c2 = &container.Container{
				CID:    "cafedead",
				Slug:   "python-box",
				Image:  "nitrous-base",
				State:  container.StateRunning,
				UserID: u.ID,
				HostID: h.ID,
			}
			err = c2.Insert(db)
			collaborator = &user.User{
				Username: "shared",
				Email:    "shared@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, collaborator.Insert)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c1.ID, collaborator.ID)
			Expect(err).To(BeNil())
		})

		Context("a host is stopping", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
					Message:     "stopping host",
				}

				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message = &queue.Message{
					UUID:         "uuid-1234",
					Action:       "stop",
					Event:        string(event.HostStopping),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("does not return any errors", func() {
				err := handler(message)
				Expect(err).To(BeNil())
			})
		})

		Context("a host is stopped", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
					Host: &hh.Info{
						Provider:       "docker",
						ID:             "host-1234",
						Region:         "somewhere",
						Name:           "hostname-1234",
						IPAddress:      "1.2.3.4",
						FreighterdPort: 62345,
						State:          state.Stopped,
					},
				}

				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message = &queue.Message{
					UUID:         "uuid-1234",
					Action:       "stop",
					Event:        string(event.HostStopped),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("updates the host to be stopped", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())

				Expect(updatedHost.State).To(Equal(host.StateStopped))
			})

			It("logs transitions for the host", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				var count int
				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					h.ID, host.StateStopped, host.StateStopping).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("publishes containaer stopped event host stopeed event to user", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				evtCh := make(chan []*controllers.Event)
				events := make([]*controllers.Event, 0)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerStopped ||
							evt.Name == controllers.EventName(event.HostStopped) {
							events = append(events, evt)
							if len(events) >= 3 {
								evtCh <- events
								break
							}
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timeout while getting image created event")
				case events := <-evtCh:
					for _, event := range events[0:2] {
						evtData, ok := event.Data.(map[string]interface{})
						Expect(ok).To(BeTrue())
						cn, ok := evtData["container"].(*serializers.Container)
						Expect(cn.State).To(Equal(container.StateStopped.String()))
					}

					evtData, ok := events[2].Data.(map[string]*serializers.Host)
					Expect(ok).To(BeTrue())
					Expect(evtData["host"].State).To(Equal(host.StateStopped))
				}
			})

			It("publishes containaer stopped event to collaborator", func() {
				ch := svr.Events.Subscribe(collaborator.Topic())
				defer svr.Events.Unsubscribe(ch, collaborator.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				evtCh := make(chan []*controllers.Event)
				events := make([]*controllers.Event, 0)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerStopped {
							events = append(events, evt)
							if len(events) >= 1 {
								evtCh <- events
								break
							}
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timeout while getting image created event")
				case events := <-evtCh:
					for _, event := range events {
						evtData, ok := event.Data.(map[string]interface{})
						Expect(ok).To(BeTrue())
						cn, ok := evtData["container"].(*serializers.Container)
						Expect(cn.State).To(Equal(container.StateStopped.String()))
					}
				}
			})
		})

		Context("a host failed to stop", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
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
					Action:       "stop",
					Event:        string(event.HostStopFailed),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("resets the host to state in host info", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())

				Expect(updatedHost.State).To(Equal(host.StateRunning))
			})

			It("logs transitions for the host", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				var count int
				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					h.ID, host.StateRunning, host.StateStopping).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("publishes containaer stopped event host stopeed event to user", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				evtCh := make(chan *controllers.Event)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventName(event.HostUpdated) {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timeout while getting image created event")
				case event := <-evtCh:
					evtData, ok := event.Data.(map[string]*serializers.Host)
					Expect(ok).To(BeTrue())
					Expect(evtData["host"].State).To(Equal(host.StateRunning))
				}
			})
		})

		Context("When fails to stop host and host info is nil", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
					Host:        nil,
				}

				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message = &queue.Message{
					UUID:         "uuid-1234",
					Action:       "stop",
					Event:        string(event.HostStopFailed),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("should return error", func() {
				err := handler(message)
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
