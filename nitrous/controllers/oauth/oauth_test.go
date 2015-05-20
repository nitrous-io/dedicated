package oauth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/oauth"
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
	RunSpecs(t, "controllers/oauth")
}

var _ = Describe("controllers/oauth", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		svr     *server.Server
		u       *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = NewTestServer("", db, logger, NewFakeTracker())
		handler = svr.Router()
		res = httptest.NewRecorder()

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

		client1 := &oauth.Client{
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

		client2 := &oauth.Client{
			UserID:      clientUser2.ID,
			Name:        "Reliable Excavation Demolition",
			Description: "A fictional company",
			ClientID:    "red_team",
			RedirectURI: "https://red.co/oauth/callback",
		}
		err = client2.InsertWithSecret(db, "top_secret")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/oauth/authorize by user agent", func() {
		var (
			t      *token.Token
			params *url.Values
		)

		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewAuthedFormRequest(t, "POST", "/api/v0/oauth/authorize", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		getJsonResponse := func() map[string]interface{} {
			var j map[string]interface{}
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			return j
		}

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("grant_type", "authorization_code")
			params.Set("state", "anti_csrf")
			params.Set("response_type", "code")
			params.Set("client_id", "blu_team")
		})

		Describe("when user is not logged in", func() {
			It("should not able to get authorization code", func() {
				req, err := NewFormRequest("POST", "/api/v0/oauth/authorize", params)
				Expect(err).To(BeNil())
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("auth token is required"))
			})
		})

		Describe("when token is not full scope", func() {
			BeforeEach(func() {
				t = &token.Token{
					UserID: u.ID,
					Type:   "login",
					Scope:  token.ScopeThirdParty,
				}

				err := t.Insert(db)
				Expect(err).To(BeNil())
			})

			It("should not be able to authorize other third party", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusForbidden))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("access_denied"))
				Expect(errDesc).To(Equal("token scope not allowed to request new authorization code"))
			})
		})

		Describe("when user is logged in", func() {
			BeforeEach(func() {
				t = &token.Token{
					UserID: u.ID,
					Type:   "login",
					Scope:  token.ScopeFull,
				}
				err := t.Insert(db)
				Expect(err).To(BeNil())
			})

			Describe("when invalid parameters requested", func() {
				It("should fail if response_type is not code", func() {
					params.Set("response_type", "klingon")
					res := doRequest()
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("unsupported_response_type"))
					Expect(errDesc).To(Equal("response type must be 'code'"))
				})

				It("should fail if invalid client id is provided", func() {
					params.Set("client_id", "green")

					res := doRequest()
					Expect(res.Code).To(Equal(http.StatusNotFound))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_request"))
					Expect(errDesc).To(Equal("client ID not found"))
				})

				It("should fail if redirect URI is not subpath of original", func() {
					params.Set("redirect_uri", "http://red.co/oauth/callback")

					res := doRequest()
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_request"))
					Expect(errDesc).To(Equal("invalid redirect_uri"))
				})

				It("should fail if redirect URI different scheme", func() {
					params.Set("redirect_uri", "http://blu.co/oauth/callback")

					res := doRequest()
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_request"))
					Expect(errDesc).To(Equal("invalid redirect_uri"))
				})
			})

			Describe("when valid parameters requested", func() {
				It("should get authorization code", func() {
					j := getJsonResponse()
					Expect(j["state"]).To(Equal("anti_csrf"))
					Expect(j["code"]).ToNot(Equal(""))
					Expect(j["redirect_uri"]).To(Equal("https://blu.co/oauth/callback"))
				})

				It("should success if redirect URI is subpath of original", func() {
					params.Set("redirect_uri", "https://blu.co/oauth/callback?special=true")

					j := getJsonResponse()
					Expect(j["redirect_uri"]).To(Equal("https://blu.co/oauth/callback?special=true"))
				})

				Describe("POST /api/v0/oauth/token, grant_type=authorization_code by client", func() {
					var (
						params *url.Values
					)

					getCode := func() string {
						j := getJsonResponse()
						code := j["code"].(string)
						Expect(code).ToNot(Equal(""))
						return code
					}

					doRequest := func() *httptest.ResponseRecorder {
						req, err := NewAuthedFormRequest(t, "POST", "/api/v0/oauth/token", params)
						Expect(err).To(BeNil())
						res := httptest.NewRecorder()
						handler.ServeHTTP(res, req)
						return res
					}

					getJsonResponse := func() map[string]interface{} {
						res := doRequest()
						Expect(res.Code).To(Equal(http.StatusOK))

						var j map[string]interface{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						return j
					}

					BeforeEach(func() {
						params = &url.Values{}

						params.Set("grant_type", "authorization_code")
						params.Set("code", getCode())
						params.Set("client_id", "blu_team")
						params.Set("client_secret", "secret_sauce")
						params.Set("redirect_uri", "https://blu.co/oauth/callback")
					})

					Describe("when client request with invalid parameters", func() {
						It("should fail if requested by different client", func() {
							params.Set("client_id", "red_team")
							params.Set("client_secret", "top_secret")
							params.Set("redirect_uri", "https://red.co/oauth/callback")

							res := doRequest()
							Expect(res.Code).To(Equal(http.StatusForbidden))
							errType, errDesc := ErrorSlug(res.Body)
							Expect(errType).To(Equal("access_denied"))
							Expect(errDesc).To(Equal("client ID does not match authorization code"))
						})

						It("should fail if client secret not match", func() {
							params.Set("client_secret", "fake_secret")
							res := doRequest()
							Expect(res.Code).To(Equal(http.StatusUnauthorized))
							errType, errDesc := ErrorSlug(res.Body)
							Expect(errType).To(Equal("access_denied"))
							Expect(errDesc).To(Equal("invalid client secret"))
						})

						It("should fail if authorization code is tempered", func() {
							code := getCode()
							code = "tempered" + code
							params.Set("code", code)

							res := doRequest()
							Expect(res.Code).To(Equal(http.StatusBadRequest))
							errType, errDesc := ErrorSlug(res.Body)
							Expect(errType).To(Equal("invalid_request"))
							Expect(errDesc).To(Equal("invalid authoriazation code"))
						})

						It("should fail if authorization code is expired", func() {
							// modify global to get a negative-lived code
							originalValidity := oauth.ValidityPeriod
							oauth.ValidityPeriod = -5 * time.Second
							params.Set("code", getCode())
							oauth.ValidityPeriod = originalValidity

							res := doRequest()
							Expect(res.Code).To(Equal(http.StatusUnauthorized))
							errType, errDesc := ErrorSlug(res.Body)
							Expect(errType).To(Equal("access_denied"))
							Expect(errDesc).To(Equal("authorization code has expired"))
						})

						It("should fail if requested twice with same nonce", func() {
							res := doRequest()
							Expect(res.Code).To(Equal(http.StatusOK))

							res = doRequest()
							Expect(res.Code).To(Equal(http.StatusUnauthorized))
							errType, errDesc := ErrorSlug(res.Body)
							Expect(errType).To(Equal("access_denied"))
							Expect(errDesc).To(Equal("authorization code cannot be reused"))
						})
					})

					Describe("when client request with valid parameters", func() {
						It("should get bearer token", func() {
							j := getJsonResponse()
							Expect(j["access_token"]).ToNot(Equal(""))
							Expect(j["token_type"]).To(Equal("Bearer"))
							Expect(j["scope"]).To(Equal("third_party"))
						})

						It("should be able to gain basic user access", func() {
							j := getJsonResponse()
							token := j["access_token"].(string)
							Expect(token).ToNot(Equal(""))

							req, err := NewRequest("GET", "/api/v0/user", nil)
							Expect(err).To(BeNil())
							req.Header.Set("Authorization", "Bearer "+token)
							res := httptest.NewRecorder()
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
