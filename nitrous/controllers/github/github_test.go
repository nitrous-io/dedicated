package github_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gopkg.in/redis.v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	gh "github.com/nitrous-io/dedicated/nitrous/models/github"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	redistest "github.com/nitrous-io/dedicated/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/github")
}

// return gitHubUserName and gitHubTokenJson
func mockFetchGitHubUser(code string) (string, string, string, error) {
	return "alice", `{ "access_token": "keytogithub" }`, "alice@example.com", nil
}

type GetRedisKeyForOAuthFunc func(token string) string
type FetchReposFunc func(env *middleware.Env) ([]github.GitHubRepository, error)

var _ = Describe("controllers/github", func() {
	var (
		db      *db.Database
		svr     *server.Server
		t       *token.Token
		u       *user.User
		ft      *test.FakeTracker
		res     *httptest.ResponseRecorder
		handler *mux.Router
	)

	BeforeEach(func() {
		var err error
		github.FetchGitHubUser = mockFetchGitHubUser

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = server.NewServer("", db, nil, nil)
		svr.Tracker = ft
		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "alice",
			Password: "123456",
			Name:     "Alice",
			Email:    "alice@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "random",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("RateLimit", func() {
		var (
			env           *middleware.Env
			handlerCalled bool
		)

		BeforeEach(func() {
			env = &middleware.Env{
				DB:   db,
				User: u,
			}

			handlerCalled = false

			h := func(w http.ResponseWriter, r *http.Request) {
				mw := middleware.RateLimit(func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
					handlerCalled = true
					return nil
				})
				err := mw(env, w, r)
				if err != nil {
					svr.HandleError(env, w, r, err)
				}
			}
			handler = mux.NewRouter()
			handler.HandleFunc("/api/v0/github/{action}", h).Methods("GET")
		})

		Context("when there is no existing github record for user", func() {
			It("inserts a new record", func() {
				_, err := gh.FindByUserID(db, u.ID)
				Expect(err).ToNot(BeNil())

				req, err := test.NewRequest("GET", "/api/v0/github/repos", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(handlerCalled).To(BeTrue())

				g, err := gh.FindByUserID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(g.UserID).To(Equal(u.ID))
				Expect(g.Threshold).To(Equal(10))

				timestamps, err := g.Accesses()
				Expect(err).To(BeNil())
				Expect(timestamps).To(ConsistOf(BeNumerically("~", time.Now().Unix())))
			})
		})

		Context("when there is an existing github record for user", func() {
			BeforeEach(func() {
				_, err := gh.FindByUserID(db, u.ID)
				Expect(err).ToNot(BeNil())

				_, err = db.Exec(`INSERT INTO github (user_id, accesses_at) VALUES ($1, $2);`, u.ID, strconv.Itoa(int(time.Now().Add(-30*time.Minute).Unix())))
				Expect(err).To(BeNil())

				_, err = gh.FindByUserID(db, u.ID)
				Expect(err).To(BeNil())
			})

			It("appends unix timestamp to accesses_at string", func() {
				req, err := test.NewRequest("GET", "/api/v0/github/repos", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(handlerCalled).To(BeTrue())

				g, err := gh.FindByUserID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(g.UserID).To(Equal(u.ID))
				Expect(g.Threshold).To(Equal(10))

				timestamps, err := g.Accesses()
				Expect(err).To(BeNil())
				Expect(timestamps).To(ConsistOf(BeNumerically("~", time.Now().Add(-30*time.Minute).Unix()), BeNumerically("~", time.Now().Unix())))

			})

			It("deletes from accesses_at string if unix timestamp has expired", func() {
				g, err := gh.FindByUserID(db, u.ID)
				Expect(err).To(BeNil())
				g.Update(db, strconv.Itoa(int(time.Now().Add(-2*time.Hour).Unix())))
				Expect(err).To(BeNil())

				timestamps, err := g.Accesses()
				Expect(err).To(BeNil())
				Expect(timestamps).To(ConsistOf(BeNumerically("~", time.Now().Add(-30*time.Minute).Unix()), BeNumerically("~", time.Now().Add(-2*time.Hour).Unix())))

				req, err := test.NewRequest("GET", "/api/v0/github/repos", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(handlerCalled).To(BeTrue())

				g, err = gh.FindByUserID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(g.UserID).To(Equal(u.ID))
				Expect(g.Threshold).To(Equal(10))

				timestamps1, err := g.Accesses()
				Expect(err).To(BeNil())
				Expect(timestamps1).To(ConsistOf(BeNumerically("~", time.Now().Add(-30*time.Minute).Unix()), BeNumerically("~", time.Now().Unix())))

			})

			It("returns error if threshold is exceeded", func() {
				_, err := db.Exec(`UPDATE github SET threshold=1 WHERE user_id=$1;`, u.ID)
				Expect(err).To(BeNil())
				g, err := gh.FindByUserID(db, u.ID)
				Expect(err).To(BeNil())
				g.Update(db, strconv.Itoa(int(time.Now().Add(-30*time.Minute).Unix())))
				Expect(err).To(BeNil())

				req, err := test.NewRequest("GET", "/api/v0/github/repos", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(handlerCalled).To(BeFalse())

				Expect(res.Code).To(Equal(http.StatusForbidden))
				_, errDesc := test.ErrorSlug(res.Body)
				Expect(errDesc).To(Equal("you have exceeded the number of GitHub accesses. Please try again later"))
			})

		})
	})

	Describe("GET /api/v0/github/oauth_cb", func() {
		doRequest := func(code, state, cookieState string) *httptest.ResponseRecorder {
			params := &url.Values{}

			if code != "" {
				params.Set("code", code)
			}

			if state != "" {
				params.Set("state", state)
			}

			req, err := test.NewRequest("GET", "/api/v0/github/oauth_cb", params)
			Expect(err).To(BeNil())

			if cookieState != "" {
				req.AddCookie(&http.Cookie{
					Name:   "github_state",
					Value:  cookieState,
					MaxAge: github.ValidityPeriodSeconds,
				})
			}

			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with error message when state is missing", func() {
			res := doRequest("githubcode", "", "")

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			slug, _ := test.ErrorSlug(res.Body)
			Expect(slug).To(Equal("invalid_state"))
		})

		It("responds with error message when state is tempered", func() {
			res := doRequest("githubcode", "temperedstate", "temperedstate")

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			slug, _ := test.ErrorSlug(res.Body)
			Expect(slug).To(Equal("invalid_state"))
		})

		It("responds with error message when state is different from cookie", func() {
			state1, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, false)
			Expect(err).To(BeNil())

			state2, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, false)
			Expect(err).To(BeNil())

			res := doRequest("githubcode", state1, state2)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			slug, _ := test.ErrorSlug(res.Body)
			Expect(slug).To(Equal("state_mismatch"))
		})

		It("redirects with error message when code is missing", func() {
			state, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, false)
			Expect(err).To(BeNil())

			res := doRequest("", state, state)

			Expect(res.Code).To(Equal(http.StatusFound))
			Expect(res.Body.String()).To(ContainSubstring("invalid_code"))
		})

		It("inserts github username and token to database", func() {
			state, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, false)
			Expect(err).To(BeNil())

			res := doRequest("githubcode", state, state)
			Expect(res.Code).To(Equal(http.StatusFound))

			var gitHubUserName, gitHubToken, gitHubUserScopes sql.NullString
			err = db.QueryRow("SELECT github_user_name, github_oauth_token_json, github_user_scopes_json FROM user_profiles WHERE user_id = $1;",
				u.ID).Scan(&gitHubUserName, &gitHubToken, &gitHubUserScopes)
			Expect(err).To(BeNil())

			Expect(gitHubUserName.String).To(Equal("alice"))
			Expect(gitHubToken.String).To(Equal(`{ "access_token": "keytogithub" }`))
			Expect(gitHubUserScopes.String).To(Equal(`["user:email","repo","admin:public_key"]`))

			// and tracks authorization on mixpanel
			Expect(ft.PropsCalled).To(HaveLen(2))
			pc := ft.PropsCalled[0]
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.Mode).To(Equal("set"))
			Expect(pc.PropsSet["github_username"]).To(Equal("alice"))

			pc2 := ft.PropsCalled[1]
			Expect(pc2.UserID).To(Equal(u.ID))
			Expect(pc2.Mode).To(Equal("setnx"))
			Expect(pc2.PropsSet["github_authorized_date"]).To(BeTemporally("~", time.Now(), time.Minute))
		})

		It("redirects to app account", func() {
			state, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, false)
			Expect(err).To(BeNil())

			res := doRequest("githubcode", state, state)
			Expect(res.Code).To(Equal(http.StatusFound))

			Expect(res.Header().Get("Location")).To(Equal("/app/#/account"))
		})

		It("redirects to beta dashboard if the state is beta", func() {
			state, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, true)
			Expect(err).To(BeNil())

			envvars.BetaDashboardLocation = "/beta/#/dashboard/settings"

			res := doRequest("githubcode", state, state)
			Expect(res.Code).To(Equal(http.StatusFound))

			Expect(res.Header().Get("Location")).To(Equal("/beta/#/dashboard/settings"))
		})

		Context("purpose is 'login'", func() {
			var (
				state                      string
				err                        error
				orgGetRedisKeyForOAuthFunc GetRedisKeyForOAuthFunc
				redisClient                *redis.Client
			)

			BeforeEach(func() {
				state, err = github.MakeOAuthState(0, "tempkey", github.OAuthStatePurposeLogin, false)
				Expect(err).NotTo(HaveOccurred())
				orgGetRedisKeyForOAuthFunc = github.GetRedisKeyForOAuth
				github.GetRedisKeyForOAuth = func(token string) string {
					return "github:oauth:tempkey"
				}
				redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
				Expect(err).NotTo(HaveOccurred())

				_, err = redisClient.Set("github:oauth:tempkey", "1").Result()
				Expect(err).NotTo(HaveOccurred())

				// Remove the token
				_, err = db.Exec("DELETE FROM tokens WHERE user_id = $1;", u.ID)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				github.GetRedisKeyForOAuth = orgGetRedisKeyForOAuthFunc
				if redisClient != nil {
					redisClient.Close()
				}
			})

			Context("the user does not exist", func() {
				BeforeEach(func() {
					github.FetchGitHubUser = func(code string) (string, string, string, error) {
						return "new-github-user", `{ "access_token": "new-token" }`, "newgithub@example.com", nil
					}
				})

				It("redirects to sign up page", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/app/#/signup?gitHubUserName=new-github-user&gitHubUserEmail=newgithub@example.com"))
				})

				It("redirects to the beta signup page if the state is beta", func() {
					state, err = github.MakeOAuthState(0, "tempkey", github.OAuthStatePurposeLogin, true)
					Expect(err).NotTo(HaveOccurred())

					envvars.BetaSignupLocation = "/beta-app/#/signupnow"

					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/beta-app/#/signupnow?gitHubUserName=new-github-user&gitHubUserEmail=newgithub@example.com"))
				})

				It("keeps github info in redis", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					val, err := redisClient.Get("github:userinfo:new-github-user").Result()
					var gitHubUserInfo github.GitHubUserInfo
					err = json.Unmarshal([]byte(val), &gitHubUserInfo)
					Expect(err).NotTo(HaveOccurred())

					Expect(gitHubUserInfo.UserName).To(Equal("new-github-user"))
					Expect(gitHubUserInfo.TokenJSON).To(Equal(`{ "access_token": "new-token" }`))
				})

				It("redirects to login page if a user exists with the same email", func() {
					github.FetchGitHubUser = mockFetchGitHubUser

					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/app/#/login?existingGitHubEmail=alice@example.com"))
				})
			})

			Context("the user exists", func() {
				BeforeEach(func() {
					up, err := userprofile.Find(db, u.ID)
					Expect(err).NotTo(HaveOccurred())

					up.GitHubUserName = sql.NullString{"alice", true}
					err = up.UpdateGithubInfo(db)
					Expect(err).NotTo(HaveOccurred())
				})

				It("creates a token and redirect to login page with token", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					t, err = token.FindByUser(db, u.ID)
					Expect(err).NotTo(HaveOccurred())

					expectedUrl := fmt.Sprintf("/app/#/login?token=%s", t.Token)
					Expect(res.Header().Get("Location")).To(Equal(expectedUrl))
				})

				It("redirects to the beta login page if the state is beta", func() {
					state, err = github.MakeOAuthState(0, "tempkey", github.OAuthStatePurposeLogin, true)
					Expect(err).NotTo(HaveOccurred())

					envvars.BetaLoginLocation = "/beta-app/#/loginnow"

					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					t, err = token.FindByUser(db, u.ID)
					Expect(err).NotTo(HaveOccurred())

					expectedUrl := fmt.Sprintf("/beta-app/#/loginnow?token=%s", t.Token)
					Expect(res.Header().Get("Location")).To(Equal(expectedUrl))
				})

				Context("the redis key is expired or does not exist", func() {
					BeforeEach(func() {
						_, err := redisClient.Del("github:oauth:tempkey").Result()
						Expect(err).NotTo(HaveOccurred())
					})

					It("does not create a token and redirects to login page", func() {
						res := doRequest("githubcode", state, state)
						Expect(res.Code).To(Equal(http.StatusFound))

						t, err = token.FindByUser(db, u.ID)
						Expect(err).To(Equal(token.ErrInvalidToken))
						Expect(t).To(BeNil())

						Expect(res.Header().Get("Location")).To(Equal("/app/#/login?oauthError=true"))
					})
				})
			})
		})

		Context("purpose is 'signup'", func() {
			var (
				state                      string
				err                        error
				orgGetRedisKeyForOAuthFunc GetRedisKeyForOAuthFunc
				redisClient                *redis.Client
			)

			BeforeEach(func() {
				state, err = github.MakeOAuthState(0, "tempkey", github.OAuthStatePurposeSignUp, false)
				Expect(err).NotTo(HaveOccurred())
				orgGetRedisKeyForOAuthFunc = github.GetRedisKeyForOAuth
				github.GetRedisKeyForOAuth = func(token string) string {
					return "github:oauth:tempkey"
				}
				redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
				Expect(err).NotTo(HaveOccurred())

				_, err = redisClient.Set("github:oauth:tempkey", "1").Result()
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				github.GetRedisKeyForOAuth = orgGetRedisKeyForOAuthFunc
				if redisClient != nil {
					redisClient.Close()
				}
			})

			Context("the user does not exist", func() {
				BeforeEach(func() {
					github.FetchGitHubUser = func(code string) (string, string, string, error) {
						return "new-github-user", `{ "access_token": "new-token" }`, "newgithub@example.com", nil
					}
				})

				It("redirects to sign up page", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/app/#/signup?gitHubUserName=new-github-user&gitHubUserEmail=newgithub@example.com"))
				})

				It("redirects to the beta signup page if the state is beta", func() {
					state, err = github.MakeOAuthState(0, "tempkey", github.OAuthStatePurposeSignUp, true)
					Expect(err).NotTo(HaveOccurred())

					envvars.BetaSignupLocation = "/beta-app/#/signupnow"

					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/beta-app/#/signupnow?gitHubUserName=new-github-user&gitHubUserEmail=newgithub@example.com"))
				})

				It("keeps github info in redis", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					val, err := redisClient.Get("github:userinfo:new-github-user").Result()
					var gitHubUserInfo github.GitHubUserInfo
					err = json.Unmarshal([]byte(val), &gitHubUserInfo)
					Expect(err).NotTo(HaveOccurred())

					Expect(gitHubUserInfo.UserName).To(Equal("new-github-user"))
					Expect(gitHubUserInfo.TokenJSON).To(Equal(`{ "access_token": "new-token" }`))
				})

				It("redirects to login page if a user exists with the same email", func() {
					github.FetchGitHubUser = mockFetchGitHubUser

					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/app/#/login?existingGitHubEmail=alice@example.com"))
				})
			})

			Context("the user exist", func() {
				BeforeEach(func() {
					_, err = db.Exec("DELETE FROM tokens WHERE user_id = $1;", u.ID)
					Expect(err).NotTo(HaveOccurred())

					up, err := userprofile.Find(db, u.ID)
					Expect(err).NotTo(HaveOccurred())

					up.GitHubUserName = sql.NullString{"alice", true}
					err = up.UpdateGithubInfo(db)
					Expect(err).NotTo(HaveOccurred())
				})

				It("redirects to sign in page with token", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					t, err = token.FindByUser(db, u.ID)
					Expect(err).NotTo(HaveOccurred())

					expectedUrl := fmt.Sprintf("/app/#/login?token=%s", t.Token)
					Expect(res.Header().Get("Location")).To(Equal(expectedUrl))
				})

				It("redirects to the beta login page if the state is beta", func() {
					state, err = github.MakeOAuthState(0, "tempkey", github.OAuthStatePurposeSignUp, true)
					Expect(err).NotTo(HaveOccurred())

					envvars.BetaLoginLocation = "/beta-app/#/loginnow"

					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					t, err = token.FindByUser(db, u.ID)
					Expect(err).NotTo(HaveOccurred())

					expectedUrl := fmt.Sprintf("/beta-app/#/loginnow?token=%s", t.Token)
					Expect(res.Header().Get("Location")).To(Equal(expectedUrl))
				})
			})

			Context("the redis key is expired or does not exist", func() {
				BeforeEach(func() {
					_, err := redisClient.Del("github:oauth:tempkey").Result()
					Expect(err).NotTo(HaveOccurred())

					github.FetchGitHubUser = func(code string) (string, string, string, error) {
						return "new-github-user", `{ "access_token": "new-token" }`, "newgithub@example.com", nil
					}
				})

				It("redirects to signin page with oauthError=true", func() {
					res := doRequest("githubcode", state, state)
					Expect(res.Code).To(Equal(http.StatusFound))

					Expect(res.Header().Get("Location")).To(Equal("/app/#/signup?oauthError=true"))
				})
			})
		})
	})

	Describe("GET /api/v0/github/authorize", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/github/authorize", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should not allow authorize if token scope is not full", func() {
			_, err := db.Exec(`UPDATE tokens SET scope = 'third_party' WHERE id = $1;`, t.ID)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("returns valid json response", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			redirectURL := j["redirect_url"].(string)
			Expect(redirectURL).To(ContainSubstring("https://github.com/login/oauth/authorize?access_type=online"))

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			Expect(cookieState).ToNot(Equal(""))

			u, err := url.Parse(redirectURL)
			Expect(err).To(BeNil())

			urlState := u.Query().Get("state")
			Expect(urlState).To(Equal(cookieState))
		})

		It("reuses state cookie if valid", func() {
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/github/authorize", nil)
			Expect(err).To(BeNil())

			existingState, err := github.MakeOAuthState(u.ID, "", github.OAuthStatePurposeConnect, false)
			Expect(err).To(BeNil())

			req.AddCookie(&http.Cookie{
				Name:   "github_state",
				Value:  existingState,
				MaxAge: github.ValidityPeriodSeconds,
			})

			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(0))

			j := map[string]interface{}{}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			redirectURL := j["redirect_url"].(string)
			u, err := url.Parse(redirectURL)
			Expect(err).To(BeNil())

			urlState := u.Query().Get("state")
			Expect(urlState).To(Equal(existingState))
		})
	})

	Describe("GET /api/v0/github/authorize_to_login", func() {
		var orgGetRedisKeyForOAuthFunc GetRedisKeyForOAuthFunc

		BeforeEach(func() {
			orgGetRedisKeyForOAuthFunc = github.GetRedisKeyForOAuth
			github.GetRedisKeyForOAuth = func(token string) string {
				return "github:oauth:tempkey"
			}
		})

		AfterEach(func() {
			github.GetRedisKeyForOAuth = orgGetRedisKeyForOAuthFunc
		})

		doRequest := func(beta bool) *httptest.ResponseRecorder {
			params := &url.Values{}
			if beta {
				params.Set("beta", "true")
			}
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/github/authorize_to_login", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("returns valid json response", func() {
			res := doRequest(false)
			Expect(res.Code).To(Equal(http.StatusOK))

			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			redirectURL := j["redirect_url"].(string)
			Expect(redirectURL).To(ContainSubstring("https://github.com/login/oauth/authorize?access_type=online"))

			u, err := url.Parse(redirectURL)
			Expect(err).To(BeNil())

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			Expect(cookieState).ToNot(Equal(""))

			urlState := u.Query().Get("state")
			Expect(urlState).To(Equal(cookieState))
		})

		It("sets github state to cookie", func() {
			res := doRequest(false)
			Expect(res.Code).To(Equal(http.StatusOK))

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			state, err := github.ReadOAuthState(cookieState)
			Expect(err).NotTo(HaveOccurred())

			Expect(state.UserID).To(BeZero())
			Expect(state.Token).NotTo(BeZero())
			Expect(state.Purpose).To(Equal(github.OAuthStatePurposeLogin))
			Expect(state.Beta).To(BeFalse())
		})

		It("sets beta state if request is beta", func() {
			res := doRequest(true)
			Expect(res.Code).To(Equal(http.StatusOK))

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			state, err := github.ReadOAuthState(cookieState)
			Expect(err).NotTo(HaveOccurred())

			Expect(state.UserID).To(BeZero())
			Expect(state.Token).NotTo(BeZero())
			Expect(state.Purpose).To(Equal(github.OAuthStatePurposeLogin))
			Expect(state.Beta).To(BeTrue())
		})

		It("saves temporal key in redis with expiry", func() {
			redisClient, err := redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				redisClient.Close()
			}()

			res := doRequest(false)
			Expect(res.Code).To(Equal(http.StatusOK))

			val, err := redisClient.Get("github:oauth:tempkey").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("1"))

			ttl, err := redisClient.TTL("github:oauth:tempkey").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(ttl).To(BeNumerically("~", 10*time.Minute, 5*time.Second)) // TTL should be around 10 minutes
		})
	})

	Describe("CanDeleteGitHubPublicKeys", func() {
		var up *userprofile.UserProfile

		BeforeEach(func() {
			up = &userprofile.UserProfile{}
		})

		It("returns false when github scopes is nil", func() {
			Expect(github.CanDeleteGitHubPublicKeys(up)).To(Equal(false))
		})

		It("returns false when github scopes is empty", func() {
			up.GitHubUserScopesJson = sql.NullString{"", true}
			Expect(github.CanDeleteGitHubPublicKeys(up)).To(Equal(false))
		})

		It("returns false when github scopes does not contain `admin:public_key`", func() {
			up.GitHubUserScopesJson = sql.NullString{`["repo", "write:public_key"]`, true}
			Expect(github.CanDeleteGitHubPublicKeys(up)).To(Equal(false))
		})

		It("returns true when github scopes contains `admin:public_key`", func() {
			up.GitHubUserScopesJson = sql.NullString{`["repo", "admin:public_key"]`, true}
			Expect(github.CanDeleteGitHubPublicKeys(up)).To(Equal(true))
		})
	})

	Describe("GET /api/v0/github/authorize_to_signup", func() {
		var orgGetRedisKeyForOAuthFunc GetRedisKeyForOAuthFunc

		BeforeEach(func() {
			orgGetRedisKeyForOAuthFunc = github.GetRedisKeyForOAuth
			github.GetRedisKeyForOAuth = func(token string) string {
				return "github:oauth:tempkey"
			}
		})

		AfterEach(func() {
			github.GetRedisKeyForOAuth = orgGetRedisKeyForOAuthFunc
		})

		doRequest := func(beta bool) *httptest.ResponseRecorder {
			params := &url.Values{}
			if beta {
				params.Set("beta", "true")
			}
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/github/authorize_to_signup", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("returns valid json response", func() {
			res := doRequest(false)
			Expect(res.Code).To(Equal(http.StatusOK))

			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			redirectURL := j["redirect_url"].(string)
			Expect(redirectURL).To(ContainSubstring("https://github.com/login/oauth/authorize?access_type=online"))

			u, err := url.Parse(redirectURL)
			Expect(err).To(BeNil())

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			Expect(cookieState).ToNot(Equal(""))

			urlState := u.Query().Get("state")
			Expect(urlState).To(Equal(cookieState))
		})

		It("sets github state to cookie", func() {
			res := doRequest(false)
			Expect(res.Code).To(Equal(http.StatusOK))

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			state, err := github.ReadOAuthState(cookieState)
			Expect(err).NotTo(HaveOccurred())

			Expect(state.UserID).To(BeZero())
			Expect(state.Token).NotTo(BeZero())
			Expect(state.Purpose).To(Equal(github.OAuthStatePurposeSignUp))
		})

		It("sets beta state if request is beta", func() {
			res := doRequest(true)
			Expect(res.Code).To(Equal(http.StatusOK))

			cookies := (&http.Response{Header: res.Header()}).Cookies()
			Expect(len(cookies)).To(Equal(1))

			cookie := cookies[0]
			Expect(cookie.Name).To(Equal("github_state"))

			cookieState := cookie.Value
			state, err := github.ReadOAuthState(cookieState)
			Expect(err).NotTo(HaveOccurred())

			Expect(state.UserID).To(BeZero())
			Expect(state.Token).NotTo(BeZero())
			Expect(state.Beta).To(BeTrue())
			Expect(state.Purpose).To(Equal(github.OAuthStatePurposeSignUp))
		})

		It("saves temporal key in redis with expiry", func() {
			redisClient, err := redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				redisClient.Close()
			}()

			res := doRequest(false)
			Expect(res.Code).To(Equal(http.StatusOK))

			val, err := redisClient.Get("github:oauth:tempkey").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("1"))

			ttl, err := redisClient.TTL("github:oauth:tempkey").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(ttl).To(BeNumerically("~", 10*time.Minute, 5*time.Second)) // TTL should be around 10 minutes
		})
	})

	Describe("UpdateGitHubInfoFromRedis", func() {
		var (
			redisClient *redis.Client
			err         error
		)

		BeforeEach(func() {
			redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
			Expect(err).NotTo(HaveOccurred())

			gitHubUserInfo := github.GitHubUserInfo{
				UserName:  "github-user",
				TokenJSON: `{ "access_token": "keytogithub" }`,
			}

			b, err := json.Marshal(gitHubUserInfo)
			Expect(err).NotTo(HaveOccurred())

			_, err = redisClient.Set("github:userinfo:github-user", string(b)).Result()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if redisClient != nil {
				redisClient.Close()
			}
		})

		It("updates user profile with github info", func() {
			github.UpdateGitHubInfoFromRedis(db, u.ID, "github-user")
			up, err := userprofile.FindByGitHubUserName(db, "github-user")
			Expect(err).NotTo(HaveOccurred())

			Expect(up.GitHubUserName.String).To(Equal("github-user"))
			Expect(up.GitHubOAuthTokenJson.String).To(Equal(`{ "access_token": "keytogithub" }`))
			Expect(up.GitHubUserScopesJson.String).To(Equal(`["user:email","repo","admin:public_key"]`))
		})

		It("removes github info from redis", func() {
			github.UpdateGitHubInfoFromRedis(db, u.ID, "github-user")
			val, err := redisClient.Exists("github:userinfo:github-user").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeFalse())
		})

		Context("the github info is not present in redis", func() {
			BeforeEach(func() {
				_, err = redisClient.Del("github:userinfo:github-user").Result()
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not update user profile", func() {
				github.UpdateGitHubInfoFromRedis(db, u.ID, "github-user")
				u, err := user.FindByID(db, u.ID)
				Expect(err).NotTo(HaveOccurred())

				up, err := userprofile.Find(db, u.ID)
				Expect(err).NotTo(HaveOccurred())

				Expect(up.GitHubUserName.Valid).To(BeFalse())
			})
		})
	})

	Describe("GET /api/v0/github/repos", func() {
		var orgFetchReposFunc FetchReposFunc

		BeforeEach(func() {
			orgFetchReposFunc = github.FetchRepos
			github.FetchRepos = func(env *middleware.Env) ([]github.GitHubRepository, error) {
				var repos []github.GitHubRepository

				repo := github.GitHubRepository{
					Name:    "test-nitrous-io",
					GitURL:  "git://github.com/test-nitrous-io/test.git",
					Private: false,
				}

				repos = append(repos, repo)
				return repos, nil
			}
		})

		AfterEach(func() {
			github.FetchRepos = orgFetchReposFunc
		})

		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/github/repos", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("returns repos", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			repos := j["repos"].([]interface{})
			Expect(err).To(BeNil())
			Expect(repos).To(HaveLen(1))
			repo := repos[0].(map[string]interface{})
			Expect(repo["name"]).To(Equal("test-nitrous-io"))
			Expect(repo["git_url"]).To(Equal("git://github.com/test-nitrous-io/test.git"))
			Expect(repo["private"]).To(Equal(false))
		})

		Context("a user hasn't connected with GitHub", func() {
			BeforeEach(func() {
				github.FetchRepos = func(env *middleware.Env) ([]github.GitHubRepository, error) {
					return nil, github.ErrTokenEmpty
				}
			})

			It("returns empty body", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusForbidden))

				slug, _ := test.ErrorSlug(res.Body)
				Expect(slug).To(Equal("not_connected_with_github"))
			})
		})
	})
})
