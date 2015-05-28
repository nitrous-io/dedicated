package oauth_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/oauth"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/authorizationcode"
	"github.com/nitrous-io/dedicated/nitrous/pkg/logintoken"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/oauth")
}

var _ = Describe("controllers/oauth", func() {
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
		params = &url.Values{}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /oauth/token", func() {
		var (
			u *user.User
			j map[string]interface{}
		)

		doRequest := func() {
			var err error
			req, err = NewFormRequest("POST", "/api/v0/oauth/token", params)
			Expect(err).To(BeNil())
			res = httptest.NewRecorder()
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			j = map[string]interface{}{}
		})

		itRespondsWithUniqueAccessToken := func() {
			It("responds with a unique access token", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["access_token"]).NotTo(BeNil())
				Expect(j["token_type"]).To(Equal("Bearer"))
			})

			It("persists the token in the database", func() {
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
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
		}

		Context("when grant_type is password", func() {
			BeforeEach(func() {
				params.Set("grant_type", "password")
				params.Set("username", "foo")
				params.Set("password", "123456")
			})

			logins := []string{
				"foo",
				"foo@example.com",
			}

			for _, login := range logins {
				l := login

				Context("when requested with valid login("+l+") and password", func() {
					BeforeEach(func() {
						params.Set("username", l)
						doRequest()

						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
					})

					itRespondsWithUniqueAccessToken()
				})
			}

			Context("when missing username is given", func() {
				BeforeEach(func() {
					params.Set("username", "nosuch")
				})

				It("responds with invalid creds", func() {
					doRequest()
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_grant"))
					Expect(j["error_description"]).To(Equal("incorrect username or password"))
					Expect(j["access_token"]).To(BeNil())
				})
			})

			Context("when a wrong password is given", func() {
				var durationReceived time.Duration

				BeforeEach(func() {
					oauth.TimerFunc = func(d time.Duration) *time.Timer {
						durationReceived = d
						return time.NewTimer(0) // set duration to 0 for faster tests
					}
				})

				It("responds with 400 bad request", func() {
					params.Set("password", "111111")
					doRequest()

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_grant"))
					Expect(j["error_description"]).To(Equal("incorrect username or password"))
					Expect(j["access_token"]).To(BeNil())
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
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_grant"))
					Expect(j["access_token"]).To(BeNil())
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
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						Expect(j["error"]).To(Equal("invalid_request"))
						Expect(j["error_description"]).To(Equal(v + " is required"))
						Expect(j["access_token"]).To(BeNil())
					})
				})
			}
		})

		Context("when grant_type is login_token", func() {
			BeforeEach(func() {
				params.Set("grant_type", "login_token")
			})

			Context("when a valid login token is given", func() {
				BeforeEach(func() {
					loginToken, err := logintoken.Generate(u.ID)
					Expect(err).To(BeNil())
					params.Set("token", loginToken)
					doRequest()
				})

				itRespondsWithUniqueAccessToken()
			})

			Context("when a login token with non-existent user id is given", func() {
				BeforeEach(func() {
					loginToken, err := logintoken.Generate(u.ID + 99999)
					Expect(err).To(BeNil())
					params.Set("token", loginToken)
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_grant"))
					Expect(j["error_description"]).To(Equal("invalid login token"))
					Expect(j["access_token"]).To(BeNil())
				})
			})

			Context("when an expired login token is given", func() {
				BeforeEach(func() {
					origPeriod := logintoken.DefaultValidityPeriod
					logintoken.DefaultValidityPeriod = 1 * time.Millisecond
					defer func() {
						logintoken.DefaultValidityPeriod = origPeriod
					}()

					loginToken, err := logintoken.Generate(u.ID)
					Expect(err).To(BeNil())
					params.Set("login_token", loginToken)

					time.Sleep(2 * time.Millisecond)

					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_grant"))
					Expect(j["error_description"]).To(Equal("invalid login token"))
					Expect(j["access_token"]).To(BeNil())
				})
			})

			Context("when an invalid login token is given", func() {
				BeforeEach(func() {
					origSecret := logintoken.Secret
					logintoken.Secret = "F4k3 53cR37 y0"
					defer func() {
						logintoken.Secret = origSecret
					}()

					loginToken, err := logintoken.Generate(u.ID)
					Expect(err).To(BeNil())
					params.Set("login_token", loginToken)

					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_grant"))
					Expect(j["error_description"]).To(Equal("invalid login token"))
					Expect(j["access_token"]).To(BeNil())
				})
			})
		})

		Context("when an unsupported grant type is given", func() {
			BeforeEach(func() {
				params.Set("grant_type", "backdoor")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("unsupported_grant_type"))
			})
		})
	})

	Describe("DELETE /logout", func() {
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

				res = httptest.NewRecorder()
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

	Describe("authorization code", func() {
		var u *user.User

		BeforeEach(func() {
			var err error
			u = &user.User{
				Username: "demoman",
				Password: "scottishhandshake",
				Email:    "demoman@mann.co",
			}
			err = WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			clientUser1 := &user.User{
				Username: "blutarch",
				Password: "mannup",
				Email:    "info@blutarch.com",
			}
			err = WrapInTransaction(db, clientUser1.Insert)
			Expect(err).To(BeNil())
			Expect(clientUser1.ID).NotTo(BeZero())

			client1 := &oauthclient.Client{
				UserID:      clientUser1.ID,
				Name:        "Builders League United",
				Description: "A fictional company",
				ClientID:    "blu_team",
				RedirectURI: "https://blu.co/oauth/callback",
			}
			err = client1.InsertWithSecret(db, "secret_sauce")
			Expect(err).To(BeNil())

			clientUser2 := &user.User{
				Username: "redmond",
				Password: "123456",
				Email:    "info@redmond.com",
			}
			err = WrapInTransaction(db, clientUser2.Insert)
			Expect(err).To(BeNil())
			Expect(clientUser2.ID).NotTo(BeZero())

			client2 := &oauthclient.Client{
				UserID:      clientUser2.ID,
				Name:        "Reliable Excavation Demolition",
				Description: "A fictional company",
				ClientID:    "red_team",
				RedirectURI: "https://red.co/oauth/callback",
			}
			err = client2.InsertWithSecret(db, "top_secret")
			Expect(err).To(BeNil())
		})

		Describe("POST /api/v0/oauth/authorize by user agent", func() {
			var t *token.Token

			doRequest := func() {
				var err error
				req, err = NewFormRequest("POST", "/api/v0/oauth/authorize", params)
				Expect(err).To(BeNil())
				res = httptest.NewRecorder()
				handler.ServeHTTP(res, req)
			}

			parseJSONResponse := func() map[string]interface{} {
				var j map[string]interface{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				return j
			}

			// Checks if two URI matches, excluding query string
			expectBaseURI := func(expectedURIStr string) {
				resURI, err := url.Parse(res.Header().Get("Location"))
				Expect(err).To(BeNil())
				resURI.RawQuery = ""

				expectURI, err := url.Parse(expectedURIStr)
				Expect(err).To(BeNil())
				expectURI.RawQuery = ""

				Expect(resURI.String()).To(Equal(expectURI.String()))
			}

			expectErrorResponse := func(errCode, errDesc, redirectURI string) {
				Expect(res.Code).To(Equal(http.StatusFound))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal(errCode))
				Expect(errDesc).To(Equal(errDesc))
				expectBaseURI(redirectURI)
			}

			BeforeEach(func() {
				params.Set("action", "approve")
				params.Set("grant_type", "authorization_code")
				params.Set("state", "anti_csrf")
				params.Set("response_type", "code")
				params.Set("client_id", "blu_team")
			})

			Context("when user is not logged in", func() {
				It("should not able to get authorization code", func() {
					var err error
					req, err = NewFormRequest("POST", "/api/v0/oauth/authorize", params)
					Expect(err).To(BeNil())
					res = httptest.NewRecorder()
					handler.ServeHTTP(res, req)

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_request"))
					Expect(errDesc).To(Equal("auth token is required"))
				})
			})

			Context("when token is not full scope", func() {
				BeforeEach(func() {
					t = &token.Token{
						UserID: u.ID,
						Type:   "login",
						Scope:  token.ScopeThirdParty,
					}

					err := t.Insert(db)
					Expect(err).To(BeNil())

					params.Set("access_token", t.Token)
				})

				It("should not be able to authorize other third party", func() {
					doRequest()

					expectErrorResponse("access_denied", "token scope not allowed to request new authorization code", "/app")
				})
			})

			Context("when user is logged in", func() {
				BeforeEach(func() {
					t = &token.Token{
						UserID: u.ID,
						Type:   "login",
						Scope:  token.ScopeFull,
					}
					err := t.Insert(db)
					Expect(err).To(BeNil())

					params.Set("access_token", t.Token)
				})

				Context("when invalid parameters requested", func() {
					It("should fail if response_type is not code", func() {
						params.Set("response_type", "klingon")
						doRequest()
						expectErrorResponse("unsupported_response_type", "response type must be 'code'", "/app")
					})

					It("should fail if invalid client id is provided", func() {
						params.Set("client_id", "green")
						doRequest()
						expectErrorResponse("invalid_request", "client ID not found", "/app")
					})

					It("should fail if redirect URI is not subpath of original", func() {
						params.Set("redirect_uri", "http://red.co/oauth/callback")
						doRequest()
						expectErrorResponse("invalid_request", "invalid redirect_uri", "/app")
					})

					It("should fail if redirect URI different scheme", func() {
						params.Set("redirect_uri", "http://blu.co/oauth/callback")
						doRequest()
						expectErrorResponse("invalid_request", "invalid redirect_uri", "/app")
					})

					It("should fail if user did not approve authorization", func() {
						params.Set("action", "decline")
						params.Set("redirect_uri", "https://blu.co/oauth/callback")

						doRequest()
						expectErrorResponse("access_denied", "the resource owner has denied the request", "https://blu.co/oauth/callback")
					})
				})

				Context("when valid parameters requested", func() {
					It("should get authorization code", func() {
						doRequest()
						Expect(res.Code).To(Equal(http.StatusFound))
						expectBaseURI("https://blu.co/oauth/callback")
						j := parseJSONResponse()
						Expect(j["state"]).To(Equal("anti_csrf"))
						Expect(j["code"]).ToNot(Equal(""))
						Expect(j["redirect_uri"]).To(Equal("https://blu.co/oauth/callback"))
					})

					It("should success if redirect URI is subpath of original", func() {
						params.Set("redirect_uri", "https://blu.co/oauth/callback?special=true")
						doRequest()
						Expect(res.Code).To(Equal(http.StatusFound))
						expectBaseURI("https://blu.co/oauth/callback")
						j := parseJSONResponse()
						Expect(j["redirect_uri"]).To(Equal("https://blu.co/oauth/callback?special=true"))
					})

					Describe("POST /api/v0/oauth/token, grant_type=authorization_code by client", func() {
						getCode := func() string {
							doRequest()
							Expect(res.Code).To(Equal(http.StatusFound))
							j := parseJSONResponse()
							code := j["code"].(string)
							Expect(code).ToNot(Equal(""))
							return code
						}

						doRequest := func() {
							var err error
							req, err = NewAuthedFormRequest(t, "POST", "/api/v0/oauth/token", params)
							Expect(err).To(BeNil())
							res = httptest.NewRecorder()
							handler.ServeHTTP(res, req)
						}

						BeforeEach(func() {
							params.Set("grant_type", "authorization_code")
							params.Set("code", getCode())
							params.Set("client_id", "blu_team")
							params.Set("client_secret", "secret_sauce")
							params.Set("redirect_uri", "https://blu.co/oauth/callback")
						})

						Context("when client request with invalid parameters", func() {
							It("should fail if requested by different client", func() {
								params.Set("client_id", "red_team")
								params.Set("client_secret", "top_secret")
								params.Set("redirect_uri", "https://red.co/oauth/callback")

								doRequest()
								Expect(res.Code).To(Equal(http.StatusForbidden))
								errType, errDesc := ErrorSlug(res.Body)
								Expect(errType).To(Equal("access_denied"))
								Expect(errDesc).To(Equal("client ID does not match authorization code"))
							})

							It("should fail if client secret not match", func() {
								params.Set("client_secret", "fake_secret")
								doRequest()
								Expect(res.Code).To(Equal(http.StatusUnauthorized))
								errType, errDesc := ErrorSlug(res.Body)
								Expect(errType).To(Equal("access_denied"))
								Expect(errDesc).To(Equal("invalid client secret"))
							})

							It("should fail if authorization code is tempered", func() {
								code := getCode()
								code = "tempered" + code
								params.Set("code", code)

								doRequest()
								Expect(res.Code).To(Equal(http.StatusBadRequest))
								errType, errDesc := ErrorSlug(res.Body)
								Expect(errType).To(Equal("invalid_request"))
								Expect(errDesc).To(Equal("invalid authoriazation code"))
							})

							It("should fail if authorization code is expired", func() {
								// modify global to get a negative-lived code
								originalValidity := authorizationcode.ValidityPeriod
								authorizationcode.ValidityPeriod = -5 * time.Second
								params.Set("code", getCode())
								authorizationcode.ValidityPeriod = originalValidity

								doRequest()
								Expect(res.Code).To(Equal(http.StatusUnauthorized))
								errType, errDesc := ErrorSlug(res.Body)
								Expect(errType).To(Equal("access_denied"))
								Expect(errDesc).To(Equal("authorization code has expired"))
							})

							It("should fail if requested twice with same nonce", func() {
								doRequest()
								Expect(res.Code).To(Equal(http.StatusOK))

								doRequest()
								Expect(res.Code).To(Equal(http.StatusUnauthorized))
								errType, errDesc := ErrorSlug(res.Body)
								Expect(errType).To(Equal("access_denied"))
								Expect(errDesc).To(Equal("authorization code cannot be reused"))
							})
						})

						Context("when client request with valid parameters", func() {
							It("should get bearer token", func() {
								doRequest()
								Expect(res.Code).To(Equal(http.StatusOK))
								j := parseJSONResponse()
								Expect(j["access_token"]).ToNot(Equal(""))
								Expect(j["token_type"]).To(Equal("Bearer"))
								Expect(j["scope"]).To(Equal("third_party"))
							})

							It("should be able to gain basic user access", func() {
								doRequest()
								Expect(res.Code).To(Equal(http.StatusOK))
								j := parseJSONResponse()
								token := j["access_token"].(string)
								Expect(token).ToNot(Equal(""))

								var err error
								req, err = NewRequest("GET", "/api/v0/user", nil)
								Expect(err).To(BeNil())
								req.Header.Set("Authorization", "Bearer "+token)
								res = httptest.NewRecorder()
								handler.ServeHTTP(res, req)

								Expect(res.Code).To(Equal(http.StatusOK))

								j2 := map[string]interface{}{}
								err = json.Unmarshal(res.Body.Bytes(), &j2)
								Expect(err).To(BeNil())

								user, ok := j2["user"].(map[string]interface{})
								Expect(ok).To(BeTrue())
								Expect(user["username"]).To(Equal("demoman"))
							})
						})
					})
				})
			})
		})
	})
})
