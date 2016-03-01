package twitter_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mrjones/oauth"
	"github.com/nitrous-io/dedicated/nitrous/controllers/twitter"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthtoken"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/twitter")
}

func mockGetRequestTokenAndUrl(code string) (*oauth.RequestToken, string, error) {
	return &oauth.RequestToken{Token: "mock-request-token"}, "https://api.twitter.com/oauth/authorize?oauth_token=mock-oauth-token", nil
}

func mockAuthorizeToken(requestToken *oauth.RequestToken, oauthVerifier string) (*oauth.AccessToken, error) {
	return &oauth.AccessToken{Token: "mock-access-token", Secret: "mock-secret-token"}, nil
}

var _ = Describe("controllers/twitter", func() {
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
		twitter.GetRequestTokenAndUrl = mockGetRequestTokenAndUrl
		twitter.AuthorizeToken = mockAuthorizeToken

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)
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

	Describe("POST /api/v0/twitter/connect", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/twitter/connect", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should insert record in oauth_tokens", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			oauthToken := &oauthtoken.OAuthToken{}
			err := db.Get(oauthToken, "SELECT * FROM oauth_tokens;")
			Expect(err).To(BeNil())

			Expect(oauthToken.UserID).To(Equal(u.ID))
			Expect(oauthToken.RequestToken.String).To(Equal("mock-request-token"))
			Expect(oauthToken.NitrousToken.String).To(Equal("random"))
			Expect(oauthToken.Service).To(Equal("twitter"))
		})

		It("returns valid json response", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			redirectURL := j["redirect_url"].(string)
			Expect(redirectURL).To(Equal("https://api.twitter.com/oauth/authorize?oauth_token=mock-oauth-token"))
		})
	})

	Describe("GET /api/v0/twitter/oauth_cb", func() {
		doRequest := func(oauthToken, oauthVerifier string) *httptest.ResponseRecorder {
			params := &url.Values{}
			params.Set("oauth_token", oauthToken)
			params.Add("oauth_verifier", oauthVerifier)

			req, err := test.NewRequest("GET", "/api/v0/twitter/oauth_cb", params)
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			t := &oauthtoken.OAuthToken{
				UserID:       u.ID,
				Service:      oauthtoken.Twitter,
				NitrousToken: sql.NullString{"random", true},
				RequestToken: sql.NullString{"mock-request-token", true},
			}
			Expect(t.Upsert(db)).To(BeNil())
		})

		It("responds with error message when oauth param is missing", func() {
			res := doRequest("", "")

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("invalid_param"))
			Expect(errDesc).To(Equal("invalid oauth param"))
		})

		It("returns forbidden if request token does not exist in db", func() {
			res := doRequest("new-request-token", "mock-oauth-verifier")

			Expect(res.Code).To(Equal(http.StatusForbidden))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you do not have permission to access the requested resource"))
		})

		It("returns not found if bonus action does not exist in db", func() {
			res := doRequest("mock-request-token", "mock-oauth-verifier")
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("bonus_not_found"))
			Expect(errDesc).To(Equal("the bonus type is invalid"))
		})

		It("returns not found if user does not have free host", func() {
			_, err := db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('connect-twitter', 12345);`)
			Expect(err).To(BeNil())

			res := doRequest("mock-request-token", "mock-oauth-verifier")
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("user_not_found"))
			Expect(errDesc).To(Equal("user eligible for bonus not found"))
		})

		It("updates db with access token and secret and redirects", func() {
			_, err := db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('connect-twitter', 12345);`)
			Expect(err).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: &host.Host{
					Slug: "free-host",
					Free: true,
				},
			})
			Expect(err).To(BeNil())

			res := doRequest("mock-request-token", "mock-oauth-verifier")
			Expect(res.Code).To(Equal(http.StatusFound))

			oauthToken := &oauthtoken.OAuthToken{}
			err = db.Get(oauthToken, "SELECT * FROM oauth_tokens;")
			Expect(err).To(BeNil())

			Expect(oauthToken.UserID).To(Equal(u.ID))
			Expect(oauthToken.RequestToken.String).To(Equal("mock-request-token"))
			Expect(oauthToken.NitrousToken.String).To(Equal("random"))
			Expect(oauthToken.Service).To(Equal("twitter"))
			Expect(oauthToken.AccessToken.String).To(Equal("mock-access-token"))
			Expect(oauthToken.Secret.String).To(Equal("mock-secret-token"))
		})
	})

})
