package userprofiles_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/userprofiles")
}

var _ = Describe("controllers/userprofiles", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router

		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()

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

	Describe("GET /api/v0/user/profile", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", "/api/v0/user/profile", &url.Values{})
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should respond even if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("responds with given user's profile serialized as JSON'", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				UserProfile serializers.UserProfile `json:"user_profile"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.UserProfile.Advanced).To(Equal(false))
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user/profile"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/user/profile", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "POST", "/api/v0/user/profile", params)
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			params := &url.Values{}
			params.Set("advanced", "true")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		Context("when user is a paid user", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StatePaid, u.ID)
				Expect(err).To(BeNil())
			})

			It("allows advanced settings", func() {
				params := &url.Values{}
				params.Set("advanced", "true")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					UserProfile serializers.UserProfile `json:"user_profile"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.UserProfile.Advanced).To(Equal(true))
			})
		})

		Context("when user is a free tier user", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())
			})

			It("does not allow advanced settings", func() {
				params := &url.Values{}
				params.Set("advanced", "true")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				up, err := userprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
				Expect(up.Advanced).To(BeFalse())
			})
		})

		Context("when user has no card on record", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateNoCard, u.ID)
				Expect(err).To(BeNil())
			})

			It("does not allow advanced settings", func() {
				params := &url.Values{}
				params.Set("advanced", "true")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				up, err := userprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
				Expect(up.Advanced).To(BeFalse())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user/profile"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
