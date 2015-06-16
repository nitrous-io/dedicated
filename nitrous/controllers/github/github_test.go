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
	"github.com/nitrous-io/nitrous-common/logger"

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
		db     *db.Database
		svr    *server.Server
		t      *token.Token
		u      *user.User
		ft     *test.FakeTracker
		params *url.Values
	)

	BeforeEach(func() {
		var err error
		github.FetchGithubUser = mockFetchGitHubUser

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		ft = test.NewFakeTracker()
		svr = server.NewServer("", db, logger, nil)
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

		params = &url.Values{}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	makeState := func() string {
		state, err := github.MakeOauthState(u.ID)
		Expect(err).To(BeNil())
		return state
	}

	addStateCookie := func(req *http.Request, state string) {
		req.AddCookie(&http.Cookie{
			Name:   "github_state",
			Value:  state,
			MaxAge: github.ValidityPeriodSeconds,
		})
	}

	sendRequest := func(req *http.Request) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		svr.Router().ServeHTTP(res, req)
		return res
	}

	Describe("GET /api/v0/github/oauth_cb", func() {
		makeRequest := func() *http.Request {
			req, err := test.NewRequest("GET", "/api/v0/github/oauth_cb", params)
			Expect(err).To(BeNil())
			return req
		}

		requestWithState := func(state string) *httptest.ResponseRecorder {
			params.Set("state", state)
			req := makeRequest()
			addStateCookie(req, state)

			return sendRequest(req)
		}

		requestWithStateAndCode := func(state, code string) *httptest.ResponseRecorder {
			params.Set("code", code)
			return requestWithState(state)
		}

		It("responds with error message when state is missing", func() {
			params.Set("code", "githubcode")
			res := sendRequest(makeRequest())

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			Expect(res.Body.String()).To(ContainSubstring("invalid_state"))
		})

		It("responds with error message when state is missing", func() {
			params.Set("code", "githubcode")
			params.Set("state", "temperedstate")
			res := sendRequest(makeRequest())

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			Expect(res.Body.String()).To(ContainSubstring("invalid_state"))
		})

		It("responds with error message when state is different from cookie", func() {
			state1 := makeState()
			state2 := makeState()

			params.Set("state", state1)
			params.Set("code", "githubcode")
			req := makeRequest()
			addStateCookie(req, state2)

			res := sendRequest(makeRequest())
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			Expect(res.Body.String()).To(ContainSubstring("state_mismatch"))
		})

		It("redirects with error message when code is missing", func() {
			res := requestWithState(makeState())

			Expect(res.Code).To(Equal(http.StatusFound))
			Expect(res.Body.String()).To(ContainSubstring("invalid_code"))
		})

		It("inserts github username and token to database", func() {
			res := requestWithStateAndCode(makeState(), "foo")
			Expect(res.Code).To(Equal(http.StatusFound))

			var gitHubUserName, gitHubToken sql.NullString
			err := db.QueryRow("SELECT github_user_name, github_oauth_token_json FROM user_profiles WHERE user_id = $1;", u.ID).Scan(&gitHubUserName, &gitHubToken)
			Expect(err).To(BeNil())
			Expect(gitHubUserName.String).To(Equal("alice"))
			Expect(gitHubToken.String).To(Equal(`{ "access_token": "keytogithub" }`))

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
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/github/authorize", nil)
			Expect(err).To(BeNil())
			return sendRequest(req)
		}

		It("returns valid json response", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j["redirect_url"]).To(ContainSubstring("https://github.com/login/oauth/authorize?access_type=online"))
		})
	})
})
