package hosts_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
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
})
