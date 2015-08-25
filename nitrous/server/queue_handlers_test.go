package server_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
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
		redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
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

		_, err = db.Exec(`UPDATE plans SET stripe_plan_id = 'plan-id' WHERE id = $1;`, hs.P.ID)
		Expect(err).To(BeNil())

		svr = server.NewServer("", db, nil, nil)
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
			var (
				message    *queue.Message
				mockClient *freighterclient.MockClient
				fsub       *test.FakeSubscriptionProvider
				rt         *test.FakeRoundTripper
			)

			BeforeEach(func() {
				mockClient = &freighterclient.MockClient{}
				test.StubDefaultFreighterFn(mockClient)
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
						VolumeID:       "volume-1234",
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

			AfterEach(func() {
				test.UnstubFreighterFn()
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
				Expect(updatedHost.ProviderStorageIdentifier.String).To(Equal("volume-1234"))
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

			It("logs transitions for the host", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				hosts, err := host.FindAllByUser(db, u.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))

				var count int
				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hosts[0].ID, host.StateRunning, host.StateBorn).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("creates a shared volume and publishes a volume created event", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				hosts, err := host.FindAllByUser(db, u.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))
				vol, err := volume.Find(db, hosts[0].ID, "shared")
				Expect(err).To(BeNil())
				Expect(vol).NotTo(BeNil())
				Expect(vol.System).To(BeTrue())

				evtCh := make(chan *controllers.Event)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventVolumeCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timeout while getting shared volume created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventVolumeCreated))
					evtData, ok := evt.Data.(map[string]*serializers.Volume)
					Expect(ok).To(BeTrue())
					Expect(evtData["volume"]).To(Equal(serializers.SerializeVolume(vol)))
				}
			})

			It("terminates the host if the subscribe fails", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				fsub.SubscribeError = errors.New("I failed")

				err := handler(message)
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

				fsub.SubscribeError = subscriptions.ErrCardDeclined

				err := handler(message)
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

				fsub.SubscribeError = subscriptions.ErrCardExpired

				err := handler(message)
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
				fsub.SubscribeError = nil
				fsub.SubscribeReturn = &subscription.Subscription{}

				err := handler(message)
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

			Context("with an organisation", func() {
				var (
					o *org.Org
				)

				BeforeEach(func() {
					o = &org.Org{
						Name:  "Foo Bar, Inc.",
						Slug:  "foobar",
						Email: "eng@foobar.com",
					}
					err := o.Insert(db)
					Expect(err).To(BeNil())
					err = db.WithTransaction(func(t *sqlx.Tx) error {
						Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
						return nil
					})
					Expect(err).To(BeNil())
					err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE hosts SET org_id = $1 WHERE id = $2;`, o.ID, hs.H.ID)
					Expect(err).To(BeNil())
				})

				It("terminates the host if the organisation could not be found", func() {
					_, err := db.Exec(`UPDATE orgs SET deleted_at = now() WHERE id = $1;`, o.ID)
					Expect(err).To(BeNil())

					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					err = handler(message)
					Expect(err).To(MatchError("Unable to locate organization."))
					Expect(rt.Requests).To(HaveLen(1))

					Expect(fsub.SubscribeCalled).To(BeFalse())

					evt, err := test.WaitForEvent(ch, controllers.EventHostCreateFailed, 250*time.Millisecond)
					Expect(err).To(BeNil())
					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(s).To(Equal("Unable to locate organization."))

					hosts, _ := host.FindAllByUser(db, u.ID)
					Expect(hosts).To(HaveLen(0))
				})

				It("terminates the host if the user is not a member of the organisation", func() {
					_, err := db.Exec(`DELETE FROM org_memberships WHERE org_id = $1`, o.ID)
					Expect(err).To(BeNil())

					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					err = handler(message)
					Expect(err).To(MatchError("You do not have access to this organization."))
					Expect(rt.Requests).To(HaveLen(1))

					Expect(fsub.SubscribeCalled).To(BeFalse())

					evt, err := test.WaitForEvent(ch, controllers.EventHostCreateFailed, 250*time.Millisecond)
					Expect(err).To(BeNil())
					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(s).To(Equal("You do not have access to this organization."))

					hosts, _ := host.FindAllByUser(db, u.ID)
					Expect(hosts).To(HaveLen(0))
				})

				It("continues if the user is a member of the organisation", func() {
					fsub.SubscribeError = nil
					fsub.SubscribeReturn = &subscription.Subscription{}

					err := handler(message)
					Expect(err).To(BeNil())
					Expect(rt.Requests).To(HaveLen(0))

					Expect(fsub.SubscribeCalled).To(BeTrue())
					Expect(fsub.SubscribeHost.Slug).To(HavePrefix("host-123"))
					Expect(fsub.SubscribePlan.Slug).To(Equal(hs.P.Slug))
					Expect(fsub.SubscribeUser.ID).To(Equal(u.ID))
					Expect(fsub.SubscribeOrg.ID).To(Equal(o.ID))

					hosts, err := host.FindAllByUser(db, u.ID)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					Expect(hosts[0].Slug).To(HavePrefix("host-123"))
				})
			})

			Context("Provider is amazon", func() {
				BeforeEach(func() {
					queueInfo = &hh.QueueInfo{
						ID:     h.ID,
						Slug:   h.Slug,
						UserID: u.ID,
						Host: &hh.Info{
							Provider:       "amazon",
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

				It("returns error when volumeID is nil", func() {
					err := handler(message)
					Expect(err).ToNot(BeNil())
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
						ID:             "", // Host ID is cleared when host is stopped.
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

			It("updates the host's provider identifier", func() {
				Expect(h.ProviderIdentifier.String).ToNot(BeEmpty())

				err := handler(message)
				Expect(err).To(BeNil())

				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())

				Expect(updatedHost.ProviderIdentifier.String).To(Equal(""))
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

	Describe("Respec Host Message", func() {
		var (
			message   *queue.Message
			requester *user.User
			r         *respec.Respec
		)

		BeforeEach(func() {
			requester = &user.User{
				Username: "requester",
				Password: "123456",
				Email:    "requester@example.com",
			}
			Expect(test.WrapInTransaction(db, requester.Insert)).To(BeNil())

			p := &plan.Plan{Slug: "1gb", Name: "small", MaxContainers: 3, Price: 1000}
			Expect(p.Insert(db)).To(BeNil())

			pt := &providertemplate.ProviderTemplate{
				RegionID:        hs.PT.RegionID,
				PlanID:          p.ID,
				Name:            "new-host-size",
				Provider:        "ec2",
				PreprovisionMax: 2,
			}
			Expect(pt.Insert(db))

			r = &respec.Respec{
				HostID:             hs.H.ID,
				ProviderTemplateID: pt.ID,
			}
			Expect(test.WrapInTransaction(db, r.Insert)).To(BeNil())
			_, err := db.Exec("UPDATE respecs SET state = $1 WHERE id = $2", respec.StateProcessing, r.ID)
			Expect(err).To(BeNil())
		})

		Context("a host is respeccing", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
					Message:     "respeccing host",
				}

				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message = &queue.Message{
					UUID:         "uuid-1234",
					Action:       "respec",
					Event:        string(event.HostRespeccing),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("emits the message to the event publisher", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				evt, err := test.WaitForEvent(ch, controllers.EventHostRespeccing, 250*time.Millisecond)
				Expect(err).To(BeNil())

				evtData, _ := evt.Data.(map[string]interface{})
				h2, ok := evtData["host"].(*serializers.Host)
				Expect(ok).To(BeTrue())
				Expect(h2.ID).To(Equal(h.ID))
			})
		})

		Context("a host is respecced", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
					Host: &hh.Info{
						Provider:       "docker",
						ID:             "host-2345",
						VolumeID:       "vol-abcd",
						Region:         "somewhere",
						Name:           "hostname-1234",
						IPAddress:      "192.2.3.4",
						FreighterdPort: 62345,
						State:          state.Running,
					},
					Message: "respecced host",
				}

				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message = &queue.Message{
					UUID:         "uuid-1234",
					Action:       "respec",
					Event:        string(event.HostRespecced),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("updates the host", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())

				Expect(updatedHost.ProviderTemplateID).To(Equal(r.ProviderTemplateID))
				Expect(updatedHost.State).To(Equal(host.StateRunning))
				Expect(updatedHost.IPAddress.String).To(Equal("192.2.3.4"))
				Expect(updatedHost.ProviderIdentifier.String).To(Equal("host-2345"))
				Expect(updatedHost.ProviderStorageIdentifier.String).To(Equal("vol-abcd"))
			})

			It("updates the respec record", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				r := &respec.Respec{}
				err = db.Get(r, `SELECT * FROM respecs WHERE host_id = $1 LIMIT 1`, h.ID)
				Expect(err).To(BeNil())

				Expect(r.State).To(Equal(respec.StateCompleted))
			})

			It("emits the message to the event publisher", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				evt, err := test.WaitForEvent(ch, controllers.EventHostRespecced, 250*time.Millisecond)
				Expect(err).To(BeNil())

				evtData, _ := evt.Data.(map[string]interface{})
				h2, ok := evtData["host"].(*serializers.Host)
				Expect(ok).To(BeTrue())
				Expect(h2.ID).To(Equal(h.ID))
			})

			It("should return an error if the HostInfo is nil", func() {
				queueInfo.Host = nil
				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message.Payload = payload

				err = handler(message)
				Expect(err).ToNot(BeNil())
			})

			It("updates state to stopped if hostbuilder says that host state is stopped", func() {
				// Send message with state of "stopped".
				queueInfo.Host.State = state.Stopped
				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message := &queue.Message{
					UUID:         "uuid-1234",
					Action:       "respec",
					Event:        string(event.HostRespecced),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}

				err = handler(message)
				Expect(err).To(BeNil())

				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())

				Expect(updatedHost.State).To(Equal(host.StateStopped))
			})
		})

		Context("a host respec fails", func() {
			BeforeEach(func() {
				queueInfo = &hh.QueueInfo{
					ID:          h.ID,
					Slug:        h.Slug,
					UserID:      u.ID,
					RequesterID: requester.ID,
					Message:     "Amazon is down",
				}

				payload, err := json.Marshal(queueInfo)
				Expect(err).To(BeNil())
				message = &queue.Message{
					UUID:         "uuid-1234",
					Action:       "respec",
					Event:        string(event.HostRespecFailed),
					ResourceName: "host",
					ResourceID:   "host-1234",
					Payload:      payload,
				}
			})

			It("updates the host", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				updatedHost, err := host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())

				Expect(updatedHost.ProviderTemplateID).ToNot(Equal(r.ProviderTemplateID))
				Expect(updatedHost.State).To(Equal(host.StateRespecFailed))
			})

			It("updates the respec record", func() {
				err := handler(message)
				Expect(err).To(BeNil())

				r := &respec.Respec{}
				err = db.Get(r, `SELECT * FROM respecs WHERE host_id = $1 LIMIT 1`, h.ID)
				Expect(err).To(BeNil())

				Expect(r.State).To(Equal(respec.StateFailed))
			})

			It("emits the message to the event publisher", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				err := handler(message)
				Expect(err).To(BeNil())

				evt, err := test.WaitForEvent(ch, controllers.EventHostRespecFailed, 250*time.Millisecond)
				Expect(err).To(BeNil())

				evtData, _ := evt.Data.(map[string]interface{})
				h2, ok := evtData["host"].(*serializers.Host)
				Expect(ok).To(BeTrue())
				Expect(h2.ID).To(Equal(h.ID))
				e, ok := evtData["error"].(string)
				Expect(ok).To(BeTrue())
				Expect(e).To(Equal("Amazon is down"))
			})
		})

	})
})
