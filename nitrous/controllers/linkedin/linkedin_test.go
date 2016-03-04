package linkedin_test

//
//import (
//	"database/sql"
//	"encoding/json"
//	"net/http"
//	"net/http/httptest"
//	"net/url"
//	"testing"
//
//	"github.com/gorilla/mux"
//	"github.com/nitrous-io/dedicated/nitrous/controllers/linkedin"
//	"github.com/nitrous-io/dedicated/nitrous/db"
//	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
//	"github.com/nitrous-io/dedicated/nitrous/models/host"
//	"github.com/nitrous-io/dedicated/nitrous/models/oauthtoken"
//	"github.com/nitrous-io/dedicated/nitrous/models/token"
//	"github.com/nitrous-io/dedicated/nitrous/models/user"
//	"github.com/nitrous-io/dedicated/nitrous/server"
//	"github.com/nitrous-io/dedicated/nitrous/test"
//	"golang.org/x/oauth2"
//	"golang.org/x/oauth2/linkedin"
//
//	. "github.com/onsi/ginkgo"
//	. "github.com/onsi/gomega"
//)
//
//func Test(t *testing.T) {
//	RegisterFailHandler(Fail)
//	RunSpecs(t, "controllers/twitter")
//}
//
//func mockGetRequestTokenAndUrl(string) (*oauth.RequestToken, string, error) {
//	return &oauth.RequestToken{Token: "mock-request-token"}, "https://api.twitter.com/oauth/authorize?oauth_token=mock-oauth-token", nil
//}
//
//func mockAuthorizeToken(*oauth.RequestToken, string) (*oauth.AccessToken, error) {
//	return &oauth.AccessToken{Token: "mock-access-token", Secret: "mock-secret-token"}, nil
//}
//
//func mockCreateFriendship(*http.Client) (*http.Response, error) {
//	return &http.Response{StatusCode: http.StatusOK}, nil
//}
//
//func mockPostStatus(string, *http.Client) (*http.Response, error) {
//	return &http.Response{StatusCode: http.StatusOK}, nil
//}
//
//var _ = Describe("controllers/twitter", func() {
//	var (
//		db      *db.Database
//		svr     *server.Server
//		t       *token.Token
//		u       *user.User
//		ft      *test.FakeTracker
//		res     *httptest.ResponseRecorder
//		handler *mux.Router
//	)
//
//	BeforeEach(func() {
//		var err error
//		twitter.GetRequestTokenAndUrl = mockGetRequestTokenAndUrl
//		twitter.AuthorizeToken = mockAuthorizeToken
//		twitter.CreateFriendship = mockCreateFriendship
//		twitter.PostStatus = mockPostStatus
//
//		db, err = test.TestDatabase("nitrous_test")
//		Expect(err).To(BeNil())
//		ft = test.NewFakeTracker()
//		svr = test.NewTestServer("", db, ft)
//		handler = svr.Router()
//		res = httptest.NewRecorder()
//
//		u = &user.User{
//			Username: "alice",
//			Password: "123456",
//			Name:     "Alice",
//			Email:    "alice@example.com",
//		}
//		err = test.WrapInTransaction(db, u.Insert)
//		Expect(err).To(BeNil())
//
//		t = &token.Token{
//			UserID: u.ID,
//			Token:  "random",
//		}
//		err = t.Insert(db)
//		Expect(err).To(BeNil())
//	})
//
//	AfterEach(func() {
//		if db != nil {
//			db.Close()
//		}
//	})
//
//	Describe("POST /api/v0/twitter/connect", func() {
//		doRequest := func() *httptest.ResponseRecorder {
//			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/twitter/connect", nil)
//			Expect(err).To(BeNil())
//			res := httptest.NewRecorder()
//			svr.Router().ServeHTTP(res, req)
//			return res
//		}
//
//		It("should insert record in oauth_tokens", func() {
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusOK))
//
//			oauthToken := &oauthtoken.OAuthToken{}
//			err := db.Get(oauthToken, "SELECT * FROM oauth_tokens;")
//			Expect(err).To(BeNil())
//
//			Expect(oauthToken.UserID).To(Equal(u.ID))
//			Expect(oauthToken.RequestToken.String).To(Equal("mock-request-token"))
//			Expect(oauthToken.NitrousToken.String).To(Equal("random"))
//			Expect(oauthToken.Service).To(Equal("twitter"))
//		})
//
//		It("returns valid json response", func() {
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusOK))
//
//			j := map[string]interface{}{}
//			err := json.NewDecoder(res.Body).Decode(&j)
//			Expect(err).To(BeNil())
//
//			redirectURL := j["redirect_url"].(string)
//			Expect(redirectURL).To(Equal("https://api.twitter.com/oauth/authorize?oauth_token=mock-oauth-token"))
//		})
//	})
//
//	Describe("GET /api/v0/twitter/oauth_cb", func() {
//		doRequest := func(oauthToken, oauthVerifier string) *httptest.ResponseRecorder {
//			params := &url.Values{}
//			params.Set("oauth_token", oauthToken)
//			params.Add("oauth_verifier", oauthVerifier)
//
//			req, err := test.NewRequest("GET", "/api/v0/twitter/oauth_cb", params)
//			Expect(err).To(BeNil())
//
//			res := httptest.NewRecorder()
//			svr.Router().ServeHTTP(res, req)
//			return res
//		}
//
//		BeforeEach(func() {
//			t := &oauthtoken.OAuthToken{
//				UserID:       u.ID,
//				Service:      oauthtoken.Twitter,
//				NitrousToken: sql.NullString{"random", true},
//				RequestToken: sql.NullString{"mock-request-token", true},
//			}
//			Expect(t.Upsert(db)).To(BeNil())
//		})
//
//		It("responds with error message when oauth param is missing", func() {
//			res := doRequest("", "")
//			Expect(res.Code).To(Equal(http.StatusBadRequest))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("invalid_param"))
//			Expect(errDesc).To(Equal("invalid oauth param"))
//		})
//
//		It("returns forbidden if request token does not exist in db", func() {
//			res := doRequest("new-request-token", "mock-oauth-verifier")
//			Expect(res.Code).To(Equal(http.StatusForbidden))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("forbidden"))
//			Expect(errDesc).To(Equal("you do not have permission to access the requested resource"))
//		})
//
//		It("returns not found if bonus action does not exist in db", func() {
//			res := doRequest("mock-request-token", "mock-oauth-verifier")
//			Expect(res.Code).To(Equal(http.StatusNotFound))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("bonus_not_found"))
//			Expect(errDesc).To(Equal("the bonus type is invalid"))
//		})
//
//		It("returns not found if user does not have free host", func() {
//			_, err := db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('connect-twitter', 12345);`)
//			Expect(err).To(BeNil())
//
//			res := doRequest("mock-request-token", "mock-oauth-verifier")
//			Expect(res.Code).To(Equal(http.StatusNotFound))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("user_not_found"))
//			Expect(errDesc).To(Equal("user eligible for bonus not found"))
//		})
//
//		It("updates db with access token and secret and redirects", func() {
//			_, err := db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('connect-twitter', 12345);`)
//			Expect(err).To(BeNil())
//
//			_, err = test.CreateHostStack(db, &test.HostStack{
//				U: u,
//				H: &host.Host{
//					Slug: "free-host",
//					Free: true,
//				},
//			})
//			Expect(err).To(BeNil())
//
//			res := doRequest("mock-request-token", "mock-oauth-verifier")
//			Expect(res.Code).To(Equal(http.StatusFound))
//
//			oauthToken := &oauthtoken.OAuthToken{}
//			err = db.Get(oauthToken, "SELECT * FROM oauth_tokens;")
//			Expect(err).To(BeNil())
//
//			Expect(oauthToken.UserID).To(Equal(u.ID))
//			Expect(oauthToken.RequestToken.String).To(Equal("mock-request-token"))
//			Expect(oauthToken.NitrousToken.String).To(Equal("random"))
//			Expect(oauthToken.Service).To(Equal("twitter"))
//			Expect(oauthToken.AccessToken.String).To(Equal("mock-access-token"))
//			Expect(oauthToken.Secret.String).To(Equal("mock-secret-token"))
//		})
//	})
//
//	Describe("POST /api/v0/twitter/follow", func() {
//		doRequest := func() *httptest.ResponseRecorder {
//			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/twitter/follow", nil)
//			Expect(err).To(BeNil())
//			res := httptest.NewRecorder()
//			svr.Router().ServeHTTP(res, req)
//			return res
//		}
//
//		It("updates and serializes earned_bonuses", func() {
//			_, err := db.Exec(`INSERT INTO oauth_tokens (user_id, service, nitrous_token, request_token, access_token, secret) VALUES ($1, 'twitter', 'random', 'mock-request-token', 'mock-access-token', 'mock-token-secret');`, u.ID)
//			Expect(err).To(BeNil())
//
//			_, err = db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('follow-twitter', 12345);`)
//			Expect(err).To(BeNil())
//
//			_, err = test.CreateHostStack(db, &test.HostStack{
//				U: u,
//				H: &host.Host{
//					Slug: "free-host",
//					Free: true,
//				},
//			})
//			Expect(err).To(BeNil())
//
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusOK))
//
//			ebn := []bonus.EarnedBonus{}
//			err = db.Select(&ebn, "SELECT * FROM earned_bonuses WHERE user_id = $1;", u.ID)
//			Expect(err).To(BeNil())
//
//			var bnID int64
//			err = db.Get(&bnID, "SELECT id FROM bonuses WHERE slug = 'follow-twitter';")
//			Expect(err).To(BeNil())
//
//			Expect(ebn).To(HaveLen(1))
//			Expect(ebn[0].UserID).To(Equal(u.ID))
//			Expect(ebn[0].BonusID).To(Equal(bnID))
//		})
//
//		It("returns error if there is no oauth token record", func() {
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusNotFound))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("token_not_found"))
//			Expect(errDesc).To(Equal("token not found, please connect with twitter"))
//		})
//
//		It("returns error if there is no bonus record", func() {
//			_, err := db.Exec(`INSERT INTO oauth_tokens (user_id, service, nitrous_token, request_token, access_token, secret) VALUES ($1, 'twitter', 'random', 'mock-request-token', 'mock-access-token', 'mock-token-secret');`, u.ID)
//			Expect(err).To(BeNil())
//
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusNotFound))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("bonus_not_found"))
//			Expect(errDesc).To(Equal("the bonus type is invalid"))
//		})
//
//		It("returns error if the bonus is already earned", func() {
//			_, err := db.Exec(`INSERT INTO oauth_tokens (user_id, service, nitrous_token, request_token, access_token, secret) VALUES ($1, 'twitter', 'random', 'mock-request-token', 'mock-access-token', 'mock-token-secret');`, u.ID)
//			Expect(err).To(BeNil())
//
//			_, err = db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('follow-twitter', 12345);`)
//			Expect(err).To(BeNil())
//
//			_, err = test.CreateHostStack(db, &test.HostStack{
//				U: u,
//				H: &host.Host{
//					Slug: "free-host",
//					Free: true,
//				},
//			})
//			Expect(err).To(BeNil())
//
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusOK))
//
//			res1 := doRequest()
//			Expect(res1.Code).To(Equal(http.StatusForbidden))
//
//			errSlug, errDesc := test.ErrorSlug(res1.Body)
//			Expect(errSlug).To(Equal("forbidden"))
//			Expect(errDesc).To(Equal("you have already earned this bonus"))
//		})
//	})
//
//	Describe("POST /api/v0/twitter/tweet", func() {
//		doRequest := func() *httptest.ResponseRecorder {
//			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/twitter/tweet", nil)
//			Expect(err).To(BeNil())
//			res := httptest.NewRecorder()
//			svr.Router().ServeHTTP(res, req)
//			return res
//		}
//
//		It("updates and serializes earned_bonuses", func() {
//			_, err := db.Exec(`INSERT INTO oauth_tokens (user_id, service, nitrous_token, request_token, access_token, secret) VALUES ($1, 'twitter', 'random', 'mock-request-token', 'mock-access-token', 'mock-token-secret');`, u.ID)
//			Expect(err).To(BeNil())
//
//			_, err = db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('tweet', 12345);`)
//			Expect(err).To(BeNil())
//
//			_, err = test.CreateHostStack(db, &test.HostStack{
//				U: u,
//				H: &host.Host{
//					Slug: "free-host",
//					Free: true,
//				},
//			})
//			Expect(err).To(BeNil())
//
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusOK))
//
//			ebn := []bonus.EarnedBonus{}
//			err = db.Select(&ebn, "SELECT * FROM earned_bonuses WHERE user_id = $1;", u.ID)
//			Expect(err).To(BeNil())
//
//			var bnID int64
//			err = db.Get(&bnID, "SELECT id FROM bonuses WHERE slug = 'tweet';")
//			Expect(err).To(BeNil())
//
//			Expect(ebn).To(HaveLen(1))
//			Expect(ebn[0].UserID).To(Equal(u.ID))
//			Expect(ebn[0].BonusID).To(Equal(bnID))
//		})
//
//		It("returns error if there is no oauth token record", func() {
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusNotFound))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("token_not_found"))
//			Expect(errDesc).To(Equal("token not found, please connect with twitter"))
//		})
//
//		It("returns error if there is no bonus record", func() {
//			_, err := db.Exec(`INSERT INTO oauth_tokens (user_id, service, nitrous_token, request_token, access_token, secret) VALUES ($1, 'twitter', 'random', 'mock-request-token', 'mock-access-token', 'mock-token-secret');`, u.ID)
//			Expect(err).To(BeNil())
//
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusNotFound))
//
//			errSlug, errDesc := test.ErrorSlug(res.Body)
//			Expect(errSlug).To(Equal("bonus_not_found"))
//			Expect(errDesc).To(Equal("the bonus type is invalid"))
//		})
//
//		It("returns error if the bonus is already earned", func() {
//			_, err := db.Exec(`INSERT INTO oauth_tokens (user_id, service, nitrous_token, request_token, access_token, secret) VALUES ($1, 'twitter', 'random', 'mock-request-token', 'mock-access-token', 'mock-token-secret');`, u.ID)
//			Expect(err).To(BeNil())
//
//			_, err = db.Exec(`INSERT INTO bonuses (slug, amount) VALUES ('tweet', 12345);`)
//			Expect(err).To(BeNil())
//
//			_, err = test.CreateHostStack(db, &test.HostStack{
//				U: u,
//				H: &host.Host{
//					Slug: "free-host",
//					Free: true,
//				},
//			})
//			Expect(err).To(BeNil())
//
//			res := doRequest()
//			Expect(res.Code).To(Equal(http.StatusOK))
//
//			res1 := doRequest()
//			Expect(res1.Code).To(Equal(http.StatusForbidden))
//
//			errSlug, errDesc := test.ErrorSlug(res1.Body)
//			Expect(errSlug).To(Equal("forbidden"))
//			Expect(errDesc).To(Equal("you have already earned this bonus"))
//		})
//	})
//
//})
