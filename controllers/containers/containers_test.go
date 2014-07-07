package containers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/containers")
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
		r  *region.Region
		p  *plan.Plan
		pt *providertemplate.ProviderTemplate
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

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
		err = r.Insert(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
		err = p.Insert(db)
		Expect(err).To(BeNil())

		pt = &providertemplate.ProviderTemplate{
			RegionID: r.ID,
			PlanID:   p.ID,
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
		err = pt.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/hosts/{host}/containers", func() {
		var h *host.Host

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Name:               "foo-host",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(token, host string) {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/hosts/%s/containers", host), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			Context("when host exists for user", func() {
				Context("when host has containers", func() {
					var c *container.Container

					BeforeEach(func() {
						var err error
						c = &container.Container{
							ID:     "deadbeef",
							UserID: u.ID,
							HostID: h.ID,
							Image:  "nitrous-base",
							Name:   "dominant-intercooler-42",
						}
						err = c.Insert(db)
						Expect(err).To(BeNil())
					})

					It("responds with the containers on the host", func() {
						doRequest("valid-token", "foo-host")

						Expect(res.Code).To(Equal(http.StatusOK))

						var j map[string][]container.Container
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						containers, ok := j["containers"]
						Expect(ok).To(BeTrue())
						Expect(containers).To(HaveLen(1))

						Expect(containers[0].ID).To(Equal("deadbeef"))
						Expect(containers[0].UserID).To(Equal(u.ID))
						Expect(containers[0].HostID).To(Equal(h.ID))
						Expect(containers[0].Image).To(Equal("nitrous-base"))
						Expect(containers[0].Name).To(Equal("dominant-intercooler-42"))
					})
				})

				Context("when host has no containers", func() {
					It("responds with an empty array", func() {
						doRequest("valid-token", "foo-host")

						Expect(res.Code).To(Equal(http.StatusOK))
						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						containers, ok := j["containers"].([]interface{})
						Expect(ok).To(BeTrue())
						Expect(containers).To(BeEmpty())
					})
				})
			})

			Context("when host does not exist for user", func() {
				It("returns 404", func() {
					doRequest("valid-token", "no-such-host")

					Expect(res.Code).To(Equal(http.StatusNotFound))

					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					errType, ok := j["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(errType).To(Equal("not_found"))

					errDesc, ok := j["error_description"].(string)
					Expect(ok).To(BeTrue())
					Expect(errDesc).To(Equal("host does not exist"))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo-host/containers"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
