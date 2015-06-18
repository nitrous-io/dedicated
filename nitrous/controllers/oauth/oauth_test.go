package oauth_test

import (
	"encoding/json"
	"fmt"
	"html/template"
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
		svr = NewTestServer("", db, NewFakeTracker())
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
		var (
			u *user.User

			client1, client2 *oauthclient.Client
			client1Secret    = "secret_sauce"
			client2Secret    = "top_secret"

			stateStr = "anti_csrf"
		)

		parseJSONResponse := func() map[string]interface{} {
			var j map[string]interface{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			return j
		}

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

			client1 = &oauthclient.Client{
				UserID:      clientUser1.ID,
				Name:        "Builders League United",
				Description: "A fictional company",
				ClientID:    "blu_team",
				RedirectURI: "https://blu.co/oauth/callback",
			}
			err = client1.InsertWithSecret(db, client1Secret)
			Expect(err).To(BeNil())

			clientUser2 := &user.User{
				Username: "redmond",
				Password: "123456",
				Email:    "info@redmond.com",
			}
			err = WrapInTransaction(db, clientUser2.Insert)
			Expect(err).To(BeNil())
			Expect(clientUser2.ID).NotTo(BeZero())

			client2 = &oauthclient.Client{
				UserID:      clientUser2.ID,
				Name:        "Reliable Excavation Demolition",
				Description: "A fictional company",
				ClientID:    "red_team",
				RedirectURI: "https://red.co/oauth/callback",
			}
			err = client2.InsertWithSecret(db, client2Secret)
			Expect(err).To(BeNil())
		})

		expectRedirectResponse := func(redirectURI string, query map[string]string) *url.Values {
			Expect(res.Code).To(Equal(http.StatusFound))
			ru, err := url.Parse(res.Header().Get("Location"))
			Expect(err).To(BeNil())
			iu, err := url.Parse(redirectURI)
			Expect(err).To(BeNil())
			Expect(ru.Scheme).To(Equal(iu.Scheme))
			Expect(ru.Host).To(Equal(iu.Host))
			Expect(ru.Path).To(Equal(iu.Path))
			q := ru.Query()
			for k, v := range query {
				Expect(q).To(HaveKeyWithValue(k, []string{v}))
			}
			return &q
		}

		expectErrorResponse := func(status int, errCode, errDesc string) {
			Expect(res.Code).To(Equal(status))
			c, d := ErrorSlug(res.Body)
			Expect(c).To(Equal(errCode))
			Expect(d).To(Equal(errDesc))
		}

		Describe("GET /api/v0/oauth/authorize by user agent", func() {
			doRequest := func() {
				tmpl, err := template.New("oauth/authorize").Parse("<pre><code>{{.ClientID}} {{.ClientName}} {{.ClientDescription}} {{.RedirectURI}} {{.Scope}} {{.State}}</code></pre>")
				Expect(err).To(BeNil())
				origTmpl := oauth.TmplAuthorize
				oauth.TmplAuthorize = tmpl
				defer func() {
					oauth.TmplAuthorize = origTmpl
				}()

				req, err = NewRequest("GET", "/api/v0/oauth/authorize", params)
				Expect(err).To(BeNil())
				res = httptest.NewRecorder()
				handler.ServeHTTP(res, req)
			}

			BeforeEach(func() {
				params.Set("response_type", "code")
				params.Set("client_id", client1.ClientID)
				params.Set("redirect_uri", client1.RedirectURI)
				params.Set("state", stateStr)
				params.Set("scope", "full")
			})

			Context("when response type is not code", func() {
				BeforeEach(func() {
					params.Set("response_type", "bs")
					doRequest()
				})

				It("redirects back with error", func() {
					expectRedirectResponse(client1.RedirectURI, map[string]string{
						"error":             "unsupported_response_type",
						"error_description": "The authorization server does not support obtaining an authorization code using this method.",
						"state":             stateStr,
					})
				})
			})

			Context("when client id is invalid", func() {
				BeforeEach(func() {
					params.Set("client_id", "no-such-client")
					doRequest()
				})

				It("returns 404 bad request with error", func() {
					expectErrorResponse(http.StatusNotFound, "invalid_request", "client not found")
				})
			})

			Context("when redirect uri is different", func() {
				BeforeEach(func() {
					params.Set("redirect_uri", client2.RedirectURI)
					doRequest()
				})

				It("returns 400 bad request with error", func() {
					expectErrorResponse(http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
				})
			})

			Context("when redirect uri is empty", func() {
				BeforeEach(func() {
					params.Set("redirect_uri", "")
					doRequest()
				})

				It("renders oauth authorize consent page", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
					body := res.Body.String()
					Expect(body).To(ContainSubstring(client1.ClientID))
					Expect(body).To(ContainSubstring(client1.Name))
					Expect(body).To(ContainSubstring(client1.Description))
					Expect(body).To(ContainSubstring(client1.RedirectURI))
					Expect(body).To(ContainSubstring("full"))
					Expect(body).To(ContainSubstring(stateStr))
				})
			})

			Context("when everything is valid", func() {
				BeforeEach(func() {
					doRequest()
				})

				It("renders oauth authorize consent page", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
					body := res.Body.String()
					Expect(body).To(ContainSubstring(client1.ClientID))
					Expect(body).To(ContainSubstring(client1.Name))
					Expect(body).To(ContainSubstring(client1.Description))
					Expect(body).To(ContainSubstring(client1.RedirectURI))
					Expect(body).To(ContainSubstring("full"))
					Expect(body).To(ContainSubstring(stateStr))
				})
			})
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

			BeforeEach(func() {
				params.Set("action", "approve")
				params.Set("state", stateStr)
				params.Set("client_id", client1.ClientID)
				params.Set("redirect_uri", client1.RedirectURI)
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

					expectRedirectResponse(client1.RedirectURI, map[string]string{
						"error":             "access_denied",
						"error_description": "The access token used does not permit authorization of new clients",
						"state":             stateStr,
					})
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
					It("should fail if invalid client id is provided", func() {
						params.Set("client_id", "green")
						doRequest()
						expectErrorResponse(http.StatusNotFound, "invalid_request", "client not found")
					})

					It("should fail if redirect URI is not subpath of original", func() {
						params.Set("redirect_uri", "http://red.co/oauth/callback")
						doRequest()
						expectErrorResponse(http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
					})

					It("should fail if redirect URI different scheme", func() {
						params.Set("redirect_uri", "http://blu.co/oauth/callback")
						doRequest()
						expectErrorResponse(http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
					})

					It("should fail if user did not approve authorization", func() {
						params.Set("action", "decline")

						doRequest()
						expectRedirectResponse(client1.RedirectURI, map[string]string{
							"error":             "access_denied",
							"error_description": "The resource owner or authorization server denied the request.",
							"state":             stateStr,
						})
					})
				})

				Context("when valid parameters requested", func() {
					expectValidAuthCode := func(code, redirectURI string) {
						ac, err := authorizationcode.Read(code)
						Expect(err).To(BeNil())
						Expect(ac.UserID).To(Equal(u.ID))
						Expect(ac.ClientID).To(Equal(client1.ClientID))
						Expect(ac.RedirectURI).To(Equal(redirectURI))
					}

					It("should get authorization code", func() {
						doRequest()
						q := expectRedirectResponse(client1.RedirectURI, map[string]string{
							"state": stateStr,
						})
						expectValidAuthCode(q.Get("code"), client1.RedirectURI)
					})

					It("should still succeed if redirect URI has some extra query string", func() {
						uri := "https://blu.co/oauth/callback?special=true"
						params.Set("redirect_uri", uri)
						doRequest()
						q := expectRedirectResponse(uri, map[string]string{
							"state": stateStr,
						})
						expectValidAuthCode(q.Get("code"), uri)
					})

				})
			})
		})

		Describe("POST /api/v0/oauth/token, grant_type=authorization_code by client", func() {
			doRequest := func() {
				var err error
				req, err = NewFormRequest("POST", "/api/v0/oauth/token", params)
				Expect(err).To(BeNil())
				res = httptest.NewRecorder()
				handler.ServeHTTP(res, req)
			}

			generateCode := func() string {
				code, err := authorizationcode.Generate(u.ID, client1.ClientID, client1.RedirectURI)
				Expect(err).To(BeNil())
				return code
			}

			BeforeEach(func() {
				params.Set("grant_type", "authorization_code")
				params.Set("code", generateCode())
				params.Set("client_id", client1.ClientID)
				params.Set("client_secret", client1Secret)
				params.Set("redirect_uri", client1.RedirectURI)
			})

			Context("when client request with invalid parameters", func() {
				It("should fail if requested by different client", func() {
					params.Set("client_id", "red_team")
					params.Set("client_secret", client2Secret)
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
					code := params.Get("code")
					code = "tempered" + code
					params.Set("code", code)

					doRequest()
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_request"))
					Expect(errDesc).To(Equal("invalid authorization code"))
				})

				It("should fail if authorization code is expired", func() {
					// modify global to get a negative-lived code
					originalValidity := authorizationcode.ValidityPeriod
					authorizationcode.ValidityPeriod = -5 * time.Second
					defer func() {
						authorizationcode.ValidityPeriod = originalValidity
					}()
					params.Set("code", generateCode())

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
