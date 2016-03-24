package quickstart_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/nitrous/controllers/quickstart"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/quickstart")
}

var _ = Describe("controllers/quickstart", func() {
	var (
		db      *db.Database
		handler *mux.Router
		svr     *server.Server
		ft      *test.FakeTracker
		hs      *test.HostStack
		t       *token.Token
		tmpl    *template.Template
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)
		handler = svr.Router()

		quickstart.GithubRepoClient = &test.FakeRepoClient{}

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:          "foo",
				IPAddress:     sql.NullString{"127.0.0.1", true},
				MaxContainers: sql.NullInt64{1, true},
			},
		})
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: hs.U.ID,
			Token:  "minitoken",
		}
		Expect(t.Insert(db)).To(BeNil())

		tmpl = &template.Template{
			Slug:        "rails",
			Name:        sql.NullString{"Ruby on Rails", true},
			Description: sql.NullString{"A ruby on rails stack template", true},
			Image:       "rails:latest",
			Config: `{
				"privileged": true,
				"publish_all": false,
				"ports": ["public::2341", "public::8931"],
				"env": ["SHELL=powershell"]
			}`,
			Type: template.TypeStack,
		}
		Expect(tmpl.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /quickstart", func() {
		doRequest := func(params *url.Values, referer string) *httptest.ResponseRecorder {
			var err error
			req, err := test.NewRequest("GET", "/quickstart", params)
			req.Header.Set("Referer", referer)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("redirects to quickstart dashboard with repo url value from query string", func() {
			params := &url.Values{}
			params.Set("repo", "https://github.com/yeouchien/hack.git")
			res := doRequest(params, "")
			Expect(res.Code).To(Equal(http.StatusMovedPermanently))
			redirectURL := "/app/#/dashboard/quickstart?repo=https%3A%2F%2Fgithub.com%2Fyeouchien%2Fhack.git"
			Expect(res.Header().Get("Location")).To(Equal(redirectURL))
		})

		It("redirects to quickstart dashboard with repo url value from http referer", func() {
			res := doRequest(nil, "https://github.com/yeouchien/hack.git")
			Expect(res.Code).To(Equal(http.StatusMovedPermanently))
			redirectURL := "/app/#/dashboard/quickstart?repo=https%3A%2F%2Fgithub.com%2Fyeouchien%2Fhack.git"
			Expect(res.Header().Get("Location")).To(Equal(redirectURL))
		})

		It("returns error if http referer and repo query string are missing", func() {
			res := doRequest(nil, "")
			Expect(res.Code).To(Equal(http.StatusForbidden))
			Expect(res.Body.String()).To(ContainSubstring("Invalid repository URL"))
		})
	})

	Describe("GET /api/v0/quickstart/repo_info", func() {
		doRequest := func(repoUrl string) *httptest.ResponseRecorder {
			params := &url.Values{}
			if repoUrl != "" {
				params.Set("repo", repoUrl)
			}
			req, err := test.NewRequest("GET", "/api/v0/quickstart/repo_info", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.Token))
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("responds with repository information", func() {
			_, err := db.Exec(`UPDATE users
				SET confirmation_sent_at = now()
				WHERE id = $1;`, hs.U.ID)
			Expect(err).To(BeNil())

			res := doRequest("https://github.com/yeouchien/hack.git")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Quickstart struct {
					Owner       string  `json:"owner"`
					OwnerAvatar string  `json:"owner_avatar"`
					RepoName    string  `json:"repo_name"`
					Template    string  `json:"template"`
					Ports       []int64 `json:"ports"`
				} `json:"quickstart"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Quickstart.Template).To(Equal("rails"))
			Expect(j.Quickstart.Owner).To(Equal("owner"))
			Expect(j.Quickstart.OwnerAvatar).To(Equal("https://example.com/rails.png"))
			Expect(j.Quickstart.RepoName).To(Equal("repo-name"))
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

			res := doRequest("https://github.com/yeouchien/hack.git")
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

		It("returns invalid json error if nitrous.json is invalid", func() {
			_, err := db.Exec(`UPDATE users
				SET confirmation_sent_at = now()
				WHERE id = $1;`, hs.U.ID)
			Expect(err).To(BeNil())

			quickstart.GithubRepoClient = &test.FakeRepoClient{
				Content: `{
					"template": "rails",,
					"ports": [8000, 9000, 10000],,,,,
				}`,
			}

			res := doRequest("https://github.com/yeouchien/hack.git")
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("Invalid nitrous.json"))
		})

		It("returns error if template slug does not exist", func() {
			_, err := db.Exec(`UPDATE users
				SET confirmation_sent_at = now()
				WHERE id = $1;`, hs.U.ID)
			Expect(err).To(BeNil())

			quickstart.GithubRepoClient = &test.FakeRepoClient{
				Content: `{
					"template": "ruby-on-rails",
					"ports": [8000, 9000, 10000]
				}`,
			}

			res := doRequest("https://github.com/yeouchien/hack.git")
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("Invalid repository info"))
		})
	})
})
