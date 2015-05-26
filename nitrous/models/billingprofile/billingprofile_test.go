package billingprofile_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
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
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
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
			Expect(p.State).To(Equal("no_card"))
			Expect(p.PendingActivation).To(BeTrue())
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})

		It("billing profile should be cached when called with user.BillingProfile()", func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`DELETE FROM billing_profiles WHERE user_id=$1;`, u.ID)
			Expect(err).To(BeNil())

			// test that calling it doesn't hit the database and get error
			bp2, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			Expect(bp2).To(Equal(bp))

			// test that normal Find() still hit the database and get error
			_, err = billingprofile.Find(db, u.ID)
			Expect(err).ToNot(BeNil())
		})

		Context("when there's no billing profile for the given user id", func() {
			It("returns error", func() {
				p, err := billingprofile.Find(db, 9999)
				Expect(p).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
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
			fakeStripe *test.FakeStripe
		)

		BeforeEach(func() {
			u := &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())

			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)
		})

		Context("when called on a profile with an empty StripeID", func() {
			It("returns StripeProfileRequiredError", func() {
				err := bp.UpdateCard(db, "token")
				Expect(err).To(Equal(billingprofile.StripeProfileRequiredError))
			})
		})

		Context("when called on a profile with a StripeID", func() {
			BeforeEach(func() {
				bp.State = "paid"
				bp.StripeID = sql.NullString{"sc_01", true}
				bp.StripeFingerprint = sql.NullString{"PRINT", true}
				bp.LastFour = sql.NullString{"1212", true}
				bp.CardType = sql.NullString{"MasterCard", true}
				bp.ExpMonth = sql.NullInt64{10, true}
				bp.ExpYear = sql.NullInt64{2014, true}

				err := bp.Update(db)
				Expect(err).To(BeNil())
			})

			It("updates the record in database, if the Stripe call was successful", func() {
				fakeStripe.Responses = []string{test.StripeCustomerResponse}

				err := bp.UpdateCard(db, "token")
				Expect(err).To(BeNil())

				Expect(bp.StripeID.String).To(Equal("sc_01"))
				Expect(bp.StripeFingerprint.String).To(Equal("H0SJieNrXmTdq3qs"))
				Expect(bp.LastFour.String).To(Equal("4242"))
				Expect(bp.CardType.String).To(Equal("Visa"))
				Expect(bp.ExpMonth.Int64).To(Equal(int64(10)))
				Expect(bp.ExpYear.Int64).To(Equal(int64(2018)))
				Expect(bp.PendingActivation).To(BeFalse())
			})

			It("removes the time restrictions on a user", func() {
				Expect(u.MaxRunningTime).To(Equal(sql.NullInt64{7200, true}))
				Expect(u.RefreshPeriod).To(Equal(sql.NullInt64{86400, true}))

				fakeStripe.Responses = []string{test.StripeCustomerResponse}

				err := bp.UpdateCard(db, "token")
				Expect(err).To(BeNil())

				u2, err := user.FindByUsername(db, "qux")
				Expect(err).To(BeNil())
				Expect(u2.MaxRunningTime).To(Equal(sql.NullInt64{0, false}))
				Expect(u2.RefreshPeriod).To(Equal(sql.NullInt64{0, false}))
			})

			It("returns CardDeclined error, if Stripe returns a card declined error", func() {
				fakeStripe.Error = test.StripeCardDeclined
				err := bp.UpdateCard(db, "token")
				Expect(err).To(Equal(billingprofile.ErrCardDeclined))
			})

			It("returns ErrCardExpired error, if Stripe returns a card expired error", func() {
				fakeStripe.Error = test.StripeCardExpired
				err := bp.UpdateCard(db, "token")
				Expect(err).To(Equal(billingprofile.ErrCardExpired))
			})

			It("returns ErrIncorrectCvc error, if Stripe returns a card expired error", func() {
				fakeStripe.Error = test.StripeIncorrectCvc
				err := bp.UpdateCard(db, "token")
				Expect(err).To(Equal(billingprofile.ErrIncorrectCvc))
			})

			It("returns ErrIncorrectZipErr error, if Stripe returns a card expired error", func() {
				fakeStripe.Error = test.StripeIncorrectZip
				err := bp.UpdateCard(db, "token")
				Expect(err).To(Equal(billingprofile.ErrIncorrectZipErr))
			})

			It("does not update the record, when Stripe returns an error", func() {
				fakeStripe.Error = &stripe.Error{
					Type: stripe.ErrorType(stripe.APIErr),
					Msg:  "some error occurred",
					Code: stripe.ErrorCode(stripe.ProcessingErr),
				}

				err := bp.UpdateCard(db, "token")
				Expect(err).NotTo(BeNil())

				// should not change existing values
				Expect(bp.StripeID.String).To(Equal("sc_01"))
				Expect(bp.StripeFingerprint.String).To(Equal("PRINT"))
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
			fakeStripe *test.FakeStripe
		)

		BeforeEach(func() {
			u := &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())

			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)
		})

		Context("when called on a profile with an empty StripeID", func() {
			It("returns StripeProfileRequiredError", func() {
				err := bp.UpdateEmail(db, "new-email@example.com")
				Expect(err).To(Equal(billingprofile.StripeProfileRequiredError))
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
				s := fmt.Sprintf(`{
						"id": "sc0001",
						"email": "new-email@example.com"
					}`)
				fakeStripe.Responses = []string{s}

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

	Describe("IsCardRequired()", func() {
		It("should be false if the state is Exempt", func() {
			bp := &billingprofile.BillingProfile{
				State: billingprofile.StateExempt,
			}
			Expect(bp.IsCardRequired()).To(BeFalse())
		})

		It("should be true if the state is NoCard", func() {
			bp := &billingprofile.BillingProfile{
				State: billingprofile.StateNoCard,
			}
			Expect(bp.IsCardRequired()).To(BeTrue())
		})

		It("should be false otherwise", func() {
			bp := &billingprofile.BillingProfile{
				State: billingprofile.StatePaid,
			}
			Expect(bp.IsCardRequired()).To(BeFalse())
		})
	})

	Describe("IsFreeOrCardRequired()", func() {
		It("should be true if the state is Free", func() {
			bp := &billingprofile.BillingProfile{
				State: billingprofile.StateFree,
			}
			Expect(bp.IsFreeOrCardRequired()).To(BeTrue())
		})

		It("should be true if the state is NoCard", func() {
			bp := &billingprofile.BillingProfile{
				State: billingprofile.StateNoCard,
			}
			Expect(bp.IsFreeOrCardRequired()).To(BeTrue())
		})

		It("should be false otherwise", func() {
			bp := &billingprofile.BillingProfile{
				State: billingprofile.StatePaid,
			}
			Expect(bp.IsFreeOrCardRequired()).To(BeFalse())
		})
	})
})
