package admin_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/admin")
}

var _ = Describe("controllers/admin", func() {
	var (
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
		svr := server.NewServer("", db, logger, nil)
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

	Describe("POST /api/v0/admin/switch_user", func() {
		doRequest := func(t *token.Token, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "POST", "/api/v0/admin/switch_user", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				params := &url.Values{}
				params.Set("login", "bar")

				res := doRequest(t, params)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("responds with a 404 Not Found, if there's no matching user for the given login'", func() {
				params := &url.Values{}
				params.Set("login", "bar")

				res := doRequest(t, params)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			Context("when there's a matching user for the given login", func() {
				var u2 *user.User

				BeforeEach(func() {
					// Create a different user that we'll try to switch to.
					u2 = &user.User{
						Username: "bar",
						Password: "123456",
						Name:     "bar",
						Email:    "bar@example.com",
					}
					err := test.WrapInTransaction(db, u2.Insert)
					Expect(err).To(BeNil())
				})

				It("adds a new token for the matching user", func() {
					params := &url.Values{}
					params.Set("login", "bar")
					doRequest(t, params)

					t, err := token.FindByUser(db, u2.ID)
					Expect(err).To(BeNil())
					Expect(t).NotTo(BeNil())
					Expect(t.UserID).To(Equal(u2.ID))
					Expect(t.Token).NotTo(BeNil())
					Expect(t.CreatedAt).NotTo(BeNil())
				})

				It("responds with the token added for the matching user", func() {
					params := &url.Values{}
					params.Set("login", "bar")
					res := doRequest(t, params)

					Expect(res.Code).To(Equal(http.StatusOK))

					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					t, err := token.FindByUser(db, u2.ID)
					Expect(err).To(BeNil())
					Expect(j["token_type"]).To(Equal("Bearer"))
					Expect(j["access_token"]).To(Equal(t.Token))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/admin/switch_user"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/admin/hosts", func() {
		doRequest := func(t *token.Token, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/admin/hosts", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(t, nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			var (
				hs1, hs2, hs3, hs4 *test.HostStack
				provider           = "digitalocean"
			)

			BeforeEach(func() {
				var err error

				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				hs1, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "digitalocean-1", Valid: true},
						State:              host.StateRunning,
						Slug:               "ok-host-1",
					},
					PT: &providertemplate.ProviderTemplate{
						Provider: provider,
						Name:     "do-sgp-512mb",
					},
					U: u,
				})
				Expect(err).To(BeNil())

				hs2, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "digitalocean-2", Valid: true},
						State:              host.StateOrphan,
						Slug:               "non-running-host-1",
					},
					U:  u,
					PT: hs1.PT,
					P:  hs1.P,
					R:  hs1.R,
				})
				Expect(err).To(BeNil())

				// Create some pre-provisioned hosts.
				u2 := &user.User{
					Username: "pre-provisioner",
					Email:    "pp@example.com",
					Password: "pp1234",
				}
				hs3, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "digitalocean-4", Valid: true},
						Slug:               "ok-host-2",
						Preprovisioned:     true,
						State:              host.StateRunning,
					},
					U:  u2,
					PT: hs1.PT,
					P:  hs1.P,
					R:  hs1.R,
				})
				Expect(err).To(BeNil())
				hs4, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "digitalocean-5", Valid: true},
						Slug:               "terminated-host-1",
						Preprovisioned:     true,
						State:              host.StateRunning,
					},
					U:  u2,
					PT: hs1.PT,
					P:  hs1.P,
					R:  hs1.R,
				})
				Expect(err).To(BeNil())
				err = hs4.H.Delete(db) // Terminate this host.
				Expect(err).To(BeNil())

				// Fake hostbuilderclient response.
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt := &test.FakeRoundTripper{
					Message: fmt.Sprintf(`{
						"hosts": [
							{
								"provider": "%s",
								"id": "digitalocean-1",
								"name": "somehost-2343",
								"name": "create-test-cj-3232-232",
								"ip_address": "1.2.3.4",
								"freighterd_port": 65333,
								"state": "Running"
							},
							{
								"provider": "%s",
								"id": "digitalocean-666",
								"name": "create-test-3232",
								"ip_address": "1.6.6.6",
								"freighterd_port": 65334,
								"state": "Stopped"
							},
							{
								"provider": "%s",
								"id": "digitalocean-831",
								"name": "create-test-ams3-10418105-1423059042",
								"ip_address": "1.2.3.1",
								"freighterd_port": 65335,
								"state": "Running"
							}
						]
					}`, provider, provider, provider),
					Status: http.StatusOK,
				}
				hb.Client.Client.Transport = rt
				admin.HostbuilderClient = hb
			})

			It("responds with hosts that are in the database and in Hostbuilder", func() {
				p := &url.Values{}
				p.Set("provider", provider)
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					DBHosts   []serializers.AdminHost `json:"db_hosts"`
					LiveHosts []struct {
						Provider       string `json:"provider"`
						ID             string `json:"id"`
						IPAddress      string `json:"ip_address"`
						FreighterdPort uint16 `json:"freighterd_port"`
						State          string `json:"state"`
					} `json:"live_hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.DBHosts).To(HaveLen(3))

				Expect(j.DBHosts[0].ID).To(Equal(hs1.H.ID))
				Expect(j.DBHosts[0].Preprovisioned).To(BeFalse())
				Expect(j.DBHosts[1].ID).To(Equal(hs2.H.ID))
				Expect(j.DBHosts[1].Preprovisioned).To(BeFalse())
				Expect(j.DBHosts[2].ID).To(Equal(hs3.H.ID))
				Expect(j.DBHosts[2].Preprovisioned).To(BeTrue())

				Expect(j.LiveHosts).To(HaveLen(2))

				Expect(j.LiveHosts[0].Provider).To(Equal(provider))
				Expect(j.LiveHosts[0].ID).To(Equal("digitalocean-1"))
				Expect(j.LiveHosts[0].IPAddress).To(Equal("1.2.3.4"))
				Expect(j.LiveHosts[0].FreighterdPort).To(Equal(uint16(65333)))
				Expect(j.LiveHosts[0].State).To(Equal("Running"))

				Expect(j.LiveHosts[1].Provider).To(Equal(provider))
				Expect(j.LiveHosts[1].ID).To(Equal("digitalocean-666"))
				Expect(j.LiveHosts[1].IPAddress).To(Equal("1.6.6.6"))
				Expect(j.LiveHosts[1].FreighterdPort).To(Equal(uint16(65334)))
				Expect(j.LiveHosts[1].State).To(Equal("Stopped"))
			})

			It("responds with HTTP 400 Bad Request when provider is not given", func() {
				res := doRequest(t, nil)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("provider is required"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/admin/hosts"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/admin/terminate_unaccounted_host", func() {
		doRequest := func(t *token.Token, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/admin/terminate_unaccounted_host", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(t, nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			var (
				hs1, hs2 *test.HostStack
				provider = "digitalocean"
				rt       *test.FakeRoundTripper
			)

			BeforeEach(func() {
				var err error

				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				// Create hosts owned by another user.
				u2 := &user.User{
					Username: "pre-provisioner",
					Email:    "pp@example.com",
					Password: "pp1234",
				}
				hs1, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "digitalocean-1", Valid: true},
						Slug:               "ok-host",
						Preprovisioned:     true,
						State:              host.StateRunning,
					},
					PT: &providertemplate.ProviderTemplate{
						Provider: provider,
						Name:     "do-sgp-512mb",
					},
					U: u2,
				})
				Expect(err).To(BeNil())
				hs2, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						ProviderIdentifier: sql.NullString{String: "digitalocean-2", Valid: true},
						Slug:               "orphan-host",
						Preprovisioned:     true,
						State:              host.StateOrphan,
					},
					U:  u2,
					PT: hs1.PT,
					P:  hs1.P,
					R:  hs1.R,
				})
				Expect(err).To(BeNil())
				err = hs2.H.Delete(db) // Terminate this host.
				Expect(err).To(BeNil())

				// Fake hostbuilderclient response.
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt = &test.FakeRoundTripper{
					Message: `{"status": "sounds_good"}`,
					Status:  http.StatusOK,
				}
				hb.Client.Client.Transport = rt
				admin.HostbuilderClient = hb
			})

			It("responds with HTTP 400 Bad Request when provider is not given", func() {
				res := doRequest(t, nil)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("provider is required"))
			})

			It("responds with HTTP 400 Bad Request when provider_identifier is not given", func() {
				p := &url.Values{}
				p.Set("provider", provider)
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("provider_identifier is required"))
			})

			It("should not allow for terminating hosts that exist in the database", func() {
				p := &url.Values{}
				p.Set("provider", provider)
				p.Set("provider_identifier", "digitalocean-1")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("host is known in DB, cannot terminate directly"))
			})

			It("should terminate hosts that exist in the database marked as terminated", func() {
				p := &url.Values{}
				p.Set("provider", provider)
				p.Set("provider_identifier", "digitalocean-2")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]string{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j["status"]).To(Equal("sounds_good"))
			})

			It("should terminate the host on the provider", func() {
				p := &url.Values{}
				p.Set("provider", provider)
				p.Set("provider_identifier", "digitalocean-3")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]string{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j["status"]).To(Equal("sounds_good"))

				req := rt.Requests[0]
				Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())
			})
		})
	})

	Describe("PUT /api/v0/admin/host/{slug}", func() {
		var (
			hs  *test.HostStack
			err error
		)

		doRequest := func(t *token.Token, slug string, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "PUT", "/api/v0/admin/host/"+slug, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			hs, err = test.CreateHostStack(db, &test.HostStack{})
			Expect(err).To(BeNil())
		})

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(t, hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("responds with a status 400 if no freighterd version is provided", func() {
				res := doRequest(t, hs.H.Slug, &url.Values{})

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("freighterd version required"))
			})

			It("responds with a status 404 if host not found by the slug", func() {
				p := &url.Values{}
				p.Set("freighterd_version", "1.0.0")
				res := doRequest(t, "non-exsistent-host", p)

				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("host_not_found"))
				Expect(errDesc).To(Equal("host does not exist"))
			})

			It("responds with updated host on a successful request", func() {
				p := &url.Values{}
				p.Set("freighterd_version", "1.0.0")
				res := doRequest(t, hs.H.Slug, p)

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.FreighterdVersion).To(Equal("1.0.0"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/admin/host/my-host"
				authTokenInputs.Method = "PUT"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
