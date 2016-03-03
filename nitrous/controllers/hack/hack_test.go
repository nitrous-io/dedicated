package hack_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/nitrous/controllers/hack"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hack")
}

var _ = Describe("controllers/hack", func() {
	var (
		db      *db.Database
		handler *mux.Router
		svr     *server.Server
		ft      *test.FakeTracker
		hs      *test.HostStack
		t       *token.Token
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)
		handler = svr.Router()
		hack.GithubRepoClient = &test.FakeRepoClient{}

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:          "foo",
				IPAddress:     sql.NullString{"127.0.0.1", true},
				MaxContainers: sql.NullInt64{1, true},
			},
		})

		t = &token.Token{
			UserID: hs.U.ID,
			Token:  "minitoken",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /project/new", func() {
		doRequest := func(repo string) *httptest.ResponseRecorder {
			var err error
			params := &url.Values{}
			if repo != "" {
				params.Set("repo", repo)
			}
			req, err := test.NewRequest("GET", "/project/new", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("redirects to hack dashboard with repo url", func() {
			res := doRequest("https://github.com/yeouchien/hack.git")
			Expect(res.Code).To(Equal(http.StatusMovedPermanently))
			redirectURL := "/app/#/dashboard/hack?repo=https%3A%2F%2Fgithub.com%2Fyeouchien%2Fhack.git"
			Expect(res.Header().Get("Location")).To(Equal(redirectURL))
		})
	})

	Describe("GET /api/v0/hack/repo_info", func() {
		doRequest := func(repoUrl, token string) *httptest.ResponseRecorder {
			params := &url.Values{}
			if repoUrl != "" {
				params.Set("repo", repoUrl)
			}
			req, err := test.NewRequest("GET", "/api/v0/hack/repo_info", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("responds with repository information", func() {
			_, err := db.Exec(`UPDATE users
				SET confirmation_sent_at = now()
				WHERE id = $1;`, hs.U.ID)
			Expect(err).To(BeNil())

			res := doRequest("https://github.com/yeouchien/hack.git", t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Hack struct {
					Owner    string  `json:"owner"`
					RepoName string  `json:"repo_name"`
					Template string  `json:"template"`
					Ports    []int64 `json:"ports"`
				} `json:"hack"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Hack.Template).To(Equal("rails"))
			Expect(j.Hack.Owner).To(Equal("owner"))
			Expect(j.Hack.RepoName).To(Equal("repo-name"))
		})

		It("confirms user automatically if they have not yet confirmed their email", func() {
			_, err := db.Exec(`UPDATE users
				SET confirmed_at = null, confirmation_sent_at = now(),
					confirmation_token = 'token'
				WHERE id = $1;`, hs.U.ID)
			Expect(err).To(BeNil())

			u, err := user.FindByID(db, hs.U.ID)
			Expect(err).To(BeNil())

			Expect(u.ConfirmedAt.Valid).ToNot(Equal(true))

			res := doRequest("https://github.com/yeouchien/hack.git", t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			u, err = user.FindByID(db, hs.U.ID)
			Expect(err).To(BeNil())

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			Expect(u.ConfirmedAt.Valid).To(Equal(true))
			Expect(u.ConfirmationSentAt.Valid).To(Equal(false))
			Expect(u.ConfirmationToken.Valid).To(Equal(false))
			Expect(bp.State).To(Equal(billingprofile.StateFree))
		})
	})
})
