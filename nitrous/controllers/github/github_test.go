package github_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/github")
}

// return gitHubUserName and gitHubTokenJson
func mockFetchGitHubUser(code string) (string, string, error) {
	return "alice", `{ "access_token": "keytogithub" }`, nil
}

var _ = Describe("controllers/github", func() {
	var (
		db  *db.Database
		svr *server.Server
		t   *token.Token
		u   *user.User
		ft  *test.FakeTracker
	)

	BeforeEach(func() {
		var err error
		github.FetchGithubUser = mockFetchGitHubUser

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = server.NewServer("", db, nil, nil)
		svr.Tracker = ft

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
			state1, err := github.MakeOAuthState(u.ID)
			Expect(err).To(BeNil())

			state2, err := github.MakeOAuthState(u.ID)
			Expect(err).To(BeNil())

			res := doRequest("githubcode", state1, state2)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			slug, _ := test.ErrorSlug(res.Body)
			Expect(slug).To(Equal("state_mismatch"))
		})

		It("redirects with error message when code is missing", func() {
			state, err := github.MakeOAuthState(u.ID)
			Expect(err).To(BeNil())

			res := doRequest("", state, state)

			Expect(res.Code).To(Equal(http.StatusFound))
			Expect(res.Body.String()).To(ContainSubstring("invalid_code"))
		})

		It("inserts github username and token to database", func() {
			state, err := github.MakeOAuthState(u.ID)
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

			existingState, err := github.MakeOAuthState(u.ID)
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
})
