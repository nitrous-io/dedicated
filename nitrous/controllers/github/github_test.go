package github_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
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
func mockGetGitHubUser(env *middleware.Env, w http.ResponseWriter, r *http.Request, code string) (string, string, error) {
	return "kjk", "tokenAsJson", nil
}

var _ = Describe("controllers/github", func() {
	var (
		db  *db.Database
		svr *server.Server
		res *httptest.ResponseRecorder
		t   *token.Token
		u   *user.User
		ft  *test.FakeTracker
	)

	BeforeEach(func() {
		var err error
		github.GetGithubUserFunc = mockGetGitHubUser

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		ft = test.NewFakeTracker()
		svr = server.NewServer("", db, logger, nil, &dns.FakeProvider{})
		svr.Tracker = ft
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "foo1",
			Password: "123456",
			Name:     "foo1",
			Email:    "foo1@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "foobarbazqux",
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
		doRequest := func(uri string) {
			secret := github.MakeOauthSecretFromUserId(u.ID)
			if uri == "" {
				uri = fmt.Sprintf("/api/v0/github/oauth_cb?state=%s&code=foo", secret)
			}
			req, err := test.NewRequest("GET", uri, nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		Context("missing 'state'", func() {
			It("responds with error message", func() {
				doRequest("/api/v0/github/oauth_cb?code=foo")
				fmt.Fprintf(GinkgoWriter, "\nbody: %s\n", res.Body.String())
				Expect(res.Code).To(Equal(http.StatusTemporaryRedirect))
				Expect(res.Body.String()).To(ContainSubstring("failed to connect with GitHub"))
			})
		})

		Context("missing 'code'", func() {
			It("responds with error message", func() {
				secret := github.MakeOauthSecretFromUserId(u.ID)
				doRequest(fmt.Sprintf("/api/v0/github/oauth_cb?secret=%s", secret))
				fmt.Fprintf(GinkgoWriter, "\nbody: %s\n", res.Body.String())
				Expect(res.Code).To(Equal(http.StatusTemporaryRedirect))
				Expect(res.Body.String()).To(ContainSubstring("failed to connect with GitHub"))
			})
		})

		It("inserts github username and token to database", func() {
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusTemporaryRedirect))
			var gitHubUserName, gitHubToken sql.NullString
			err := db.QueryRow("SELECT github_user_name, github_oauth_token_json FROM user_profiles WHERE user_id = $1;", u.ID).Scan(&gitHubUserName, &gitHubToken)
			Expect(err).To(BeNil())
			Expect(gitHubUserName.String).To(Equal("kjk"))
			Expect(gitHubToken.String).To(Equal("tokenAsJson"))

			// and tracks authorization on mixpanel
			Expect(ft.PropsCalled).To(HaveLen(2))
			pc := ft.PropsCalled[0]
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.Mode).To(Equal("set"))
			Expect(pc.PropsSet["github_username"]).To(Equal("kjk"))
			pc2 := ft.PropsCalled[1]
			Expect(pc2.UserID).To(Equal(u.ID))
			Expect(pc2.Mode).To(Equal("setnx"))
			Expect(pc2.PropsSet["github_authorized_date"]).To(BeTemporally("~", time.Now(), time.Minute))
		})
	})

	Describe("GET /api/v0/github/authorize", func() {
		doRequest := func() {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/github/authorize", nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		It("returns valid json response", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j["redirect_url"]).To(ContainSubstring("https://github.com/login/oauth/authorize?access_type=online"))
		})
	})
})
