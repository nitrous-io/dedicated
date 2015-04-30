package tokens_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/tokens"
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
	RunSpecs(t, "controllers/tokens")
}

var _ = Describe("controllers/tokens", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
		params  *url.Values
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = NewTestServer("", db, logger, NewFakeTracker())
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /tokens", func() {
		var (
			u                *user.User
			durationReceived time.Duration
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			tokens.TimerFunc = func(d time.Duration) *time.Timer {
				durationReceived = d
				return time.NewTimer(0) // set duration to 0 for faster tests
			}

			params = &url.Values{}
			params.Set("grant_type", "password")
			params.Set("username", "foo")
			params.Set("password", "123456")
		})

		doRequest := func() {
			var err error
			req, err = NewFormRequest("POST", "/api/v0/oauth/token", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		logins := []string{
			"foo",
			"foo@example.com",
		}

		for _, login := range logins {
			l := login

			Context("when requested with valid login("+l+") and password", func() {
				var j map[string]interface{}

				BeforeEach(func() {
					params.Set("username", l)
					doRequest()

					j = map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
				})

				It("responds with a unique access token", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
					Expect(j["access_token"]).NotTo(BeNil())
					Expect(j["token_type"]).To(Equal("Bearer"))
				})

				It("persists the token in the database", func() {
					t, err := token.FindByToken(db, j["access_token"].(string))
					Expect(err).To(BeNil())
					Expect(t).NotTo(BeNil())
					Expect(t.UserID).To(Equal(u.ID))
					Expect(t.Token).To(Equal(j["access_token"]))
					Expect(t.CreatedAt).NotTo(BeNil())
				})

				It("has no-cache headers", func() {
					Expect(res.Header().Get("Cache-Control")).To(Equal("no-store"))
					Expect(res.Header().Get("Pragma")).To(Equal("no-cache"))
				})

				Context("making another token request", func() {
					var prevToken interface{}

					BeforeEach(func() {
						res = httptest.NewRecorder()
						prevToken = j["access_token"]
						doRequest()
					})

					It("responds with a different access token", func() {
						Expect(res.Code).To(Equal(http.StatusOK))
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						Expect(j["access_token"]).NotTo(Equal(prevToken))
						Expect(j["token_type"]).To(Equal("Bearer"))
					})
				})
			})
		}

		Context("when missing username is given", func() {
			BeforeEach(func() {
				params.Set("username", "nosuch")
			})

			It("responds with invalid creds", func() {
				doRequest()
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_grant"))
				Expect(j["error_description"]).To(Equal("incorrect username or password"))
			})
		})

		Context("when a wrong password is given", func() {
			It("responds with 400 bad request", func() {
				params.Set("password", "111111")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_grant"))
			})

			It("should not delay under 3 requests", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				params.Set("password", "111111")

				doRequest()
				Expect(durationReceived).To(Equal(time.Duration(0)))

				doRequest()
				Expect(durationReceived).To(Equal(time.Duration(0)))

				doRequest()
				Expect(durationReceived).To(Equal(time.Duration(0)))
			})

			It("should delay correctly", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				params.Set("password", "111111")

				doRequest()
				Expect(durationReceived).To(Equal(time.Duration(0)))

				_, err = db.Exec("UPDATE users SET login_attempts = 4 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				doRequest()
				Expect(durationReceived).To(Equal(time.Second * 1))

				_, err = db.Exec("UPDATE users SET login_attempts = 7 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				doRequest()
				Expect(durationReceived).To(Equal(time.Second * 3))

				_, err = db.Exec("UPDATE users SET login_attempts = 10 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				doRequest()
				Expect(durationReceived).To(Equal(time.Second * 5))
			})

			It("should reset the delay after a successful auth", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 3 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				params.Set("password", "123456")
				doRequest()
				Expect(durationReceived).To(Equal(time.Second * 1))

				doRequest()
				Expect(durationReceived).To(Equal(time.Duration(0)))
			})
		})

		Context("user is deleted", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE users SET deleted_at = now() WHERE id = $1", u.ID)
				Expect(err).To(BeNil())

				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_grant"))
			})
		})

		Context("when grant type is not password", func() {
			BeforeEach(func() {
				params.Set("grant_type", "authorization_code")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("unsupported_grant_type"))
			})
		})

		for _, v := range []string{"username", "password"} {
			Context(fmt.Sprintf(`when the required param "%s" is missing`, v), func() {
				BeforeEach(func() {
					params.Del(v)
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
					Expect(j["error_description"]).To(Equal(v + " is required"))
				})
			})
		}
	})

	Describe("POST /logout", func() {
		var (
			u *user.User
			t *token.Token
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Name:     "foo",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			t = &token.Token{
				UserID: u.ID,
				Token:  "valid-token",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())
		})

		ItSuccessfullyDeletesTheToken := func() {
			It("successfully deletes the token", func() {
				req, err := NewAuthedRequest(t, "DELETE", "/api/v0/logout", &url.Values{})
				Expect(err).To(BeNil())

				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusOK))

				toke, err := token.FindByToken(db, "valid-token")
				Expect(err).To(Equal(token.ErrInvalidToken))
				Expect(toke).To(BeNil())

				req, err = NewAuthedRequest(t, "GET", "/api/v0/user", &url.Values{})
				Expect(err).To(BeNil())

				res = httptest.NewRecorder()
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusUnauthorized))
			})
		}

		Context("when user has billing issues", func() {
			BeforeEach(func() {
				MarkDelinquentUser(db, u)
			})

			ItSuccessfullyDeletesTheToken()
		})

		ItSuccessfullyDeletesTheToken()
	})
})
