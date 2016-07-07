package referralcodes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/referralcodes")
}

var _ = Describe("controllers/referralcodes", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		authTokenInputs test.AuthTokenInputs
		svr             *server.Server
		t               *token.Token
		u               *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = NewTestServer("", db, nil)
		handler = svr.Router()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "foobarbazqux",
		}
		err = t.Insert(db)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /user/referral_code", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "GET", "/api/v0/user/referral_code", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should return valid referral code json", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				ReferralCode *serializers.ReferralCode `json:"referral_code"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.ReferralCode.UserID).To(Equal(u.ID))
		})

	})
})
