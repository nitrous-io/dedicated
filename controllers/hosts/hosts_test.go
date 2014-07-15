package hosts_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/server"

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

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
}

func mockHostbuilderOK(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
	return &hostbuilder.Response{Status: "ok"}, nil
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
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/hosts", func() {
		var (
			u *user.User
			t *token.Token
		)

		doRequest := func(token string) {
			req, err := NewRequest("GET", "/api/v0/hosts", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when user has hosts", func() {
				BeforeEach(func() {
					hs, err := CreateHostStack(db, &HostStack{
						H: &host.Host{Slug: "foo"},
						U: u,
					})
					Expect(err).To(BeNil())

					_, err = CreateHostStack(db, &HostStack{
						H:  &host.Host{Slug: "bar"},
						U:  u,
						R:  hs.R,
						P:  hs.P,
						PT: hs.PT,
					})
					Expect(err).To(BeNil())
				})

				It("responds with the authenticated user's hosts", func() {
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusOK))
					var j map[string][]host.Host
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					hosts, ok := j["hosts"]
					Expect(ok).To(BeTrue())
					Expect(hosts).To(HaveLen(2))

					Expect(hosts[0].Slug).To(Equal("foo"))
					Expect(hosts[0].UserID).To(Equal(u.ID))

					Expect(hosts[1].Slug).To(Equal("bar"))
					Expect(hosts[1].UserID).To(Equal(u.ID))
				})
			})

			Context("when user has no hosts", func() {
				It("responds with an empty array", func() {
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusOK))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					hosts, ok := j["hosts"].([]interface{})
					Expect(ok).To(BeTrue())
					Expect(hosts).To(BeEmpty())
				})
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

	Describe("Create", func() {
		var (
			params *url.Values
			u      *user.User
			env    *middlewares.Env
			hs     *HostStack
		)

		doRequest := func() error {
			req, err := NewRequest("POST", "/api/v0/hosts", params)
			Expect(err).To(BeNil())

			return hosts.Create(env, svr, res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			env = &middlewares.Env{User: u}

			hs, err = CreateHostStack(db, &HostStack{
				R: &region.Region{
					Name: "use",
				},
				P: &plan.Plan{
					Name: "large",
				},
			})
			Expect(err).To(BeNil())
		})

		Context("when request does not specify a size", func() {
			It("returns error size required", func() {
				params = &url.Values{}
				params.Set("slug", "honeybadger")
				params.Set("size", "")
				params.Set("region", hs.R.Name)
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrSizeRequired))
			})
		})

		Context("when request does not specify a region", func() {
			It("returns error region required", func() {
				params = &url.Values{}
				params.Set("slug", "honeybadger")
				params.Set("size", hs.P.Name)
				params.Set("region", "")
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrRegionRequired))
			})
		})

		Context("when request not specify a slug", func() {
			It("returns error slug required", func() {
				params = &url.Values{}
				params.Set("slug", "")
				params.Set("size", hs.P.Name)
				params.Set("region", hs.R.Name)
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrSlugRequired))
			})
		})

		Context("when a matching plan cannot be found for the specified size", func() {
			It("returns error plan not found", func() {
				params = &url.Values{}
				params.Set("size", "doubledouble")
				params.Set("slug", "honeybadger")
				params.Set("region", hs.R.Name)
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrPlanNotFound))
			})
		})

		Context("when an invalid region is specified", func() {
			It("returns error region not found", func() {
				params = &url.Values{}
				params.Set("size", hs.P.Name)
				params.Set("slug", "honeybadger")
				params.Set("region", "antarctica")
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrRegionNotFound))
			})
		})

		Context("when no provider template found for the given plan and region", func() {
			var r2 *region.Region
			BeforeEach(func() {
				r2 = &region.Region{Name: "usw", Description: "US West"}
				err := r2.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns provider template not found error", func() {
				params = &url.Values{}
				params.Set("size", hs.P.Name)
				params.Set("slug", "honeybadger")
				params.Set("region", r2.Name)
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrProvidertemplateNotFound))
			})
		})

		Context("with valid parameters", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("slug", "honeybadger")
				params.Set("name", "honeybadger don't care")
				params.Set("size", hs.P.Name)
				params.Set("region", hs.R.Name)
			})

			Context("when hostbuilder successfully creates a host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					ch := func(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
						return &hostbuilder.Response{
							ID:             "abcd1234",
							IPAddress:      "0.0.0.0",
							Provider:       hs.PT.Name,
							FreighterdPort: uint16(65333),
						}, nil
					}
					hb.OverrideRequestHandler(ch)

					hosts.HostbuilderClient = hb

					err := doRequest()
					Expect(err).To(BeNil())
				})

				It("creates a host record for the user", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(1))

					h := hosts[0]
					Expect(h.RegionID).To(Equal(hs.R.ID))
					Expect(h.PlanID).To(Equal(hs.P.ID))
					Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

					Expect(h.Slug).To(Equal("honeybadger"))
					Expect(h.Name.String).To(Equal("honeybadger don't care"))

					Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
					Expect(h.State).To(Equal(host.StateRunning))
					Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
					Expect(h.Size.String).To(Equal(hs.P.Name))
					Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
					Expect(h.FreighterdPort.Int64).To(Equal(int64(65333)))
				})

				It("creates a keypair associated with that host", func() {
					hosts, _ := host.FindAllByUser(db, u)
					kp, err := keypairs.Get(db, "host", hosts[0].ID)
					Expect(err).To(BeNil())
					Expect(kp.OwnerID).To(Equal(hosts[0].ID))
					Expect(kp.OwnerType).To(Equal("host"))
				})

				It("responds with the created host and status created", func() {
					Expect(env.Status).To(Equal(http.StatusCreated))
					Expect(env.Data["host"]).NotTo(BeNil())
					Expect(env.Data["status"]).To(Equal("created"))
					h := env.Data["host"].(*host.Host)
					Expect(h.Slug).To(Equal("honeybadger"))
				})
			})

			Context("when hostbuilder fails to create the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					err := doRequest()
					Expect(err).To(MatchError("some error occurred"))
				})

				It("does not create a host for the user", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(0))
				})
			})
		})
	})

	Describe("Destroy", func() {
		var (
			env *middlewares.Env
			hs  *HostStack
			c   *container.Container
			err error
		)

		doRequest := func(slug string) error {
			req, err := NewRequest("DELETE", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"slug": slug}
			return hosts.Destroy(env, svr, res, req)
		}

		BeforeEach(func() {
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{Slug: "honeybadger", ProviderIdentifier: sql.NullString{"001", true}},
			})
			Expect(err).To(BeNil())

			env = &middlewares.Env{User: hs.U}

			c = &container.Container{
				CID:    "beefface",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Slug:   "beef-jerky",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("with an invalid host slug", func() {
			It("returns error host not found", func() {
				err = doRequest("somehost")
				Expect(err).To(MatchError(hosts.ErrHostNotFound))
			})
		})

		Context("with a valid host slug", func() {
			Context("when hostbuilder successfully deletes the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderOK)
					hosts.HostbuilderClient = hb
				})

				It("deletes the host record", func() {
					err = doRequest(hs.H.Slug)
					Expect(err).To(BeNil())

					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h).To(BeNil())
				})

				It("deletes all container records belonging to the host", func() {
					c2, err := container.FindByUserAndSlug(db, hs.U, c.Slug)
					Expect(err).To(BeNil())
					Expect(c2).To(Equal(c))

					err = doRequest(hs.H.Slug)
					Expect(err).To(BeNil())

					c2, err = container.FindByUserAndSlug(db, hs.U, c.Slug)
					Expect(err).To(BeNil())
					Expect(c2).To(BeNil())
				})

				It("responds with status OK", func() {
					err = doRequest(hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to delete the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(MatchError("some error occurred"))
				})

				It("does not delete the host", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h).NotTo(BeNil())
				})
			})
		})
	})

	Describe("Start", func() {
		var (
			env *middlewares.Env
			hs  *HostStack
			err error
		)

		doRequest := func(slug string) error {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/start", nil)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"slug": slug}

			return hosts.Start(env, svr, res, req)
		}

		BeforeEach(func() {
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:               "honeybadger",
					ProviderIdentifier: sql.NullString{"001", true},
					State:              host.StateStopped,
				},
			})
			Expect(err).To(BeNil())
			env = &middlewares.Env{User: hs.U}
		})

		Context("when host is not in the stopped state", func() {
			It("returns an invalid start state error", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.UpdateState(db, host.StateRestarting)
				err = doRequest(hs.H.Slug)
				Expect(err).To(MatchError(hosts.ErrHostStartInvalidState))
			})
		})

		Context("with an invalid host name", func() {
			It("returns error host not found", func() {
				err = doRequest("somehost")
				Expect(err).To(MatchError(hosts.ErrHostNotFound))
			})
		})

		Context("with a valid host name", func() {
			Context("when hostbuilder successfully starts the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderOK)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(BeNil())
				})

				It("updates the host state to running", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateRunning))
				})

				It("responds with status OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to start the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(MatchError("some error occurred"))
				})

				It("leaves host in intermediate state", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateStarting))
				})
			})
		})
	})

	Describe("Stop", func() {
		var (
			env *middlewares.Env
			hs  *HostStack
			err error
		)

		doRequest := func(slug string) error {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/stop", nil)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"slug": slug}

			return hosts.Stop(env, svr, res, req)
		}

		BeforeEach(func() {
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:               "honeybadger",
					ProviderIdentifier: sql.NullString{"001", true},
					State:              host.StateRunning,
				},
			})
			Expect(err).To(BeNil())
			env = &middlewares.Env{User: hs.U}
		})

		Context("when host is already stopped", func() {
			It("returns an invalid stop state error", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.UpdateState(db, host.StateStopped)
				err = doRequest(hs.H.Slug)
				Expect(err).To(MatchError(hosts.ErrHostStopInvalidState))
			})
		})

		Context("with an invalid host name", func() {
			It("returns error host not found", func() {
				err = doRequest("somehost")
				Expect(err).To(MatchError(hosts.ErrHostNotFound))
			})
		})

		Context("with a valid host name", func() {
			Context("when hostbuilder successfully stops the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderOK)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(BeNil())
				})

				It("updates the host state to stopped", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateStopped))
				})

				It("responds with status OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to stop the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(MatchError("some error occurred"))
				})

				It("leaves host in intermediate state", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateStopping))
				})
			})
		})
	})

	Describe("Restart", func() {
		var (
			env *middlewares.Env
			hs  *HostStack
			err error
		)

		doRequest := func(slug string) error {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/restart", nil)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"slug": slug}

			return hosts.Restart(env, svr, res, req)
		}

		BeforeEach(func() {
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:               "honeybadger",
					ProviderIdentifier: sql.NullString{"001", true},
					State:              host.StateRunning,
				},
			})
			Expect(err).To(BeNil())
			env = &middlewares.Env{User: hs.U}
		})

		Context("when host is not in the running state", func() {
			It("returns an invalid restart state error", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.UpdateState(db, host.StateStopped)
				err = doRequest(hs.H.Slug)
				Expect(err).To(MatchError(hosts.ErrHostRestartInvalidState))
			})
		})

		Context("with an invalid host name", func() {
			It("returns error host not found", func() {
				err = doRequest("somehost")
				Expect(err).To(MatchError(hosts.ErrHostNotFound))
			})
		})

		Context("with a valid host name", func() {
			Context("when hostbuilder successfully restarts the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderOK)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(BeNil())
				})

				It("updates the host state to running", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateRunning))
				})

				It("responds with status OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to restart the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					err = doRequest(hs.H.Slug)
					Expect(err).To(MatchError("some error occurred"))
				})

				It("leaves host in intermediate state", func() {
					h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateRestarting))
				})
			})
		})
	})
})
