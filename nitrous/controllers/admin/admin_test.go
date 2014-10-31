package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/admin")
}

var _ = Describe("controllers/", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router
		res     *httptest.ResponseRecorder

		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
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
		doRequest := func(token string, params *url.Values) {
			req, err := NewFormRequest("POST", "/api/v0/admin/switch_user", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				params := &url.Values{}
				params.Set("login", "bar")

				doRequest("valid-token", params)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
			})

			It("responds with a 400 Bad Request, if there's no matching user for the given login'", func() {
				params := &url.Values{}
				params.Set("login", "bar")

				doRequest("valid-token", params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})

			Context("when there's a matching user for the given login", func() {
				var (
					u2 *user.User
					j  map[string]interface{}
				)

				BeforeEach(func() {
					// create a user with username 'bar'
					u2 = &user.User{
						Username: "bar",
						Password: "123456",
						Name:     "bar",
						Email:    "bar@example.com",
					}
					err := WrapInTransaction(db, u2.Insert)
					Expect(err).To(BeNil())

					params := &url.Values{}
					params.Set("login", "bar")
					doRequest("valid-token", params)

					j = map[string]interface{}{}
					err = json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
				})

				It("adds a new token for the matching user", func() {
					t, err := token.FindByUser(db, u2.ID)
					Expect(err).To(BeNil())
					Expect(t).NotTo(BeNil())
					Expect(t.UserID).To(Equal(u2.ID))
					Expect(t.Token).NotTo(BeNil())
					Expect(t.CreatedAt).NotTo(BeNil())
				})

				It("responds with the token added for the matching user", func() {
					t, err := token.FindByUser(db, u2.ID)
					Expect(err).To(BeNil())
					Expect(t).NotTo(BeNil())

					Expect(res.Code).To(Equal(http.StatusOK))
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

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
