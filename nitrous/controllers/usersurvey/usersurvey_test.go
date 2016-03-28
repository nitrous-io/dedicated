package usersurvey_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
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
	RunSpecs(t, "controllers/usersurvey")
}

var _ = Describe("controllers/usersurvey", func() {
	var (
		db  *db.Database
		svr *server.Server
		t   *token.Token
		u   *user.User
		ft  *test.FakeTracker
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)

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

	Describe("POST /api/v0/usersurvey/webhook_cb", func() {
		doRequest := func(wufooHandshakeKey, username string) *httptest.ResponseRecorder {
			params := &url.Values{}
			params.Set("HandshakeKey", wufooHandshakeKey)
			params.Add("Field113", username)

			req, err := test.NewRequest("POST", "/api/v0/usersurvey/webhook_cb", params)
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with forbidden if handshakekey does not match", func() {
			res := doRequest("a-fake-handshake-key", "")
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("invalid handshake key"))
		})

		It("returns not found if email does not exist", func() {
			res := doRequest(envvars.WufooHandshakeKey, "an-invalid-email")
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("email_not_found"))
			Expect(errDesc).To(Equal("email not found"))
		})

		It("returns forbidden if user does not have free host", func() {
			res := doRequest(envvars.WufooHandshakeKey, u.Email)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("ineligible_for_bonus"))
			Expect(errDesc).To(Equal("you are not eligible for bonus"))
		})

		It("grants bonus and redirects", func() {
			_, err := test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: &host.Host{
					Slug: "free-host",
					Free: true,
				},
			})
			Expect(err).To(BeNil())

			res := doRequest(envvars.WufooHandshakeKey, u.Email)
			Expect(res.Code).To(Equal(http.StatusFound))
			Expect(res.HeaderMap.Get("Location")).To(ContainSubstring("earn-bonuses?completed_survey=true"))

			ebn := []bonus.EarnedBonus{}
			err = db.Select(&ebn, "SELECT * FROM earned_bonuses WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())

			var bnID int64
			err = db.Get(&bnID, "SELECT id FROM bonuses WHERE slug = 'complete-survey';")
			Expect(err).To(BeNil())

			Expect(ebn).To(HaveLen(1))
			Expect(ebn[0].UserID).To(Equal(u.ID))
			Expect(ebn[0].BonusID).To(Equal(bnID))
		})
	})

})
