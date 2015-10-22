package billingprofiles_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/segment"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
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
		authTokenInputs test.AuthTokenInputs
		u               *user.User
		bp              *billingprofile.BillingProfile
		t               *token.Token
		fakeStripe      *test.FakeStripe
		ft              *test.FakeTracker
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)
		ft = test.NewFakeTracker()
		svr.Tracker = ft
		handler = svr.Router()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username:  "foo",
			Password:  "123456",
			Email:     "foo@example.com",
			PromoCode: sql.NullString{credittype.SignupCreditType, true},
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		bp, err = u.BillingProfile(db)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		stripe.Key = "sk_key"
		fakeStripe = &test.FakeStripe{}
		stripe.SetBackend(fakeStripe)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/user/billing_profile", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/user/billing_profile", nil)
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
				Expect(j.BillingProfile.AmountCredited).To(Equal(int64(0)))
			})

			It("returns the amount credited if the user has a promocode and is in free", func() {
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

				_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)

				Expect(j.BillingProfile.AmountCredited).To(Equal(int64(1900)))
			})

			It("silently fails if the promocode could not be found", func() {
				_, err := db.Exec(`UPDATE users SET promo_code = 'moreMoneyzz' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)

				Expect(j.BillingProfile.AmountCredited).To(Equal(int64(0)))
			})
		}

		ItReturnsBillingProfile()

		Context("when an invoice is outstanding", func() {
			BeforeEach(func() {
				test.MarkDelinquentUser(db, u)
			})

			ItReturnsBillingProfile()
		})

		Context("when accessed as a org", func() {
			var (
				o *org.Org
			)

			BeforeEach(func() {
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(t *sqlx.Tx) error {
					Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
					return nil
				})
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).To(BeNil())
			})

			doRequest := func() *httptest.ResponseRecorder {
				url := fmt.Sprintf("/api/v0/orgs/%s/billing_profile", o.Slug)
				req, err := test.NewAuthedRequest(t, "GET", url, nil)
				Expect(err).To(BeNil())
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}

			It("should return not allow access if user is just a member", func() {
				_, err := db.Exec(`UPDATE org_memberships SET role = $1 WHERE user_id = $2 AND org_id = $3`, "member", u.ID, o.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})

			It("should return not allow access if user is just a admin", func() {
				_, err := db.Exec(`UPDATE org_memberships SET role = $1 WHERE user_id = $2 AND org_id = $3`, "admin", u.ID, o.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})

			It("should allow higher than a member to access the subscription", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET exp_month = 12, exp_year = 2019, card_type = 'AMEX', last_four = 2036 WHERE org_id = $1;`, o.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					BillingProfile *serializers.BillingProfile `json:"billing_profile"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j.BillingProfile.ExpMonth).To(Equal(int64(12)))
				Expect(j.BillingProfile.ExpYear).To(Equal(int64(2019)))
				Expect(j.BillingProfile.CardType).To(Equal("AMEX"))
				Expect(j.BillingProfile.LastFour).To(Equal("2036"))
				Expect(j.BillingProfile.AmountCredited).To(Equal(int64(0)))
			})
		})
	})

	Describe("POST /api/v0/user/billing_profiles", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/user/billing_profile", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		sharedExamples := func() {
			ItReturnsTheCreatedBillingProfile := func(expectedState string) {
				It("returns the created billing profile", func() {
					fakeStripe.Responses = []string{test.StripeCustomerResponse}

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
					Expect(j.BillingProfile.State).To(Equal(expectedState))

					b, err := billingprofile.FindByID(db, bp.ID)
					Expect(err).To(BeNil())
					Expect(b.ExpMonth.Int64).To(Equal(int64(10)))
					Expect(b.ExpYear.Int64).To(Equal(int64(2018)))
					Expect(b.CardType.String).To(Equal("Visa"))
					Expect(b.LastFour.String).To(Equal("4242"))
					Expect(b.State).To(Equal(expectedState))
				})
			}

			ItUpdatesSegmentToFreeUserDefault := func() {
				It("returns the created billing profile", func() {
					fakeStripe.Responses = []string{test.StripeCustomerResponse}

					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)

					Expect(res.Code).To(Equal(http.StatusCreated))

					b, err := billingprofile.FindByID(db, bp.ID)
					Expect(err).To(BeNil())

					u, err := user.FindByID(db, b.UserID)
					Expect(err).NotTo(HaveOccurred())

					freeUserDefaultSegmentID, err := segment.UserDefaultID(db)
					Expect(err).NotTo(HaveOccurred())

					Expect(u.SegmentID).To(Equal(freeUserDefaultSegmentID))
				})
			}

			It("should return an error when credit card token is not provided", func() {
				p := &url.Values{}
				p.Set("card_token", "")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_token_required"))
				Expect(errDesc).To(Equal("credit card token can't be blank"))
			})

			Context("when valid token is provided", func() {
				ItReturnsTheCreatedBillingProfile(billingprofile.StatePaid)
			})

			Context("when an invoice is outstanding", func() {
				BeforeEach(func() {
					test.MarkDelinquentBP(db, bp)
				})

				ItReturnsTheCreatedBillingProfile(billingprofile.StatePaid)
			})

			Context("when the customer has an existing StripeID", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET stripe_id = 'cust_1' WHERE id = $1;`, bp.ID)
					Expect(err).To(BeNil())
				})

				ItReturnsTheCreatedBillingProfile(billingprofile.StatePaid)
				ItUpdatesSegmentToFreeUserDefault()

				It("should update the existing customer object", func() {
					fakeStripe.Responses = []string{test.StripeCustomerResponse}

					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)

					Expect(res.Code).To(Equal(http.StatusCreated))

					Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
					r := fakeStripe.ReceivedRequests[0]
					Expect(r["method"]).To(Equal("POST"))
					Expect(r["path"]).To(Equal("/customers/cust_1"))
				})
			})

			Context("when credit card is valid but user is suspicious", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE users SET suspicious = true WHERE id = $1;`, u.ID)
					Expect(err).To(BeNil())
				})

				ItReturnsTheCreatedBillingProfile(billingprofile.StateDelinquent)
			})

			It("should return a card declined error when user's card is declined", func() {
				fakeStripe.Errors = []*stripe.Error{test.StripeCardDeclined}

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_declined"))
				Expect(errDesc).To(Equal("Your Credit Card was declined. Try contacting the bank directly for assistance."))
			})

			It("should return a card expired error when user's card is expired", func() {
				fakeStripe.Errors = []*stripe.Error{test.StripeCardExpired}

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_expired"))
				Expect(errDesc).To(Equal("Your Credit Card has expired. Please try again with a recent card."))

				Expect(ft.PropsCalled).To(HaveLen(0))
			})

			It("should return a incorrect cvc error when the supplied cvc is incorrect", func() {
				fakeStripe.Errors = []*stripe.Error{test.StripeIncorrectCvc}

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("incorrect_cvc"))
				Expect(errDesc).To(Equal("Your CVC is incorrect. Please try again."))

				Expect(ft.PropsCalled).To(HaveLen(0))
			})

			It("should return a incorrect zip error when the supplied zip is incorrect", func() {
				fakeStripe.Errors = []*stripe.Error{test.StripeIncorrectZip}

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("incorrect_zip"))
				Expect(errDesc).To(Equal("Your Zip code is incorrect. Please try again."))

				Expect(ft.PropsCalled).To(HaveLen(0))
			})

			It("should return an error if the card is banned", func() {
				b := &user.User{
					Username: "banner",
					Password: "123456",
					Email:    "banner@example.com",
				}
				err := test.WrapInTransaction(db, b.Insert)
				Expect(err).To(BeNil())

				u2 := &user.User{
					Username: "bad",
					Name:     "quxboy",
					Password: "123456",
					Email:    "bad@example.com",
				}
				err = test.WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET deleted_at = now(), banned_at = now(), banned_by_id = $1 WHERE id = $2;`, b.ID, u2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE billing_profiles SET stripe_fingerprint = 'H0SJieNrXmTdq3qs' WHERE user_id = $1;`, u2.ID)
				Expect(err).To(BeNil())

				fakeStripe.Responses = []string{test.StripeCustomerResponse}
				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_banned"))
				Expect(errDesc).To(Equal("Your credit card is banned."))
			})
		}

		Context("as an organisation", func() {
			var (
				o          *org.Org
				oldRequest func(params *url.Values) *httptest.ResponseRecorder
			)
			request := func(params *url.Values) *httptest.ResponseRecorder {
				url := fmt.Sprintf("/api/v0/orgs/%s/billing_profile", o.Slug)
				req, err := test.NewAuthedRequest(t, "POST", url, params)
				Expect(err).To(BeNil())
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}

			BeforeEach(func() {
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(t *sqlx.Tx) error {
					Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
					return nil
				})
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).To(BeNil())

				bp, err = billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())

				oldRequest = doRequest
				doRequest = request
			})

			AfterEach(func() {
				doRequest = oldRequest
			})

			sharedExamples()
		})

		Context("as a user", func() {
			sharedExamples()

			It("should send upgraded_to_paid_at to mixpanel", func() {
				fakeStripe.Responses = []string{test.StripeCustomerResponse}
				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)

				Expect(res.Code).To(Equal(http.StatusCreated))

				Expect(ft.PropsCalled).To(HaveLen(2))
				pc1 := ft.PropsCalled[0]
				Expect(pc1.UserID).To(Equal(u.ID))
				Expect(pc1.Mode).To(Equal("set"))
				Expect(pc1.PropsSet["upgraded_to_paid_from"]).To(Equal(billingprofile.StateNoCard))
				pc2 := ft.PropsCalled[1]
				Expect(pc2.UserID).To(Equal(u.ID))
				Expect(pc2.Mode).To(Equal("set"))
				Expect(pc2.PropsSet["upgraded_to_paid_at"]).To(BeTemporally("~", time.Now(), time.Minute))

				Expect(ft.TrackEventProperties[u.ID][0]["upgraded_to_paid_from"]).To(Equal(billingprofile.StateNoCard))
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

					fakeStripe.Responses = []string{test.StripeCustomerResponse}
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
					err := test.WrapInTransaction(db, u2.Insert)
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
	})

	Describe("PUT /api/v0/user/billing_profiles", func() {
		var doRequest func(params *url.Values) *httptest.ResponseRecorder

		sharedExamples := func() {
			It("should return an error when credit card token is not provided", func() {
				p := &url.Values{}
				p.Set("card_token", "")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("card_token_required"))
				Expect(errDesc).To(Equal("credit card token can't be blank"))
			})

			It("should return an error when trying to update an unactivated profile", func() {
				p := &url.Values{}
				p.Set("card_token", "token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("billing_profile_not_active"))
				Expect(errDesc).To(Equal("There's no existing card associated to this account."))
			})

			Context("when billing profile is active", func() {
				BeforeEach(func() {
					b, err := billingprofile.FindByID(db, bp.ID)
					Expect(err).To(BeNil())

					b.State = "paid"
					b.StripeID = sql.NullString{"sc0001", true}
					b.LastFour = sql.NullString{"1212", true}
					b.CardType = sql.NullString{"MasterCard", true}
					b.ExpMonth = sql.NullInt64{10, true}
					b.ExpYear = sql.NullInt64{2014, true}

					err = b.Update(db)
					Expect(err).To(BeNil())
				})

				It("should return a card declined error when user's card is declined'", func() {
					fakeStripe.Errors = []*stripe.Error{test.StripeCardDeclined}

					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)
					Expect(res.Code).To(Equal(http.StatusBadRequest))

					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("card_declined"))
					Expect(errDesc).To(Equal("Your Credit Card was declined. Try contacting the bank directly for assistance."))
				})

				It("should return a card expired error when user's card is expired", func() {
					fakeStripe.Errors = []*stripe.Error{test.StripeCardExpired}

					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)
					Expect(res.Code).To(Equal(http.StatusBadRequest))

					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("card_expired"))
					Expect(errDesc).To(Equal("Your Credit Card has expired. Please try again with a recent card."))
				})

				It("should return a incorrect cvc error when the supplied cvc is incorrect", func() {
					fakeStripe.Errors = []*stripe.Error{test.StripeIncorrectCvc}

					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)
					Expect(res.Code).To(Equal(http.StatusBadRequest))

					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("incorrect_cvc"))
					Expect(errDesc).To(Equal("Your CVC is incorrect. Please try again."))
				})

				It("should return a incorrect zip error when the supplied zip is incorrect", func() {
					fakeStripe.Errors = []*stripe.Error{test.StripeIncorrectZip}

					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)
					Expect(res.Code).To(Equal(http.StatusBadRequest))

					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("incorrect_zip"))
					Expect(errDesc).To(Equal("Your Zip code is incorrect. Please try again."))
				})

				It("should return an error if the card is banned", func() {
					b := &user.User{
						Username: "banner",
						Password: "123456",
						Email:    "banner@example.com",
					}
					err := test.WrapInTransaction(db, b.Insert)
					Expect(err).To(BeNil())

					u2 := &user.User{
						Username: "bad",
						Name:     "quxboy",
						Password: "123456",
						Email:    "bad@example.com",
					}
					err = test.WrapInTransaction(db, u2.Insert)
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE users SET deleted_at = now(), banned_at = now(), banned_by_id = $1 WHERE id = $2;`, b.ID, u2.ID)
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE billing_profiles SET stripe_fingerprint = 'H0SJieNrXmTdq3qs' WHERE user_id = $1;`, u2.ID)
					Expect(err).To(BeNil())

					fakeStripe.Responses = []string{test.StripeCustomerResponse}
					p := &url.Values{}
					p.Set("card_token", "cc_token")
					res := doRequest(p)

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("card_banned"))
					Expect(errDesc).To(Equal("Your credit card is banned."))
				})

				ItReturnsTheCreatedBillingProfile := func() {
					It("returns the updated billing profile", func() {
						fakeStripe.Responses = []string{test.StripeCustomerResponse}

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

						b, err := billingprofile.FindByID(db, bp.ID)
						Expect(err).To(BeNil())
						Expect(b.ExpMonth.Int64).To(Equal(int64(10)))
						Expect(b.ExpYear.Int64).To(Equal(int64(2018)))
						Expect(b.CardType.String).To(Equal("Visa"))
						Expect(b.LastFour.String).To(Equal("4242"))

						u, err := user.FindByID(db, b.UserID)
						Expect(err).NotTo(HaveOccurred())

						freeUserDefaultSegmentID, err := segment.UserDefaultID(db)
						Expect(err).NotTo(HaveOccurred())

						Expect(u.SegmentID).To(Equal(freeUserDefaultSegmentID))
					})
				}

				Context("when valid token is provided", func() {
					ItReturnsTheCreatedBillingProfile()
				})

				Context("when an invoice is outstanding", func() {
					BeforeEach(func() {
						test.MarkDelinquentUser(db, u)
					})

					ItReturnsTheCreatedBillingProfile()
				})
			})
		}

		Context("as a user", func() {
			BeforeEach(func() {
				doRequest = func(params *url.Values) *httptest.ResponseRecorder {
					req, err := test.NewAuthedRequest(t, "PUT", "/api/v0/user/billing_profile", params)
					Expect(err).To(BeNil())
					res := httptest.NewRecorder()
					handler.ServeHTTP(res, req)
					return res
				}
			})

			sharedExamples()
		})

		Context("as an organisation", func() {
			var (
				o *org.Org
			)

			BeforeEach(func() {
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(t *sqlx.Tx) error {
					Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
					return nil
				})
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o.ID, u.ID, orgmembership.RoleOwner)
				Expect(err).To(BeNil())

				bp, err = billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())

				doRequest = func(params *url.Values) *httptest.ResponseRecorder {
					url := fmt.Sprintf("/api/v0/orgs/%s/billing_profile", o.Slug)
					req, err := test.NewAuthedRequest(t, "PUT", url, params)
					Expect(err).To(BeNil())
					res := httptest.NewRecorder()
					handler.ServeHTTP(res, req)
					return res
				}
			})

			sharedExamples()

			It("should not allow a member role access", func() {
				_, err := db.Exec(`UPDATE org_memberships SET role = $1 WHERE user_id = $2;`, orgmembership.RoleMember, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})

			It("should not allow a admin role access", func() {
				_, err := db.Exec(`UPDATE org_memberships SET role = $1 WHERE user_id = $2;`, orgmembership.RoleAdmin, u.ID)
				Expect(err).To(BeNil())

				p := &url.Values{}
				p.Set("card_token", "cc_token")
				res := doRequest(p)
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})
		})
	})
})
