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
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	stripe "github.com/stripe/stripe-go"
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
			Username:  "foo",
			Password:  "123456",
			Email:     "foo@example.com",
			PromoCode: sql.NullString{credittype.SignupCreditType, true},
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

	Describe("GET /api/v0/user/billing_profile", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", "/api/v0/user/billing_profile", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		ItReturnsBillingProfile := func() {
			It("returns billing profile", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET exp_month = 10, exp_year = 2017, card_type = 'Discover', last_four = 1234 WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j.BillingProfile.ExpMonth).To(Equal(int64(10)))
				Expect(j.BillingProfile.ExpYear).To(Equal(int64(2017)))
				Expect(j.BillingProfile.CardType).To(Equal("Discover"))
				Expect(j.BillingProfile.LastFour).To(Equal("1234"))
			})
		}

		ItReturnsBillingProfile()

		Context("when an invoice is outstanding", func() {
			BeforeEach(func() {
				MarkDelinquentUser(db, u)
			})

			ItReturnsBillingProfile()
		})
	})

	Describe("POST /api/v0/user/billing_profiles", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "POST", "/api/v0/user/billing_profile", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		ItReturnsTheCreatedBillingProfile := func() {
			It("returns the created billing profile", func() {
				fakeStripe.Responses = []string{StripeCustomerResponse}

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

		Context("when valid token is provided", func() {
			ItReturnsTheCreatedBillingProfile()
		})

		Context("when an invoice is outstanding", func() {
			BeforeEach(func() {
				MarkDelinquentUser(db, u)
			})

			ItReturnsTheCreatedBillingProfile()
		})

		It("should return a card declined error when user's card is declined", func() {
			fakeStripe.Error = StripeCardDeclined

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("card_declined"))
			Expect(errDesc).To(Equal("Your Credit Card was declined. Try contacting the bank directly for assistance."))
		})

		It("should return a card expired error when user's card is expired", func() {
			fakeStripe.Error = StripeCardExpired

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("card_expired"))
			Expect(errDesc).To(Equal("Your Credit Card has expired. Please try again with a recent card."))
		})

		It("should return a incorrect cvc error when the supplied cvc is incorrect", func() {
			fakeStripe.Error = StripeIncorrectCvc

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("incorrect_cvc"))
			Expect(errDesc).To(Equal("Your CVC is incorrect. Please try again."))
		})

		It("should return a incorrect zip error when the supplied zip is incorrect", func() {
			fakeStripe.Error = StripeIncorrectZip

			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("incorrect_zip"))
			Expect(errDesc).To(Equal("Your Zip code is incorrect. Please try again."))
		})

		It("should cancel any freequeue entries the user might have", func() {
			// Add freequeue entry for user.
			fq := &freequeue.FreeQueue{
				UserID:                u.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				RegionSlug:            "slug1",
				HostName:              "name1",
			}
			Expect(fq.Insert(db)).To(BeNil())

			fakeStripe.Responses = []string{StripeCustomerResponse}
			p := &url.Values{}
			p.Set("card_token", "cc_token")
			res := doRequest(p)

			Expect(res.Code).To(Equal(http.StatusCreated))

			_, err := freequeue.Find(db, u.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		Context("rewarding credits", func() {
			BeforeEach(func() {
				cType := &credittype.CreditType{
					Name:         credittype.SignupCreditType,
					Description:  sql.NullString{"Awarded to users for signing up", true},
					Active:       true,
					DefaultValue: 500,
				}
				Expect(cType.Insert(db)).To(BeNil())

				_, err := db.Exec(`UPDATE billing_profiles SET pending_activation = true WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())

				fakeStripe.Responses = []string{StripeCustomerResponse}
			})

			It("should not award if the billing profile was previously activated", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET pending_activation = false WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM credits WHERE user_id = $1;", u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("should not award if promo code is null", func() {
				_, err := db.Exec(`UPDATE users SET promo_code = NULL WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM credits WHERE user_id = $1;", u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("should not award if promo code is empty string", func() {
				_, err := db.Exec(`UPDATE users SET promo_code = '' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM credits WHERE user_id = $1;", u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("should award if the billing profile was not previously activated", func() {
				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j.BillingProfile.AmountCredited).To(Equal(int64(500)))

				credit := &credit.Credit{}
				err = db.Get(credit, "SELECT * FROM credits WHERE user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(credit).ToNot(BeNil())
				Expect(credit.Amount).To(Equal(int64(500)))

				var used int64
				err = db.QueryRow(`SELECT used FROM credit_types WHERE name = $1;`, credittype.SignupCreditType).Scan(&used)
				Expect(err).To(BeNil())
				Expect(used).To(Equal(int64(1)))
			})

			It("should award credits if a promo code if provided", func() {
				c := &credittype.CreditType{
					Name:         "moreMoney",
					Description:  sql.NullString{"Desc", true},
					Active:       true,
					DefaultValue: 1900,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET promo_code = 'moreMoney' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j.BillingProfile.AmountCredited).To(Equal(int64(1900)))

				credit := &credit.Credit{}
				err = db.Get(credit, "SELECT * FROM credits WHERE user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(credit).ToNot(BeNil())
				Expect(credit.Amount).To(Equal(int64(1900)))

				var used int64
				err = db.QueryRow(`SELECT used FROM credit_types WHERE id = $1;`, c.ID).Scan(&used)
				Expect(err).To(BeNil())
				Expect(used).To(Equal(int64(1)))
			})

			It("should not award credits if a promo code is incorrect", func() {
				c := &credittype.CreditType{
					Name:         "moreMoney",
					Description:  sql.NullString{"Desc", true},
					Active:       false,
					DefaultValue: 1900,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET promo_code = 'moreMoney' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				credit := &credit.Credit{}
				err = db.Get(credit, "SELECT * FROM credits WHERE user_id = $1;", u.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
			})

			It("should not award if there is an existing credit card with the same fingerprint", func() {
				u2 := &user.User{
					Username: "fooey",
					Password: "123456",
					Email:    "foo2@example.com",
				}
				err := WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())
				_, err = db.Exec(`UPDATE billing_profiles SET stripe_fingerprint = 'H0SJieNrXmTdq3qs' WHERE user_id = $1;`, u2.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM credits WHERE user_id = $1;", u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})
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
			Expect(errType).To(Equal("billing_profile_not_active"))
			Expect(errDesc).To(Equal("There's no existing card associated to this account."))
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
				fakeStripe.Error = StripeCardDeclined

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_declined"))
				Expect(errDesc).To(Equal("Your Credit Card was declined. Try contacting the bank directly for assistance."))
			})

			It("should return a card expired error when user's card is expired", func() {
				fakeStripe.Error = StripeCardExpired

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_expired"))
				Expect(errDesc).To(Equal("Your Credit Card has expired. Please try again with a recent card."))
			})

			It("should return a incorrect cvc error when the supplied cvc is incorrect", func() {
				fakeStripe.Error = StripeIncorrectCvc

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("incorrect_cvc"))
				Expect(errDesc).To(Equal("Your CVC is incorrect. Please try again."))
			})

			It("should return a incorrect zip error when the supplied zip is incorrect", func() {
				fakeStripe.Error = StripeIncorrectZip

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("incorrect_zip"))
				Expect(errDesc).To(Equal("Your Zip code is incorrect. Please try again."))
			})

			ItReturnsTheCreatedBillingProfile := func() {
				It("returns the created billing profile", func() {
					fakeStripe.Responses = []string{StripeCustomerResponse}

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
			}

			Context("when valid token is provided", func() {
				ItReturnsTheCreatedBillingProfile()
			})

			Context("when an invoice is outstanding", func() {
				BeforeEach(func() {
					MarkDelinquentUser(db, u)
				})

				ItReturnsTheCreatedBillingProfile()
			})
		})
	})
})
