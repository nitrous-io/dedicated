package amazon_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/amazon"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/amazon")
}

const hbLog = `2015/04/23 17:34:39 uuid=d2cc8693 Created host: &{Provider:amazon ID:i-c425e333 Region:us-west-2 Name: IPAddress:54.149.78.56 FreighterdPort:65333 State:Running}`

var _ = Describe("controllers/amazon", func() {
	var (
		svr     *server.Server
		db      *db.Database
		handler *mux.Router

		authTokenInputs test.AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
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

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/admin/amazon/hosts", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/admin/amazon/hosts", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			var hs1, hs2, hs3 *test.HostStack

			BeforeEach(func() {
				var err error

				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				hs1, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "ec2-healthy-1", Valid: true},
						State:              host.StateRunning,
						Slug:               "healthy-host-1",
					},
					PT: &providertemplate.ProviderTemplate{
						Provider: "amazon",
						Name:     "ec2-sgp-512mb",
					},
					U: u,
				})
				Expect(err).To(BeNil())

				hs2, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "ec2-dangling-1", Valid: true},
						State:              host.StateOrphan,
						Slug:               "dangling-host-1",
					},
					U:  u,
					PT: hs1.PT,
					P:  hs1.P,
					R:  hs1.R,
				})
				Expect(err).To(BeNil())

				hs3, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "ec2-inconsistent-1", Valid: true},
						State:              host.StateUpdating,
						Slug:               "inconsistent-host-1",
					},
					U:  u,
					PT: hs1.PT,
					P:  hs1.P,
					R:  hs1.R,
				})
				Expect(err).To(BeNil())

				// Fake hostbuilderclient response.
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt := &test.FakeRoundTripper{
					Message: `{
						"hosts": [
							{
								"provider": "amazon",
								"id": "ec2-healthy-1",
								"name": "i-am-healthy",
								"ip_address": "1.2.3.4",
								"freighterd_port": 65333,
								"state": "Running"
							},
							{
								"provider": "amazon",
								"id": "ec2-unaccounted-1",
								"name": "not-in-db",
								"ip_address": "2.4.6.8",
								"freighterd_port": 65333,
								"state": "Running"
							},
							{
								"provider": "amazon",
								"id": "ec2-inconsistent-1",
								"name": "running-or-upgrading",
								"ip_address": "1.6.6.6",
								"freighterd_port": 65334,
								"state": "Running"
							}
						]
					}`,
					Status: http.StatusOK,
				}
				hb.Client.Client.Transport = rt
				amazon.HostbuilderClient = hb
			})

			It("responds with list of grouped hosts", func() {
				res := doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Hosts *serializers.HostList `json:"hosts"`
				}

				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				hosts := j.Hosts

				Expect(len(hosts.AllHosts)).To(Equal(4))

				Expect(len(hosts.DanglingHosts)).To(Equal(1))
				Expect(hosts.DanglingHosts[0].Slug).To(Equal("dangling-host-1"))
				Expect(hosts.DanglingHosts[0].ProviderIdentifier).To(Equal("ec2-dangling-1"))

				Expect(len(hosts.UnaccountedHosts)).To(Equal(1))
				Expect(hosts.UnaccountedHosts[0].ID).To(Equal("ec2-unaccounted-1"))

				Expect(len(hosts.InconsistentHosts)).To(Equal(1))
				Expect(hosts.InconsistentHosts[0].DBHost.Slug).To(Equal("inconsistent-host-1"))
				Expect(hosts.InconsistentHosts[0].DBHost.ProviderIdentifier).To(Equal("ec2-inconsistent-1"))
				Expect(hosts.InconsistentHosts[0].DBHost.State).To(Equal("updating"))
				Expect(hosts.InconsistentHosts[0].LiveHost.ID).To(Equal("ec2-inconsistent-1"))
				Expect(hosts.InconsistentHosts[0].LiveHost.State).To(Equal("Running"))

				Expect(len(hosts.HealthyHosts)).To(Equal(1))
				Expect(hosts.HealthyHosts[0].DBHost.Slug).To(Equal("healthy-host-1"))
				Expect(hosts.HealthyHosts[0].DBHost.ProviderIdentifier).To(Equal("ec2-healthy-1"))
				Expect(hosts.HealthyHosts[0].DBHost.State).To(Equal("running"))
				Expect(hosts.HealthyHosts[0].LiveHost.ID).To(Equal("ec2-healthy-1"))
				Expect(hosts.HealthyHosts[0].LiveHost.State).To(Equal("Running"))
			})
		})
	})

	Describe("POST /api/v0/admin/amazon/fix_born", func() {
		doRequest := func(hostID int64, params *url.Values) *httptest.ResponseRecorder {
			path := fmt.Sprintf("/api/v0/admin/amazon/fix_born/%d", hostID)
			req, err := test.NewAuthedRequest(t, "POST", path, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(1, nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			var (
				hs1        *test.HostStack
				mockClient *freighterclient.MockClient
			)

			BeforeEach(func() {

				mockClient = &freighterclient.MockClient{}
				test.StubDefaultFreighterFn(mockClient)

				var err error

				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				hs1, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						State: host.StateBorn,
						Slug:  "healthy-host-1",
					},
					PT: &providertemplate.ProviderTemplate{
						Provider: "amazon",
						Name:     "ec2-sgp-512mb",
					},
					P: &plan.Plan{
						Slug:          "hs1Plan",
						Name:          "hs1Plan",
						MaxContainers: 1,
						Price:         2016,
					},
					U: u,
				})
				Expect(err).To(BeNil())
			})

			It("should return an error if the host cannot be found", func() {
				res := doRequest(1, nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("host_not_found"))
				Expect(errDesc).To(Equal("host not found"))
			})

			It("should require the `host_builder` form value", func() {
				res := doRequest(hs1.H.ID, nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("missing_host_builder"))
				Expect(errDesc).To(Equal("`host_builder` must be provided"))
			})

			It("should require `host_builder` be parseable", func() {
				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{"Parseable"}})
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("missing_id"))
				Expect(errDesc).To(Equal("ID: could not be located"))
			})

			It("should return an error if another host exists with the same provider ID", func() {
				hs2, err := test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						State:              host.StateBorn,
						Slug:               "healthy-host-2",
						ProviderIdentifier: sql.NullString{"i-c425e333", true},
					},
					PT: &providertemplate.ProviderTemplate{
						Provider: "amazon",
						Name:     "ec2-sgp-512mb",
					},
					U: u,
				})
				Expect(err).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusConflict))
				errT, errDesc := test.ErrorSlug(res.Body)
				Expect(errT).To(Equal("existing_host"))
				Expect(errDesc).To(Equal(fmt.Sprintf("IP or Identifier collides with host %d", hs2.H.ID)))
			})

			It("should return an error if another host exists with the same IP address", func() {
				hs2, err := test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						State:     host.StateBorn,
						Slug:      "healthy-host-2",
						IPAddress: sql.NullString{"54.149.78.56", true},
					},
					PT: &providertemplate.ProviderTemplate{
						Provider: "amazon",
						Name:     "ec2-sgp-512mb",
					},
					U: u,
				})
				Expect(err).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusConflict))
				errT, errDesc := test.ErrorSlug(res.Body)
				Expect(errT).To(Equal("existing_host"))
				Expect(errDesc).To(Equal(fmt.Sprintf("IP or Identifier collides with host %d", hs2.H.ID)))
			})

			It("should update the host with HB details", func() {
				_, err := db.Exec("UPDATE hosts SET hourly_price = 0, free = true, max_containers = 90, size = NULL WHERE id = $1;", hs1.H.ID)
				Expect(err).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Status string `json:"status"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Status).To(Equal("ok"))

				h, err := host.FindByIdentifier(db, "healthy-host-1")
				Expect(err).To(BeNil())

				Expect(h.ProviderIdentifier).To(Equal(sql.NullString{"i-c425e333", true}))
				Expect(h.RegionIdentifier).To(Equal(sql.NullString{"us-west-2", true}))
				Expect(h.IPAddress).To(Equal(sql.NullString{"54.149.78.56", true}))
				Expect(h.FreighterdPort).To(Equal(sql.NullInt64{65333, true}))
				Expect(h.State).To(Equal("running"))
				Expect(h.HourlyPrice).To(Equal(hs1.P.HourlyPrice()))
				Expect(h.Free).To(Equal(hs1.P.IsFree()))
				Expect(h.MaxContainers).To(Equal(sql.NullInt64{1, true}))
				Expect(h.Size).To(Equal(sql.NullString{"hs1Plan", true}))
			})

			It("should create a host transition", func() {
				_, err := db.Exec("DELETE FROM host_transitions WHERE host_id = $1;", hs1.H.ID)
				Expect(err).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Status string `json:"status"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Status).To(Equal("ok"))

				h, err := host.FindByIdentifier(db, "healthy-host-1")
				Expect(err).To(BeNil())

				hT, err := hosttransition.FindMostRecentForHost(db, hs1.H.ID)
				Expect(err).To(BeNil())
				Expect(hT.PreviousState).To(Equal(sql.NullString{"born", true}))
				Expect(hT.State).To(Equal("running"))
				Expect(hT.CreatedAt).To(Equal(h.CreatedAt))
				Expect(hT.EndedAt).To(BeNil())
			})

			It("should not create a host transition if it already exists", func() {
				_, err := db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) VALUES ($1, 'born', 'running', now());", hs1.H.ID)
				Expect(err).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Status string `json:"status"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Status).To(Equal("ok"))

				h, err := host.FindByIdentifier(db, "healthy-host-1")
				Expect(err).To(BeNil())

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM host_transitions WHERE host_id = $1;`, h.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("should create a host transition if the eldest is not a previous born", func() {
				_, err := db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) VALUES ($1, 'running', 'restarting', now());", hs1.H.ID)
				Expect(err).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Status string `json:"status"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Status).To(Equal("ok"))

				h, err := host.FindByIdentifier(db, "healthy-host-1")
				Expect(err).To(BeNil())

				hT := &hosttransition.HostTransition{}
				err = db.Get(hT, `SELECT * FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at ASC LIMIT 1;`, hs1.H.ID)
				Expect(err).To(BeNil())

				Expect(hT.PreviousState).To(Equal(sql.NullString{"born", true}))
				Expect(hT.State).To(Equal("running"))
				Expect(hT.CreatedAt).To(Equal(h.CreatedAt))
				Expect(hT.EndedAt.IsZero()).To(BeFalse())
			})

			It("should create a host volume if it does not exist in the DB", func() {
				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				vol, err := volume.Find(db, hs1.H.ID, "shared")
				Expect(err).To(BeNil())
				Expect(vol.Slug).To(Equal("shared"))
				Expect(vol.Name).To(Equal("Shared Volume"))
				Expect(vol.System).To(BeTrue())

				Expect(mockClient.ListVolumesCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeSlug).To(Equal("shared"))
			})

			It("should not create a host volume if it does exist in the DB", func() {
				sVol := &volume.Volume{
					HostID: hs1.H.ID,
					Slug:   "shared",
					Name:   "Shared Volume",
					System: true,
				}
				Expect(sVol.Insert(db)).To(BeNil())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(mockClient.ListVolumesCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeSlug).To(Equal("shared"))
			})

			It("should not create a host volume on the host if it already exists", func() {
				mockClient.ListVolumesReturn = []string{"shared"}

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(mockClient.ListVolumesCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeCalled).To(Equal(0))
			})

			It("should abort the transaction if the create volume failed", func() {
				mockClient.CreateVolumeError = errors.New("I am an error :(")

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				errT, errDesc := test.ErrorSlug(res.Body)
				Expect(errT).To(Equal("create_volume_failed"))
				Expect(errDesc).To(Equal("I am an error :("))

				h, err := host.FindByIdentifier(db, "healthy-host-1")
				Expect(err).To(BeNil())
				Expect(h.ProviderIdentifier).ToNot(Equal(sql.NullString{"i-c425e333", true}))

				Expect(mockClient.ListVolumesCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeCalled).To(Equal(1))
			})

			It("should publish a host updated event", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				res := doRequest(hs1.H.ID, &url.Values{"host_builder": []string{hbLog}})
				Expect(res.Code).To(Equal(http.StatusOK))

				evtCh := make(chan *controllers.Event)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventHostUpdated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timeout while getting shared volume created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventHostUpdated))
					evtData, ok := evt.Data.(map[string]*serializers.Host)
					Expect(ok).To(BeTrue())
					h, err := host.FindByIdentifier(db, "healthy-host-1")
					Expect(err).To(BeNil())
					Expect(evtData["host"]).To(Equal(serializers.SerializeHost(h)))
				}
			})

		})
	})

	Describe("parseHostbuilder()", func() {
		It("should return an error if the ID is missing", func() {
			h, err := amazon.ParseHostbuilder("ID: Region:bla")
			Expect(h).To(BeNil())
			Expect(err).ToNot(BeNil())
			t := getHttperrorType(err)
			Expect(t).To(Equal("missing_id"))
		})

		It("should return an error if the Region is missing", func() {
			h, err := amazon.ParseHostbuilder("ID:i-8cv345x2 Region: IPAddress:127.0.0.0.1")
			Expect(h).To(BeNil())
			Expect(err).ToNot(BeNil())
			t := getHttperrorType(err)
			Expect(t).To(Equal("missing_region"))
		})

		It("should return an error if the IP Address is missing", func() {
			h, err := amazon.ParseHostbuilder("ID:i-8cv345x2 Region:us-west-2 IPAddress: FreighterdPort:892")
			Expect(h).To(BeNil())
			Expect(err).ToNot(BeNil())
			t := getHttperrorType(err)
			Expect(t).To(Equal("missing_ip"))
		})

		It("should return an error if the IP Address is malformed", func() {
			h, err := amazon.ParseHostbuilder("ID:i-8cv345x2 Region:us-west-2 IPAddress:127.8.1 FreighterdPort:892")
			Expect(h).To(BeNil())
			Expect(err).ToNot(BeNil())
			t := getHttperrorType(err)
			Expect(t).To(Equal("missing_ip"))
		})

		It("should return an error if the port is missing", func() {
			h, err := amazon.ParseHostbuilder("ID:i-8cv345x2 Region:us-west-2 IPAddress:127.8.1.2 FreighterdPort: State:running")
			Expect(h).To(BeNil())
			Expect(err).ToNot(BeNil())
			t := getHttperrorType(err)
			Expect(t).To(Equal("missing_freighterd_port"))
		})

		It("should return an error if the state is missing", func() {
			h, err := amazon.ParseHostbuilder("ID:i-8cv345x2 Region:us-west-2 IPAddress:127.8.1.2 FreighterdPort:902 State:")
			Expect(h).To(BeNil())
			t := getHttperrorType(err)
			Expect(t).To(Equal("missing_state"))
		})

		It("should return a host struct", func() {
			h, err := amazon.ParseHostbuilder("ID:i-8cv345x2 Region:us-west-2 IPAddress:127.8.1.2 FreighterdPort:902 State:Running")
			Expect(err).To(BeNil())
			Expect(h).To(Equal(&host.Host{
				ProviderIdentifier: sql.NullString{"i-8cv345x2", true},
				RegionIdentifier:   sql.NullString{"us-west-2", true},
				IPAddress:          sql.NullString{"127.8.1.2", true},
				FreighterdPort:     sql.NullInt64{902, true},
				State:              "running",
			}))
		})
	})
})

func getHttperrorType(err error) string {
	hErr := err.(*httperror.Error)
	return hErr.Type
}
