package invoice_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/invoice")
}

var _ = Describe("models/invoice", func() {
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
			Name:     "Steve",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe(".ForPeriod()", func() {
		It("should return nothing if the record does not exist", func() {
			inv, err := invoice.ForPeriod(db, 112, time.Now(), time.Now().Add(2*time.Hour))
			Expect(inv).To(BeNil())
			Expect(err).To(BeNil())
		})

		It("should return the invoice for the period", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			attempt := start.Add(1 * time.Minute)
			lAttempt := attempt.Add(2 * time.Minute)
			i := &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				CreditAmount:     120,
				PeriodStart:      &start,
				PeriodEnd:        &end,
				SuccessfulCharge: true,
				FirstAttemptAt:   &attempt,
				LastAttemptAt:    &lAttempt,
				Attempts:         23,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())

			inv, err := invoice.ForPeriod(db, u.ID, start, end)
			Expect(inv).To(Equal(i))
			Expect(err).To(BeNil())
		})
	})

	Describe(".FindByStripeID()", func() {
		It("should return nothing if the record does not exist", func() {
			inv, err := invoice.FindByStripeID(db, "Ok23x")
			Expect(inv).To(BeNil())
			Expect(err).To(BeNil())
		})

		It("should return the invoice for the stripe ID", func() {
			i := &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				SuccessfulCharge: true,
				Attempts:         23,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())

			inv, err := invoice.FindByStripeID(db, "Xcccs")
			Expect(inv).To(Equal(i))
			Expect(err).To(BeNil())
		})
	})

	Describe("Insert()", func() {
		It("persists the invoice in the database", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			attempt := start.Add(1 * time.Minute)
			lAttempt := attempt.Add(2 * time.Minute)
			i := &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				PeriodStart:      &start,
				PeriodEnd:        &end,
				SuccessfulCharge: true,
				FirstAttemptAt:   &attempt,
				LastAttemptAt:    &lAttempt,
				Attempts:         23,
				Deferred:         true,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())

			// Timestamps are set.
			Expect(i.CreatedAt).NotTo(BeNil())
			Expect(i.UpdatedAt).NotTo(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))
		})

		It("should not allow a negative total charge", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			attempt := start.Add(1 * time.Minute)
			lAttempt := attempt.Add(2 * time.Minute)
			i := &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      -1,
				PeriodStart:      &start,
				PeriodEnd:        &end,
				SuccessfulCharge: true,
				FirstAttemptAt:   &attempt,
				LastAttemptAt:    &lAttempt,
				Attempts:         23,
				Deferred:         true,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoice.ErrNegativeTotalCharge))
		})
	})

	Describe("Update()", func() {
		var i *invoice.Invoice
		BeforeEach(func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			attempt := start.Add(1 * time.Minute)
			lAttempt := attempt.Add(2 * time.Minute)
			i = &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				PeriodStart:      &start,
				PeriodEnd:        &end,
				SuccessfulCharge: false,
				FirstAttemptAt:   &attempt,
				LastAttemptAt:    &lAttempt,
				Attempts:         23,
				Deferred:         false,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the invoice in the database", func() {
			i2 := &invoice.Invoice{}
			err := db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())

			Expect(i2.Deferred).To(BeFalse())
			Expect(i2.SuccessfulCharge).To(BeFalse())
			Expect(i2.TotalCharge).To(Equal(int64(900)))
			Expect(i2.StripeID.String).To(Equal("Xcccs"))
			i2.SuccessfulCharge = true
			i2.TotalCharge = 93
			i2.StripeID.String = "Ok"
			i2.Deferred = true

			err = i2.Update(db)
			Expect(err).To(BeNil())

			i3 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i3)
			Expect(err).To(BeNil())

			Expect(i3.SuccessfulCharge).To(BeTrue())
			Expect(i3.TotalCharge).To(Equal(int64(93)))
			Expect(i3.StripeID.String).To(Equal("Ok"))
		})

		It("should not allow a negative total charge", func() {
			i.TotalCharge = -93

			err := i.Update(db)
			Expect(err).To(Equal(invoice.ErrNegativeTotalCharge))
		})
	})

	Describe("PaymentSucceeded", func() {
		var i *invoice.Invoice
		BeforeEach(func() {
			i = &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				SuccessfulCharge: false,
				Attempts:         23,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should set SuccessfulCharge as true", func() {
			err := i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			Expect(i.SuccessfulCharge).To(BeTrue())
			Expect(i.FirstAttemptAt).ToNot(BeNil())
			Expect(i.LastAttemptAt).ToNot(BeNil())
			Expect(i.FirstAttemptAt.Unix()).To(Equal(i.LastAttemptAt.Unix()))
		})

		It("should not set FirstAttemptAt if it is not nil", func() {
			t := time.Unix(20, 0)
			i.FirstAttemptAt = &t
			err := i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i2.SuccessfulCharge).To(BeTrue())
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(t.Unix()))
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt).ToNot(Equal(i2.LastAttemptAt))
		})

		It("should clear Failed state", func() {
			i.Failed = true
			err := i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			Expect(i.SuccessfulCharge).To(BeTrue())
			Expect(i.Failed).To(BeFalse())
			Expect(i.FirstAttemptAt).ToNot(BeNil())
			Expect(i.LastAttemptAt).ToNot(BeNil())
			Expect(i.FirstAttemptAt.Unix()).To(Equal(i.LastAttemptAt.Unix()))
		})

		It("should update others covered by this invoice", func() {
			other := &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				SuccessfulCharge: false,
				Attempts:         23,
				CoveredBy:        sql.NullInt64{i.ID, true},
			}
			err := other.Insert(db)
			Expect(err).To(BeNil())

			err = i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			other2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", other.ID).StructScan(other2)
			Expect(err).To(BeNil())
			Expect(other2.SuccessfulCharge).To(BeTrue())
		})
	})

	Describe("PaymentFailed", func() {
		var i *invoice.Invoice
		BeforeEach(func() {
			i = &invoice.Invoice{
				UserID:           u.ID,
				TotalCharge:      900,
				SuccessfulCharge: false,
				Attempts:         23,
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should pass through the attemptCount", func() {
			err := i.PaymentFailed(db, 2, true)
			Expect(err).To(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i2.SuccessfulCharge).To(BeFalse())
			Expect(i2.FirstAttemptAt).ToNot(BeNil())
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(i2.LastAttemptAt.Unix()))
			Expect(i2.SuccessfulCharge).To(BeFalse())
			Expect(i2.Attempts).To(Equal(2))
			Expect(i2.Failed).To(BeFalse())
		})

		It("should not set FirstAttemptAt if it is not nil", func() {
			t := time.Unix(20, 0)
			i.FirstAttemptAt = &t
			err := i.PaymentFailed(db, 2, false)
			Expect(err).To(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(t.Unix()))
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt).ToNot(Equal(i2.LastAttemptAt))
		})

		It("should set Failed and fire an email if closed out", func() {
			origMailer := invoice.Mailer
			fakeMailer := &FakeMailer{}
			invoice.Mailer = fakeMailer
			defer func() { invoice.Mailer = origMailer }()

			_, err := db.Exec("UPDATE billing_profiles SET last_four = '1901' WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())

			t := time.Unix(20, 0)
			i.FirstAttemptAt = &t
			err = i.PaymentFailed(db, 2, false)
			Expect(err).To(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(t.Unix()))
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt).ToNot(Equal(i2.LastAttemptAt))
			Expect(i2.Failed).To(BeTrue())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"Steve" <foo@example.com>`}))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Subject).To(Equal("Nitrous - payment unsuccessful"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello Steve,"))
			Expect(fakeMailer.Body).To(ContainSubstring("Unfortunately, we have been unable to successfully bill your provided credit card ending in 1901 to the value of $9.00."))
			Expect(fakeMailer.Body).To(ContainSubstring("To rectify this issue please contact our support team at support@nitrous.io."))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello Steve,"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Unfortunately, we have been unable to successfully bill your provided credit card ending in 1901 to the value of $9.00."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`To rectify this issue please contact our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>.`))
		})
	})

})
