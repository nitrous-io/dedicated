package amazon_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/amazon"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
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
	RunSpecs(t, "controllers/amazon")
}

var _ = Describe("controllers/amazon", func() {
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

	Describe("GET /api/v0/admin/amazon/hosts", func() {
		doRequest := func(t *token.Token, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/admin/amazon/hosts", params)
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
				hs1, hs2, hs3 *test.HostStack
				provider      = "amazon"
			)

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
						Provider: provider,
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
						State:              host.StateUpgrading,
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
				p := &url.Values{}
				p.Set("provider", provider)
				res := doRequest(t, p)

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
				Expect(hosts.InconsistentHosts[0].DBHost.State).To(Equal("upgrading"))
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
})
