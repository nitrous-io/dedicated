package hosts_test

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
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"

	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	// shorten key length so tests run faster
	hosts.KeyLen = 512
	keypairs.KeyLen = 512
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
}

type (
	errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
)

func mockHostbuilderOK(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
	return &hostbuilder.Response{Status: "ok", IPAddress: "127.0.0.1", FreighterdPort: 65333}, nil
}
func mockHostbuilderError(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
	return &hostbuilder.Response{}, errors.New("some error occurred")
}

var _ = Describe("controllers/hosts", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	unmarshalErrorResponse := func() (error, *errorResponse) {
		errRes := &errorResponse{}
		err := json.Unmarshal(res.Body.Bytes(), errRes)
		return err, errRes
	}

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

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

	Describe("GET /api/v0/hosts", func() {
		doRequest := func() {
			req, err := NewRequest("GET", "/api/v0/hosts", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when user has hosts", func() {
			var hs1, hs2 *HostStack

			BeforeEach(func() {
				var err error
				hs1, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:           "foo",
						IPAddress:      sql.NullString{"1.2.3.4", true},
						FreighterdPort: sql.NullInt64{64000, true},
						Size:           sql.NullString{"micro", true},
						State:          host.StateRunning,
					},
					U: u,
				})
				Expect(err).To(BeNil())

				hs2, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:           "bar",
						IPAddress:      sql.NullString{"2.4.6.8", true},
						FreighterdPort: sql.NullInt64{64001, true},
						Size:           sql.NullString{"large", true},
						State:          host.StateRunning,
					},
					U: u,
				})
				Expect(err).To(BeNil())
			})

			It("responds with the authenticated user's hosts", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Hosts []*serializers.Host `json:"hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Hosts).To(HaveLen(2))
				Expect(j.Hosts[0].ID).To(Equal(hs1.H.ID))
				Expect(j.Hosts[0].UserID).To(Equal(hs1.U.ID))
				Expect(j.Hosts[0].Slug).To(Equal("foo"))
				Expect(j.Hosts[0].IPAddress).To(Equal("1.2.3.4"))
				Expect(j.Hosts[0].FreighterdPort).To(Equal(int64(64000)))
				Expect(j.Hosts[0].Size).To(Equal("micro"))
				Expect(j.Hosts[0].State).To(Equal(host.StateRunning))
				Expect(j.Hosts[0].Hostname).To(Equal("foo.nitrouspro.com"))
				Expect(j.Hosts[0].CreatedAt).NotTo(BeNil())

				Expect(j.Hosts[1].ID).To(Equal(hs2.H.ID))
				Expect(j.Hosts[1].UserID).To(Equal(hs2.U.ID))
				Expect(j.Hosts[1].Slug).To(Equal("bar"))
				Expect(j.Hosts[1].IPAddress).To(Equal("2.4.6.8"))
				Expect(j.Hosts[1].FreighterdPort).To(Equal(int64(64001)))
				Expect(j.Hosts[1].Size).To(Equal("large"))
				Expect(j.Hosts[1].State).To(Equal(host.StateRunning))
				Expect(j.Hosts[1].Hostname).To(Equal("bar.nitrouspro.com"))
				Expect(j.Hosts[1].CreatedAt).NotTo(BeNil())
			})
		})

		Context("when user has no hosts", func() {
			It("responds with an empty array", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Hosts []*serializers.Host `json:"hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Hosts).To(BeEmpty())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts", func() {
		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				R: &region.Region{Slug: "use"},
				P: &plan.Plan{Slug: "large"},
			})
			Expect(err).To(BeNil())
		})

		type overrides map[string]string
		doRequest := func(o overrides) {
			var ok bool

			if _, ok = o["slug"]; !ok {
				o["slug"] = "honeybadger"
			}

			if _, ok = o["size"]; !ok {
				o["size"] = hs.P.Slug
			}

			if _, ok = o["region"]; !ok {
				o["region"] = hs.R.Slug
			}

			if _, ok = o["name"]; !ok {
				o["name"] = "honeybadger don't care"
			}

			p := &url.Values{}
			for k, v := range o {
				p.Set(k, v)
			}

			req, err := NewRequest("POST", "/api/v0/hosts", p)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		// tests for required params
		for _, param := range []string{"size", "region", "slug"} {
			p := param

			Context("when request does not specify a "+p, func() {
				It("responds with "+p+" required", func() {
					doRequest(overrides{p: ""})

					Expect(res.Code).To(Equal(http.StatusBadRequest))

					err, j := unmarshalErrorResponse()
					Expect(err).To(BeNil())

					Expect(j.Error).To(Equal("invalid_request"))
					Expect(j.ErrorDescription).To(Equal(p + " is required"))
				})
			})
		}

		Context("when a matching plan cannot be found for the specified size", func() {
			It("responds with plan not found", func() {
				doRequest(overrides{"size": "doubledouble"})

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())

				Expect(j.Error).To(Equal("invalid_request"))
				Expect(j.ErrorDescription).To(Equal("no matching plan found for the given size"))
			})
		})

		Context("when an invalid region is specified", func() {
			It("responds with region not found", func() {
				doRequest(overrides{"region": "antarctica"})

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())

				Expect(j.Error).To(Equal("invalid_request"))
				Expect(j.ErrorDescription).To(Equal("no matching region found for the given name"))
			})
		})

		Context("when no provider template found for the given plan and region", func() {
			It("responds with provider template not found error", func() {
				var r2 *region.Region

				r2 = &region.Region{Slug: "usw", Name: "US West"}
				err := r2.Insert(db)
				Expect(err).To(BeNil())

				doRequest(overrides{"region": r2.Slug})

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())

				Expect(j.Error).To(Equal("invalid_request"))
				Expect(j.ErrorDescription).To(Equal("no matching templates found for the given plan and regions combination"))
			})
		})

		Context("with valid parameters", func() {
			Context("when hostbuilder successfully creates a host", func() {
				var config hostbuilderclient.Config

				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					ch := func(path string, provider string, conf *hostbuilderclient.Config) (*hostbuilder.Response, error) {
						config = *conf

						return &hostbuilder.Response{
							ID:             "abcd1234",
							IPAddress:      "0.0.0.0",
							Provider:       hs.PT.Name,
							FreighterdPort: uint16(65333),
						}, nil
					}
					hb.OverrideRequestHandler(ch)

					hosts.HostbuilderClient = hb
				})

				It("creates a host record for the user", func() {
					doRequest(overrides{})
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))

					h := hosts[0]
					Expect(h.RegionID).To(Equal(hs.R.ID))
					Expect(h.PlanID).To(Equal(hs.P.ID))
					Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

					Expect(h.Slug).To(Equal("honeybadger"))
					Expect(h.Name.String).To(Equal("honeybadger don't care"))

					Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
					Expect(h.State).To(Equal(host.StateRunning))
					Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
					Expect(h.Size.String).To(Equal(hs.P.Slug))
					Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
					Expect(h.FreighterdPort.Int64).To(Equal(int64(65333)))

					Expect(config["name"]).To(Equal("honeybadger"))
				})

				It("creates a keypair associated with that host", func() {
					doRequest(overrides{})
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))

					kp, err := keypairs.Get(db, hosts[0].ID)
					Expect(err).To(BeNil())
					Expect(kp.HostID).To(Equal(hosts[0].ID))

					Expect(config["public_keys"].([]string)[0]).To(Equal(kp.PublicKey))
				})

				It("creates a shared secret for the host", func() {
					doRequest(overrides{})
					hosts, _ := host.FindAllByUser(db, u)

					Expect(hosts[0].Secret).NotTo(BeEmpty())
					Expect(config["secret"]).To(Equal(hosts[0].Secret))
				})

				It("creates a cert associated with that host", func() {
					doRequest(overrides{})
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					crt, err := cert.Find(db, hosts[0].ID)
					Expect(err).To(BeNil())
					Expect(crt.HostID).To(Equal(hosts[0].ID))
				})

				It("responds with the created host and status created", func() {
					doRequest(overrides{})
					Expect(res.Code).To(Equal(http.StatusCreated))

					var j struct {
						Host *serializers.Host `json:"host"`
					}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())

					hosts, _ := host.FindAllByUser(db, u)
					Expect(j.Host.ID).To(Equal(hosts[0].ID))
					Expect(j.Host.UserID).To(Equal(u.ID))
					Expect(j.Host.RegionID).To(Equal(hs.R.ID))
					Expect(j.Host.PlanID).To(Equal(hs.P.ID))
					Expect(j.Host.Slug).To(Equal("honeybadger"))
					Expect(j.Host.IPAddress).To(Equal("0.0.0.0"))
					Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
					Expect(j.Host.Size).To(Equal(hs.P.Slug))
					Expect(j.Host.State).To(Equal(host.StateRunning))
					Expect(j.Host.Hostname).To(Equal("honeybadger.nitrouspro.com"))
					Expect(j.Host.CreatedAt).NotTo(BeNil())
					Expect(j.Host.UpdatedAt).NotTo(BeNil())
					Expect(j.Host.TerminatedAt).To(BeNil())
				})

				It("publishes a host.created event with the created host", func(done Done) {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					doRequest(overrides{})

					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					h := hosts[0]
					Expect(<-ch).To(Equal(&controllers.Event{
						Name: "host.created",
						Data: serializers.SerializeHost(&h),
					}))
					close(done)
				}, 3) // increase timeout to allow goroutine to finish

				It("converts the 'slug' form value to lowercase", func() {
					doRequest(overrides{"slug": "HoneyBADGER"})

					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					Expect(hosts[0].Slug).To(Equal("honeybadger"))
				})
			})

			Context("when trying to create a host with an existing slug", func() {
				BeforeEach(func() {
					_, err := CreateHostStack(db, &HostStack{
						H: &host.Host{Slug: "honeybadger"},
						U: u,
					})
					Expect(err).To(BeNil())

					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))

					doRequest(overrides{"slug": "honeybadger"})
				})

				It("does not create a host", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
				})

				It("responds with HTTP 400 Bad Request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					errType, _ := j["error"].(string)
					Expect(errType).To(Equal("validation_failed"))
					errDesc, _ := j["error_description"].(string)
					Expect(errDesc).To(Equal("slug is taken"))
				})
			})

			Context("when hostbuilder fails to create the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					doRequest(overrides{})
				})

				It("creates a host in the orphan state for the user", func() {
					h, err := host.Find(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.Slug).To(Equal("honeybadger"))
					Expect(h.State).To(Equal(host.StateOrphan))
				})

				It("responds with a internal server error", func() {
					Expect(res.Code).To(Equal(http.StatusInternalServerError))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/hosts/{slug}", func() {
		var hs *HostStack

		doRequest := func(slug string) {
			req, err := NewRequest("GET", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:           "foo",
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					Size:           sql.NullString{"large", true},
					State:          host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			UnstubFreighterFn()
		})

		It("responds with host info from the database", func() {
			doRequest("foo")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
			Expect(j.Host.UserID).To(Equal(hs.U.ID))
			Expect(j.Host.Slug).To(Equal("foo"))
			Expect(j.Host.IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Host.FreighterdPort).To(Equal(int64(64000)))
			Expect(j.Host.Size).To(Equal("large"))
			Expect(j.Host.State).To(Equal(host.StateRunning))
			Expect(j.Host.Hostname).To(Equal("foo.nitrouspro.com"))
			Expect(j.Host.RegionID).To(Equal(hs.R.ID))
			Expect(j.Host.PlanID).To(Equal(hs.P.ID))
			Expect(j.Host.CreatedAt).NotTo(BeNil())
			Expect(j.Host.UpdatedAt).NotTo(BeNil())
			Expect(j.Host.TerminatedAt).To(BeNil())
		})

		It("responds with host info even when called with the host ID", func() {
			doRequest(fmt.Sprintf("%d", hs.H.ID))
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("PUT /api/v0/hosts/{slug}", func() {
		var hs *HostStack

		doRequest := func(params *url.Values) {
			req, err := NewAuthedFormRequest(t, "PUT", "/api/v0/hosts/foo", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:           "foo",
					Name:           sql.NullString{"foohost", true},
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					Size:           sql.NullString{"large", true},
					State:          host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when name field is present and non-empty", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("name", "GhostHost")
				doRequest(params)
			})

			It("updates name", func() {
				updated, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal("GhostHost"))
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *hs.H.UpdatedAt))
			})
		})

		Context("when name field is present and empty", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("name", "")
				doRequest(params)
			})

			It("nullifies name", func() {
				updated, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal(""))
				Expect(updated.Name.Valid).To(BeFalse())
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *hs.H.UpdatedAt))
			})
		})

		Context("when unused fields are given", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("slug", "younochange")
				doRequest(params)
			})

			It("does not update fields", func() {
				updated, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal("foohost"))
				Expect(updated.Slug).To(Equal("foo"))
				Expect(updated.UpdatedAt).To(Equal(hs.H.UpdatedAt))
			})
		})

		It("publishes a host.updated event with the updated host", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			params := &url.Values{}
			params.Set("name", "GhostHost")
			doRequest(params)

			updated, err := host.FindByID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(<-ch).To(Equal(&controllers.Event{
				Name: "host.updated",
				Data: serializers.SerializeHost(updated),
			}))
			close(done)
		}, 3) // add longer timeout to allow goroutine to finish
	})

	Describe("GET /api/v0/hosts/{slug}/stats", func() {
		var (
			mockClient *freighterclient.MockClient
			hs         *HostStack
		)

		doRequest := func(period string) {
			var params *url.Values
			if period != "" {
				params = &url.Values{"period": []string{period}}
			}
			req, err := NewRequest("GET", "/api/v0/hosts/foo/stats", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error

			mockClient = &freighterclient.MockClient{}
			mockClient.HostInfoReturn = &freighterclient.HostInfo{
				Uptime:       30,
				MemoryUsage:  12345678,
				MemoryTotal:  17179869184,
				StorageUsage: 2000000000,
				StorageTotal: 1000000000000,
				NumCPU:       2,
				PerCPUUsage:  []float64{0.5, 30},
			}
			StubDefaultFreighterFn(mockClient)

			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:      "foo",
					IPAddress: sql.NullString{"1.2.3.4", true},
					State:     host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			UnstubFreighterFn()
		})

		It("calls the Freighter client to get host info", func() {
			doRequest("")
			Expect(mockClient.HostInfoCalled).To(Equal(1))
			Expect(mockClient.HostInfoPeriod).To(Equal(100 * time.Millisecond)) // Default is 100ms if not specified.

			doRequest("2s")
			Expect(mockClient.HostInfoCalled).To(Equal(2))
			Expect(mockClient.HostInfoPeriod).To(Equal(2 * time.Second))
		})

		It("responds with host stats from the Freighter API", func() {
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Stats *serializers.HostInfo `json:"stats"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Stats.Uptime).To(Equal(uint64(30)))
			Expect(j.Stats.MemoryUsage).To(Equal(uint64(12345678)))
			Expect(j.Stats.MemoryTotal).To(Equal(uint64(17179869184)))
			Expect(j.Stats.StorageUsage).To(Equal(uint64(2000000000)))
			Expect(j.Stats.StorageTotal).To(Equal(uint64(1000000000000)))
			Expect(j.Stats.NumCPU).To(Equal(2))
			Expect(j.Stats.PerCPUUsage).To(Equal([]float64{0.5, 30}))
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())
				Expect(j.Error).To(Equal("invalid_state"))
				Expect(j.ErrorDescription).To(Equal("host is not running"))
			})
		})

		Context("when host does not have an IP address", func() {
			BeforeEach(func() {
				hs.H.IPAddress = sql.NullString{}
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())
				Expect(j.Error).To(Equal("invalid_state"))
				Expect(j.ErrorDescription).To(Equal("host is not running"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/stats"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}", func() {
		var c *container.Container

		doRequest := func(slug string) {
			req, err := NewRequest("DELETE", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{Slug: "honeybadger", ProviderIdentifier: sql.NullString{"001", true}},
				U: u,
			})
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "beefface",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Slug:   "beef-jerky",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when hostbuilder successfully deletes the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)
				hosts.HostbuilderClient = hb
			})

			It("deletes the host record", func() {
				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})

			It("deletes all container records belonging to the host", func() {
				c2, err := container.Find(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(Equal(c))

				doRequest(hs.H.Slug)

				c2, err = container.Find(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(BeNil())
			})

			It("deletes keypairs associated to the host", func() {
				priv, _ := keypairs.GenerateKey()
				tx, _ := db.Beginx()
				keypairs.Create(tx, priv, "aes_key", hs.H.ID)
				tx.Commit()

				doRequest(hs.H.Slug)

				_, err := keypairs.Get(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "host's keypairs should be removed")
			})

			It("deletes certs associated to the host", func() {
				tx, _ := db.Beginx()
				crt := &cert.Cert{
					ServerCert: "servercert",
					ServerKey:  "serverkey",
				}
				Expect(crt.Insert(tx, hs.H.ID)).To(BeNil())
				tx.Commit()

				doRequest(hs.H.Slug)
				_, err := cert.Find(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "failed to remove host's certificates")
			})

			It("responds with status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal("honeybadger"))
				Expect(j.Host.IPAddress).To(Equal(""))
				Expect(j.Host.FreighterdPort).To(Equal(int64(0)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateTerminated))
				Expect(j.Host.Hostname).To(Equal("honeybadger.nitrouspro.com"))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.Stats).To(BeNil())
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).NotTo(BeNil())
			})

			It("publishes a host.destroyed event with the destroyed host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				deleted, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: "host.destroyed",
					Data: serializers.SerializeHost(deleted),
				}))
				close(done)
			}, 3) // add longer timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)
				hosts.HostbuilderClient = hb
			})

			It("does not delete the host", func() {
				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("when host is provider identifier is nil", func() {
			It("should not make a request to hostbuilder", func() {
				var hostbuilderCalled bool

				hs.H.ProviderIdentifier = sql.NullString{String: "", Valid: false}
				hs.H.Update(db)

				// in this context, hostbuilder should not be called
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(func(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
					hostbuilderCalled = true
					return &hostbuilder.Response{}, errors.New("some error occurred")
				})
				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
				Expect(hostbuilderCalled).NotTo(BeTrue())
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/start", func() {
		doRequest := func(slug string) {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/start", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{State: host.StateStopped},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is not in the stopped state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateRestarting
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())

				Expect(j.Error).To(Equal("invalid_state"))
				Expect(j.ErrorDescription).To(Equal("host is not stopped"))
			})
		})

		Context("when hostbuilder successfully starts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb
			})

			It("updates the host state to running", func() {
				doRequest(hs.H.Slug)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.Stats).To(BeNil())
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host.started event with the host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: "host.started",
					Data: serializers.SerializeHost(h),
				}))
				close(done)
			}, 3) // add longer timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStarting))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/start"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/stop", func() {
		doRequest := func(slug string) {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/stop", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{State: host.StateRunning},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is already stopped", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())

				Expect(j.Error).To(Equal("invalid_state"))
				Expect(j.ErrorDescription).To(Equal("host is not running"))
			})
		})

		Context("when hostbuilder successfully stops the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb
			})

			It("updates the host state to stopped", func() {
				doRequest(hs.H.Slug)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopped))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateStopped))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.Stats).To(BeNil())
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host.stopped event with the host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: "host.stopped",
					Data: serializers.SerializeHost(h),
				}))
				close(done)
			}, 3) // add longer timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopping))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/stop"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/restart", func() {
		doRequest := func(slug string) {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/restart", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{State: host.StateRunning},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is not in the running state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				err, j := unmarshalErrorResponse()
				Expect(err).To(BeNil())

				Expect(j.Error).To(Equal("invalid_state"))
				Expect(j.ErrorDescription).To(Equal("host is not running"))
			})
		})

		Context("when hostbuilder successfully restarts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb
			})

			It("updates the host state to running", func() {
				doRequest(hs.H.Slug)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.Stats).To(BeNil())
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host.restarted event with the host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: "host.restarted",
					Data: serializers.SerializeHost(h),
				}))
				close(done)
			}, 3) // increase timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRestarting))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/restart"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
