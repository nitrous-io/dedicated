package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "middleware")
}

var _ = Describe("middleware", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router
		res     *httptest.ResponseRecorder
		env     *middleware.Env
		emptyMw middleware.Middleware
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)

		handler = mux.NewRouter()
		res = httptest.NewRecorder()

		emptyMw = func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
			return nil
		}
		env = &middleware.Env{
			Logger:  logger,
			DB:      db,
			Tracker: NewFakeTracker(),
		}
	})

	Describe("authentication middleware", func() {
		var (
			doRequest func(string)
			u         *user.User
			mw        func(middleware.Middleware) middleware.Middleware
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

			t := &token.Token{
				UserID: u.ID,
				Token:  "foobar",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			h := func(w http.ResponseWriter, r *http.Request) {
				err := mw(emptyMw)(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler.HandleFunc("/authaction", h).Methods("GET")
		})

		itWorks := func() {
			Context("when a valid OAuth token is given", func() {
				It("sets the User in the environment", func() {
					doRequest("foobar")
					Expect(env.User).To(Equal(u))
				})

				It("Responds with 200 OK", func() {
					doRequest("foobar")
					Expect(res.Code).To(Equal(http.StatusOK))
				})

				It("updates the token's last_seen_at", func() {
					doRequest("foobar")
					now := time.Now()
					t2, err := token.FindByToken(db, "foobar")
					Expect(err).To(BeNil())
					Expect(t2).NotTo(BeNil())
					Expect(t2.LastSeenAt.Unix()).To(BeNumerically("<=", now.Unix()))
					Expect(t2.LastIP.String).To(Equal("1.2.3.4"))
				})
			})

			Context("when an invalid OAuth token is given", func() {
				It("responds with 401 Unauthorized", func() {
					doRequest("invalidtoken")

					Expect(res.Code).To(Equal(http.StatusUnauthorized))
					Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_token"`))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_token"))
					Expect(j["error_description"]).To(Equal("auth token is invalid"))
				})
			})

			Context("when a token has not been used for a while", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE tokens SET last_seen_at = now() - interval '14 days' WHERE token = 'foobar';`)
					Expect(err).To(BeNil())
				})

				It("responds with 401 Unauthorized", func() {
					doRequest("foobar")

					Expect(res.Code).To(Equal(http.StatusUnauthorized))
					Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_token"`))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_token"))
					Expect(j["error_description"]).To(Equal("auth token is invalid"))
				})
			})

			Context("when no OAuth token is given", func() {
				It("responds with 400 Bad Request", func() {
					doRequest("")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_request"`))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
					Expect(j["error_description"]).To(Equal("auth token is required"))
				})
			})

			Context("when the OAuth token's owner has lapsed confirmation", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE users SET created_at = now() - interval '26 hours' WHERE id = $1;`, u.ID)
					Expect(err).To(BeNil())
				})

				It("responds with 400 Bad Request", func() {
					doRequest("foobar")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="confirmation_lapsed"`))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("confirmation_lapsed"))
					Expect(j["error_description"]).To(Equal("confirmation token has lapsed"))
				})
			})

			Context("when the user is a service account", func() {
				BeforeEach(func() {
					u := &user.User{
						Username: "host-provisioner",
						Password: "123456",
						Email:    "hp@example.com",
					}
					err := WrapInTransaction(db, u.Insert)
					Expect(err).To(BeNil())
					_, err = db.Exec(`UPDATE users SET service_account = true WHERE id = $1;`, u.ID)
					Expect(err).To(BeNil())

					// Make sure the account has "lapsed confirmation".
					_, err = db.Exec(`UPDATE users SET created_at = now() - interval '26 hours' WHERE id = $1;`, u.ID)
					Expect(err).To(BeNil())

					t := &token.Token{
						UserID: u.ID,
						Token:  "provisioner-token",
					}
					err = t.Insert(db)
					Expect(err).To(BeNil())
				})

				It("responds with 200 OK", func() {
					doRequest("provisioner-token")

					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})
		}

		// authenticate using Authorization header
		Describe("Auth", func() {
			BeforeEach(func() {
				mw = middleware.Auth
				doRequest = func(token string) {
					req, err := NewRequest("GET", "/authaction", nil)
					req.RemoteAddr = "1.2.3.4"
					Expect(err).To(BeNil())
					if token != "" {
						req.Header.Set("Authorization", "Bearer "+token)
					}
					handler.ServeHTTP(res, req)
				}
			})

			itWorks()
		})

		// authenticate using Authorization header OR query string (?token=)
		Describe("AuthQS", func() {
			BeforeEach(func() {
				mw = middleware.AuthQS
			})

			Context("auth using Authorization header", func() {
				BeforeEach(func() {
					doRequest = func(token string) {
						req, err := NewRequest("GET", "/authaction", nil)
						req.RemoteAddr = "1.2.3.4:32"
						Expect(err).To(BeNil())
						if token != "" {
							req.Header.Set("Authorization", "Bearer "+token)
						}
						handler.ServeHTTP(res, req)
					}
				})

				itWorks()
			})

			Context("auth using token query string", func() {
				BeforeEach(func() {
					doRequest = func(token string) {
						params := &url.Values{}
						params.Set("token", token)
						req, err := NewRequest("GET", "/authaction", params)
						req.RemoteAddr = "1.2.3.4 127.0.0.1"
						Expect(err).To(BeNil())
						handler.ServeHTTP(res, req)
					}
				})

				itWorks()
			})
		})
	})

	Describe("event tracking middleware", func() {
		var (
			u *user.User
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
			env.User = u
		})

		doRequest := func() {
			h := func(w http.ResponseWriter, r *http.Request) {
				err := middleware.Track("Action Called", emptyMw)(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler.HandleFunc("/action", h).Methods("GET")

			req, err := NewRequest("GET", "/action", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("sets the TrackEventName in the environment", func() {
			doRequest()
			Expect(env.TrackEventName).To(Equal("Action Called"))
		})

		It("adds $ip to event data", func() {
			doRequest()
			t := env.Tracker.(*FakeTracker)
			Expect(t.TrackEventProperties[u.ID][0]["$ip"]).To(Equal("0"))
		})

		It("Responds with 200 OK", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("Sets analytics profile attributes", func() {
			data := map[string]interface{}{"persona": "student"}
			env.ProfileSet(data)
			doRequest()
			t := env.Tracker.(*FakeTracker)
			Expect(t.PropsCalled).To(HaveLen(1))
			pc := t.PropsCalled[0]
			Expect(pc.Mode).To(Equal("set"))
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.PropsSet).To(Equal(data))
		})

		It("Appends analytics profile attributes", func() {
			data := map[string]interface{}{"templates": []string{"python", "ruby"}}
			env.ProfileAppend(data)
			doRequest()
			t := env.Tracker.(*FakeTracker)
			Expect(t.PropsCalled).To(HaveLen(1))
			pc := t.PropsCalled[0]
			Expect(pc.Mode).To(Equal("append"))
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.PropsSet).To(Equal(data))
		})

		It("Sets once analytics profile attributes", func() {
			data := map[string]interface{}{"first_template": []string{"python"}}
			env.ProfileOnce(data)
			doRequest()
			t := env.Tracker.(*FakeTracker)
			Expect(t.PropsCalled).To(HaveLen(1))
			pc := t.PropsCalled[0]
			Expect(pc.Mode).To(Equal("setnx"))
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.PropsSet).To(Equal(data))
		})

		It("Increments analytics profile attributes", func() {
			data := map[string]int{"containers_created": 1}
			env.ProfileInc(data)
			doRequest()
			t := env.Tracker.(*FakeTracker)
			Expect(t.PropsCalled).To(HaveLen(1))
			pc := t.PropsCalled[0]
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.PropsInc).To(Equal(data))
		})

		It("Makes multiple profile calls if necessary", func() {
			setData := map[string]interface{}{"persona": "student"}
			env.ProfileSet(setData)
			incData := map[string]int{"containers_created": 1}
			env.ProfileInc(incData)
			doRequest()
			t := env.Tracker.(*FakeTracker)
			Expect(t.PropsCalled).To(HaveLen(2))
			pc := t.PropsCalled[0]
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.PropsSet).To(Equal(setData))
			pc2 := t.PropsCalled[1]
			Expect(pc2.UserID).To(Equal(u.ID))
			Expect(pc2.PropsInc).To(Equal(incData))
		})
	})

	Describe("verifying user billing middleware", func() {
		var (
			u *user.User
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
			env.User = u

			protectedRoute := func(w http.ResponseWriter, r *http.Request) {
				err := middleware.VerifyUserBilling("something", emptyMw)(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			allowedRoute := func(w http.ResponseWriter, r *http.Request) {
				err := middleware.VerifyUserBilling("users.GetAuthenticatedUser", emptyMw)(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler.HandleFunc("/protected", protectedRoute).Methods("GET")
			handler.HandleFunc("/allowed", allowedRoute).Methods("GET")
		})

		It("responds with 200 OK if the user has correct billing", func() {
			req, err := NewRequest("GET", "/protected", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("rejects the request if the billing information is invalid", func() {
			MarkDelinquentUser(db, u)
			req, err := NewRequest("GET", "/protected", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
			VerifyDelinquentUserResponse(res)
		})

		It("rejects the allows a non protected request even if the billing information is invalid", func() {
			MarkDelinquentUser(db, u)
			req, err := NewRequest("GET", "/allowed", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("should not continue if the billing profile is missing", func() {
			_, err := db.Exec(`DELETE FROM billing_profiles WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
			req, err := NewRequest("GET", "/protected", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("GetRemoteAddr", func() {
		var r *http.Request
		var err error

		BeforeEach(func() {
			r, err = NewRequest("GET", "/test", nil)
			Expect(err).To(BeNil())
		})

		Context("with RemoteAddr set to IP:PORT", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10:63333"
			})

			It("should return the IP", func() {
				Expect(middleware.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr set to multiple IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10:63333 202.157.130.9 202.157.130.202:60000"
			})

			It("should return the first IP address", func() {
				Expect(middleware.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr set to multiple IP addresses some of which are 'proxy' IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000"
			})

			It("should return the IP address which is not a proxy", func() {
				Expect(middleware.GetRemoteAddr(r)).To(Equal("202.157.130.202"))
			})
		})

		Context("with RemoteAddr is not set, but X-Forwarded-For is set", func() {
			BeforeEach(func() {
				r.RemoteAddr = ""
				r.Header.Set("X-Forwarded-For", "202.157.130.10")
			})

			It("should return the IP address in the X-Forwarded-For header", func() {
				Expect(middleware.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr is not set, but X-Forwarded-For is set to multiple IP addresses, some of which are 'trusted' IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = ""
				r.Header.Set("X-Forwarded-For", "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000")
			})

			It("should return the IP address in the X-Forwarded-For header", func() {
				Expect(middleware.GetRemoteAddr(r)).To(Equal("202.157.130.202"))
			})
		})

		Context("with both RemoteAddr and X-Forwarded-For are set", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10"
				r.Header.Set("X-Forwarded-For", "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000")
			})

			It("should return the IP address stored in RemoteAddr", func() {
				Expect(middleware.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})
	})

	Describe("GetBearerToken", func() {
		It("should handle a lack of Authorization header", func() {
			r := &http.Request{}
			t, err := middleware.GetBearerToken(r)
			Expect(err).ToNot(BeNil())
			Expect(t).To(BeEmpty())
		})

		It("should handle a malformed Authorization header", func() {
			r := &http.Request{
				Header: map[string][]string{
					"Authorization": []string{"ABC"},
				},
			}
			t, err := middleware.GetBearerToken(r)
			Expect(err).ToNot(BeNil())
			Expect(t).To(BeEmpty())
		})

		It("should extract the token from the Authorization Bearer", func() {
			r := &http.Request{
				Header: map[string][]string{
					"Authorization": []string{"Bearer ABC123x2"},
				},
			}
			t, err := middleware.GetBearerToken(r)
			Expect(err).To(BeNil())
			Expect(t).To(Equal("ABC123x2"))
		})
	})
})
