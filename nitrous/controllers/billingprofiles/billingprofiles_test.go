package billingprofiles_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/billingprofiles")
}

var _ = Describe("controllers/billingprofiles", func() {
	var (
		db      *db.Database
		handler *mux.Router

		svr             *server.Server
		authTokenInputs AuthTokenInputs
		u               *user.User
		t               *token.Token
		fakeStripe      *FakeStripe
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		stripe.Key = "sk_key"
		fakeStripe = &FakeStripe{}
		stripe.SetBackend(fakeStripe)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/users/{id}/billing_profiles", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "POST", "/api/v0/user/billing_profile", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should return an error when credit card token is not provided", func() {
			p := &url.Values{}
			p.Set("card_token", "")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("card_token_required"))
			Expect(errDesc).To(Equal("credit card token can't be blank"))
		})

		It("should return the created billing profile when valid token is provided", func() {
			fakeStripe.Response = fmt.Sprintf(`{
					"id": "sc_01",
					"email": "%s",
					"default_card": {
						"exp_month": 10,
						"exp_year": 2018,
						"brand": "Visa",
						"last4": "4242"
					}
				}`, u.Email)

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)

			Expect(res.Code).To(Equal(http.StatusCreated))

			var j struct {
				BillingProfile *serializers.BillingProfile `json:"billing_profile"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			Expect(j.BillingProfile.ExpMonth).To(Equal(uint8(10)))
			Expect(j.BillingProfile.ExpYear).To(Equal(uint16(2018)))
			Expect(j.BillingProfile.CardType).To(Equal("Visa"))
			Expect(j.BillingProfile.LastFour).To(Equal("4242"))
		})
	})
})
