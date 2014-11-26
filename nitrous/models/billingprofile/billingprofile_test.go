package billingprofile_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/billingprofile")
}

var _ = Describe("models/billingprofile", func() {
	var (
		db *db.Database
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Find()", func() {
		It("returns the billing profile for the given user ID", func() {
			p, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(p.ID).NotTo(BeNil())
			Expect(p.UserID).To(Equal(u.ID))
			Expect(p.State).To(Equal("free"))
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})

		Context("when there's no billing profile for the given user id", func() {
			It("returns nil", func() {
				p, err := billingprofile.Find(db, 9999)
				Expect(p).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				p, err := billingprofile.Find(db, u.ID)
				Expect(p).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Update()", func() {
		It("updates the profile with given parameters", func() {
			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())

			oldUpdatedAt := bp.UpdatedAt
			bp.StripeID = sql.NullString{"sc_002", true}

			err = bp.Update(db)
			Expect(err).To(BeNil())

			Expect(bp.StripeID.String).To(Equal("sc_002"))
			Expect(bp.UpdatedAt).NotTo(Equal(oldUpdatedAt))
		})
	})

	Describe("UpdateCard()", func() {
		var (
			bp         *billingprofile.BillingProfile
			fakeStripe *FakeStripe
		)

		BeforeEach(func() {
			u := &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())

			fakeStripe = &FakeStripe{}
			stripe.SetBackend(fakeStripe)
		})

		Context("when called on a profile with an emtpy StripeID", func() {
			It("returns an error", func() {
				err := bp.UpdateCard(db, "token")
				Expect(err.Error()).To(ContainSubstring("requires an existing Stripe profile"))
			})
		})

		Context("when called on a profile with a StripeID", func() {
			BeforeEach(func() {
				bp.State = "paid"
				bp.StripeID = sql.NullString{"sc_01", true}
				bp.LastFour = sql.NullString{"1212", true}
				bp.CardType = sql.NullString{"MasterCard", true}
				bp.ExpMonth = sql.NullInt64{10, true}
				bp.ExpYear = sql.NullInt64{2014, true}

				err := bp.Update(db)
				Expect(err).To(BeNil())
			})

			It("updates the record in database, if the Stripe call was successful", func() {
				fakeStripe.Response = StripeCustomerResponse

				err := bp.UpdateCard(db, "token")
				Expect(err).To(BeNil())

				Expect(bp.StripeID.String).To(Equal("sc_01"))
				Expect(bp.LastFour.String).To(Equal("4242"))
				Expect(bp.CardType.String).To(Equal("Visa"))
				Expect(bp.ExpMonth.Int64).To(Equal(int64(10)))
				Expect(bp.ExpYear.Int64).To(Equal(int64(2018)))
			})

			It("does not update the record, if Stripe returns an error", func() {
				fakeStripe.Error = &stripe.Error{
					Type: stripe.ErrorType(stripe.APIErr),
					Msg:  "some error occurred",
					Code: stripe.ErrorCode(stripe.ProcessingErr),
				}

				err := bp.UpdateCard(db, "token")
				Expect(err).NotTo(BeNil())

				// should not change existing values
				Expect(bp.StripeID.String).To(Equal("sc_01"))
				Expect(bp.LastFour.String).To(Equal("1212"))
				Expect(bp.CardType.String).To(Equal("MasterCard"))
				Expect(bp.ExpMonth.Int64).To(Equal(int64(10)))
				Expect(bp.ExpYear.Int64).To(Equal(int64(2014)))
			})
		})
	})

	Describe("UpdateEmail()", func() {
		var (
			bp         *billingprofile.BillingProfile
			fakeStripe *FakeStripe
		)

		BeforeEach(func() {
			u := &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())

			fakeStripe = &FakeStripe{}
			stripe.SetBackend(fakeStripe)
		})

		Context("when called on a profile with an emtpy StripeID", func() {
			It("returns an error", func() {
				err := bp.UpdateEmail(db, "new-email@example.com")
				Expect(err.Error()).To(ContainSubstring("requires an existing Stripe profile"))
			})
		})

		Context("when called on a profile with a StripeID", func() {
			BeforeEach(func() {
				bp.State = "paid"
				bp.StripeID = sql.NullString{"sc0001", true}
				bp.LastFour = sql.NullString{"1212", true}
				bp.CardType = sql.NullString{"MasterCard", true}
				bp.ExpMonth = sql.NullInt64{10, true}
				bp.ExpYear = sql.NullInt64{2014, true}

				err := bp.Update(db)
				Expect(err).To(BeNil())
			})

			It("does not return an error, if the Stripe call was successful", func() {
				fakeStripe.Response = fmt.Sprintf(`{
						"id": "sc0001",
						"email": "new-email@example.com",
						"cards": {
							"object": "list",
							"total_count": 1,
							"has_more": false,
							"url": "/v1/customers/cus_56dgsyZAJujFFO/cards",
							"data": [
								{
									"id": "card_14wQPX2CU2PklWtHi34KFonT",
									"object": "card",
									"last4": "4242",
									"brand": "Visa",
									"funding": "credit",
									"exp_month": 10,
									"exp_year": 2018,
									"fingerprint": "H0SJieNrXmTdq3qs",
									"country": "US",
									"name": null,
									"address_line1": null,
									"address_line2": null,
									"address_city": null,
									"address_state": null,
									"address_zip": null,
									"address_country": null,
									"cvc_check": "pass",
									"address_line1_check": null,
									"address_zip_check": null,
									"dynamic_last4": null,
									"customer": "cus_56dgsyZAJujFFO"
								}
							]
						},
						"default_card": "card_14wQPX2CU2PklWtHi34KFonT"
					}`)

				err := bp.UpdateEmail(db, "new-email@example.com")
				Expect(err).To(BeNil())
			})

			It("returns an error, if Stripe fails to update", func() {
				fakeStripe.Error = &stripe.Error{
					Type: stripe.ErrorType(stripe.APIErr),
					Msg:  "some error occurred",
					Code: stripe.ErrorCode(stripe.ProcessingErr),
				}

				err := bp.UpdateEmail(db, "new-email@example.com")
				Expect(err).NotTo(BeNil())
			})
		})
	})

})
