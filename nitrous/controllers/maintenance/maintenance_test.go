package maintenance_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/maintenance")
}

var _ = Describe("controllers/maintenance", func() {
	var (
		svr     *server.Server
		db      *db.Database
		handler *mux.Router

		authTokenInputs test.AuthTokenInputs

		u *user.User
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = test.NewTestServer("", db, nil)
		authTokenInputs = test.AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/admin/amazon/hosts", func() {
		It("responds with 404 Not Found when the application lacks a TRUST_KEY", func() {
			prevKey := envvars.TrustKey
			envvars.TrustKey = ""
			defer func() {
				envvars.TrustKey = prevKey
			}()

			req, err := test.NewRequest("POST", "/api/v0/maintenance/user_refresh/902", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("responds with 404 Not Found when the TRUST_KEY does not match", func() {
			prevKey := envvars.TrustKey
			envvars.TrustKey = "I_AM_TKIP"
			defer func() {
				envvars.TrustKey = prevKey
			}()

			req, err := test.NewRequest("POST", "/api/v0/maintenance/user_refresh/902", &url.Values{"trust_key": []string{"I_AM_AES"}})
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		Context("when the requester is local", func() {
			var (
				prevKey string
			)

			doRequest := func(id int64) *httptest.ResponseRecorder {
				path := fmt.Sprintf("/api/v0/maintenance/user_refresh/%d", id)
				req, err := test.NewRequest("POST", path, &url.Values{"trust_key": []string{"I_AM_YOUR_KEY"}})
				Expect(err).To(BeNil())
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}

			BeforeEach(func() {
				prevKey = envvars.TrustKey
				envvars.TrustKey = "I_AM_YOUR_KEY"
			})

			AfterEach(func() {
				envvars.TrustKey = prevKey
			})

			It("should return an error if the user could not be located", func() {
				res := doRequest(1)
				Expect(res.Code).To(Equal(http.StatusNotFound))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("user_not_found"))
				Expect(errDesc).To(Equal("user not found"))
			})

			Context("sending out host events", func() {
				hMap := map[string]controllers.EventName{
					host.StateRunning:    controllers.EventHostUpdated,
					host.StateStopped:    controllers.EventHostStopped,
					host.StateStopping:   controllers.EventHostStopping,
					host.StateStarting:   controllers.EventHostStarting,
					host.StateRestarting: controllers.EventHostRestarting,
					host.StateTerminated: controllers.EventHostDestroyed,
				}

				for hState, evState := range hMap {
					It(fmt.Sprintf("it should emit event %s", evState), func() {
						hs1, err := test.CreateHostStack(db, &test.HostStack{
							H: &host.Host{
								State: hState,
								Slug:  "healthy-host-1",
							},
							U: u,
						})
						Expect(err).To(BeNil())

						ch := svr.Events.Subscribe(u.Topic())
						defer svr.Events.Unsubscribe(ch, u.Topic())

						res := doRequest(u.ID)
						Expect(res.Code).To(Equal(http.StatusOK))

						evtCh := make(chan *controllers.Event)
						go func() {
							var evt *controllers.Event
							for msg := range ch { // Ignore events we don't care about.
								evt = msg.(*controllers.Event)
								if evt.Name == evState {
									evtCh <- evt
									break
								}
							}
						}()

						select {
						case <-time.After(50 * time.Millisecond):
							Fail("Timeout while getting host event")
						case evt := <-evtCh:
							Expect(evt.Name).To(Equal(evState))
							evtData, ok := evt.Data.(map[string]*serializers.Host)
							Expect(ok).To(BeTrue())
							Expect(evtData["host"].ID).To(Equal(hs1.H.ID))
							Expect(evtData["host"].State).To(Equal(hState))
						}
					})
				}
			})

			Context("should emit container events", func() {
				type evInfo struct {
					Event          controllers.EventName
					ContainerState string
				}
				hMap := map[string]evInfo{
					host.StateStopped:    evInfo{controllers.EventContainerStopped, container.StateStopped},
					host.StateTerminated: evInfo{controllers.EventContainerDestroyed, container.StateTerminated},
				}

				for hState, evState := range hMap {
					It(fmt.Sprintf("it should emit event %s", evState.Event), func() {
						hs1, err := test.CreateHostStack(db, &test.HostStack{
							H: &host.Host{
								State: hState,
								Slug:  "healthy-host-1",
							},
							U: u,
						})
						Expect(err).To(BeNil())

						c := &container.Container{
							CID:    "beefface",
							Slug:   "rails-box",
							Image:  "nitrous-base",
							UserID: u.ID,
							HostID: hs1.H.ID,
						}
						err = c.Insert(db)
						Expect(err).To(BeNil())

						ch := svr.Events.Subscribe(u.Topic())
						defer svr.Events.Unsubscribe(ch, u.Topic())

						res := doRequest(u.ID)
						Expect(res.Code).To(Equal(http.StatusOK))

						evtCh := make(chan *controllers.Event)
						go func() {
							var evt *controllers.Event
							for msg := range ch { // Ignore events we don't care about.
								evt = msg.(*controllers.Event)
								if evt.Name == evState.Event {
									evtCh <- evt
									break
								}
							}
						}()

						select {
						case <-time.After(50 * time.Millisecond):
							Fail("Timeout while getting container event")
						case evt := <-evtCh:
							Expect(evt.Name).To(Equal(evState.Event))
							evtData, ok := evt.Data.(map[string]interface{})
							Expect(ok).To(BeTrue())
							cn, ok := evtData["container"].(*serializers.Container)
							Expect(ok).To(BeTrue())
							Expect(cn.ID).To(Equal(c.ID))
							Expect(cn.State).To(Equal(evState.ContainerState))
						}
					})
				}
			})
		})

	})
})
