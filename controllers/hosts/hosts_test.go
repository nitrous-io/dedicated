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
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"

	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/hostbuilder"
	"github.com/nitrous-io/nitrous/server"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
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
						H: &host.Host{Name: "foo"},
						U: u,
					})
					Expect(err).To(BeNil())

					_, err = CreateHostStack(db, &HostStack{
						H:  &host.Host{Name: "bar"},
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

					Expect(hosts[0].Name).To(Equal("foo"))
					Expect(hosts[0].UserID).To(Equal(u.ID))

					Expect(hosts[1].Name).To(Equal("bar"))
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
			hb     *hostbuilder.Hostbuilder
			env    *middlewares.Env
			hs     *HostStack
		)

		doRequest := func() error {
			req, err := NewRequest("POST", "/api/v0/hosts", params)
			Expect(err).To(BeNil())

			return hosts.Create(hb, env, svr, res, req)
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
				params.Set("size", "")
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrSizeRequired))
			})
		})

		Context("when a matching plan cannot be found for the specified size", func() {
			It("returns error plan not found", func() {
				params = &url.Values{}
				params.Set("size", "doubledouble")
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrPlanNotFound))
			})
		})

		Context("when an invalid region is specified", func() {
			It("returns error region not found", func() {
				params = &url.Values{}
				params.Set("size", hs.P.Name)
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
				params.Set("region", r2.Name)
				err := doRequest()
				Expect(err).To(MatchError(hosts.ErrProvidertemplateNotFound))
			})
		})

		Context("with valid parameters", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("name", "honeybadger")
				params.Set("size", hs.P.Name)
				params.Set("region", hs.R.Name)
			})

			Context("when hostbuilder successfully creates a host", func() {
				BeforeEach(func() {
					hb = hostbuilder.NewHostbuilder("www.example.com")
					ch := func(path string, provider string, config *map[string]string) (map[string]string, error) {
						return map[string]string{
							"id":         "abcd1234",
							"ip_address": "0.0.0.0",
							"provider":   hs.PT.Name,
						}, nil
					}
					hb.OverrideRequestHandler(ch)

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

					Expect(h.Name).To(Equal("honeybadger"))
					Expect(h.LongName.String).To(Equal("honeybadger"))

					Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
					Expect(h.State).To(Equal("created"))
					Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
					Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
				})

				It("responds with the created host", func() {
					Expect(env.Status).To(Equal(http.StatusCreated))
					Expect(env.Data["host"]).NotTo(BeNil())
					h := env.Data["host"].(*host.Host)
					Expect(h.Name).To(Equal("honeybadger"))
				})
			})

			Context("when hostbuilder fails to create the host", func() {
				BeforeEach(func() {
					hb = hostbuilder.NewHostbuilder("www.exmple.com")
					ch := func(path string, provider string, config *map[string]string) (map[string]string, error) {
						return nil, errors.New("some error occurred")
					}
					hb.OverrideRequestHandler(ch)

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
			hb  *hostbuilder.Hostbuilder
			env *middlewares.Env
			hs  *HostStack
			err error
		)

		doRequest := func(name string) error {
			req, err := NewRequest("DELETE", "/api/v0/hosts/"+name, nil)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"name": name}

			return hosts.Destroy(hb, env, svr, res, req)
		}

		BeforeEach(func() {
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{Name: "honeybadger", ProviderIdentifier: sql.NullString{"001", true}},
			})
			Expect(err).To(BeNil())
			env = &middlewares.Env{User: hs.U}
		})

		Context("with an invalid host name", func() {
			It("returns error host not found", func() {
				err = doRequest("somehost")
				Expect(err).To(MatchError(hosts.ErrHostNotFound))
			})
		})

		Context("with a valid host name", func() {
			Context("when hostbuilder successfully deletes the host", func() {
				BeforeEach(func() {
					hb = hostbuilder.NewHostbuilder("www.exmple.com")
					ch := func(path string, provider string, config *map[string]string) (map[string]string, error) {
						return map[string]string{"status": "ok"}, nil
					}
					hb.OverrideRequestHandler(ch)

					err = doRequest(hs.H.Name)
					Expect(err).To(BeNil())
				})

				It("deletes the host record", func() {
					h, err := host.FindByName(db, hs.H.Name)
					Expect(err).To(BeNil())
					Expect(h).To(BeNil())
				})

				It("responds with status OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to delete the host", func() {
				BeforeEach(func() {
					hb = hostbuilder.NewHostbuilder("www.exmple.com")
					ch := func(path string, provider string, config *map[string]string) (map[string]string, error) {
						return nil, errors.New("some error occurred")
					}
					hb.OverrideRequestHandler(ch)

					err = doRequest(hs.H.Name)
					Expect(err).To(MatchError("some error occurred"))
				})

				It("does not delete the host", func() {
					h, err := host.FindByName(db, hs.H.Name)
					Expect(err).To(BeNil())
					Expect(h).NotTo(BeNil())
				})
			})
		})
	})

	Describe("Start", func() {
		var (
			hb  *hostbuilder.Hostbuilder
			env *middlewares.Env
			hs  *HostStack
			err error
		)

		doRequest := func(name string) error {
			req, err := NewRequest("POST", "/api/v0/hosts/"+name+"/start", nil)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"name": name}

			return hosts.Start(hb, env, svr, res, req)
		}

		BeforeEach(func() {
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Name:               "honeybadger",
					ProviderIdentifier: sql.NullString{"001", true},
					State:              "stopped",
				},
			})
			Expect(err).To(BeNil())
			env = &middlewares.Env{User: hs.U}
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
					hb = hostbuilder.NewHostbuilder("www.example.com")
					ch := func(path string, provider string, config *map[string]string) (map[string]string, error) {
						return map[string]string{"status": "ok"}, nil
					}
					hb.OverrideRequestHandler(ch)

					err = doRequest(hs.H.Name)
					Expect(err).To(BeNil())
				})

				It("updates the host state to start", func() {
					h, err := host.FindByName(db, hs.H.Name)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("running"))
				})

				It("responds with status OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to start the host", func() {
				BeforeEach(func() {
					hb = hostbuilder.NewHostbuilder("www.example.com")
					ch := func(path string, provider string, config *map[string]string) (map[string]string, error) {
						return nil, errors.New("some error occurred")
					}
					hb.OverrideRequestHandler(ch)

					err = doRequest(hs.H.Name)
					Expect(err).To(MatchError("some error occurred"))
				})

				It("does not change the host state", func() {
					h, err := host.FindByName(db, hs.H.Name)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("stopped"))
				})
			})
		})
	})
})
