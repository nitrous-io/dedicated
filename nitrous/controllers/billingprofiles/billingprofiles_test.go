package billingprofiles_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
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
		svr = server.NewServer("", db, logger, nil)
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

		fakeStripe = &FakeStripe{}
		stripe.SetBackend(fakeStripe)

		// TODO - FFLAG payments
		_, err = db.Exec("UPDATE users SET billing_enabled = true WHERE id = $1", u.ID)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/user/billing_profiles", func() {
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
			fakeStripe.SetCustomerResponse()

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)

			Expect(res.Code).To(Equal(http.StatusCreated))

			var j struct {
				BillingProfile *serializers.BillingProfile `json:"billing_profile"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			Expect(j.BillingProfile.ExpMonth).To(Equal(int64(10)))
			Expect(j.BillingProfile.ExpYear).To(Equal(int64(2018)))
			Expect(j.BillingProfile.CardType).To(Equal("Visa"))
			Expect(j.BillingProfile.LastFour).To(Equal("4242"))
		})

		It("should return a card declined error when user's card is declined'", func() {
			fakeStripe.SetCardDeclinedError()

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("card_declined"))
			Expect(errDesc).To(Equal("Your Credit Card was declined. Try contacting the bank directly for assistance."))
		})
	})

	Describe("PUT /api/v0/user/billing_profiles", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "PUT", "/api/v0/user/billing_profile", params)
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

		It("should return an error when trying to update an unactivated profile", func() {
			p := &url.Values{}
			p.Set("card_token", "token")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("credit_card_required"))
			Expect(errDesc).To(Equal("You need to add a credit card to create an application."))
		})

		Context("when billing profile is active", func() {
			BeforeEach(func() {
				bp, err := billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())

				bp.State = "paid"
				bp.StripeID = sql.NullString{"sc0001", true}
				bp.LastFour = sql.NullString{"1212", true}
				bp.CardType = sql.NullString{"MasterCard", true}
				bp.ExpMonth = sql.NullInt64{10, true}
				bp.ExpYear = sql.NullInt64{2014, true}

				err = bp.Update(db)
				Expect(err).To(BeNil())
			})

			It("should return a card declined error when user's card is declined'", func() {
				fakeStripe.SetCardDeclinedError()

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_declined"))
				Expect(errDesc).To(Equal("Your Credit Card was declined. Try contacting the bank directly for assistance."))
			})

			It("should return the created billing profile when valid token is provided", func() {
				fakeStripe.SetCustomerResponse()

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j.BillingProfile.ExpMonth).To(Equal(int64(10)))
				Expect(j.BillingProfile.ExpYear).To(Equal(int64(2018)))
				Expect(j.BillingProfile.CardType).To(Equal("Visa"))
				Expect(j.BillingProfile.LastFour).To(Equal("4242"))
			})
		})
	})
})
