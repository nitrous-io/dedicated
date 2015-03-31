package invoice_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
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
		It("should fail if the record does not exist", func() {
			inv, err := invoice.ForPeriod(db, 112, time.Now(), time.Now().Add(2*time.Hour))
			Expect(inv).To(BeNil())
			Expect(err).To(MatchError(sql.ErrNoRows))
		})

		It("should return the invoice for the period", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			attempt := start.Add(1 * time.Minute)
			lAttempt := attempt.Add(2 * time.Minute)
			i := &invoice.Invoice{
				UserID:         u.ID,
				TotalCharge:    900,
				CreditAmount:   120,
				PeriodStart:    &start,
				PeriodEnd:      &end,
				State:          invoice.StateUnpaid,
				FirstAttemptAt: &attempt,
				LastAttemptAt:  &lAttempt,
				Attempts:       23,
				StripeID:       sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())

			inv, err := invoice.ForPeriod(db, u.ID, start, end)
			Expect(inv).To(Equal(i))
			Expect(err).To(BeNil())
		})
	})

	Describe(".FindByStripeID()", func() {
		It("should fail if the record does not exist", func() {
			inv, err := invoice.FindByStripeID(db, "Ok23x")
			Expect(inv).To(BeNil())
			Expect(err).To(MatchError(sql.ErrNoRows))
		})

		It("should return the invoice for the stripe ID", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			i := &invoice.Invoice{
				UserID:      u.ID,
				TotalCharge: 900,
				State:       invoice.StatePaid,
				Attempts:    23,
				StripeID:    sql.NullString{"Xcccs", true},
				PeriodStart: &start,
				PeriodEnd:   &end,
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
				UserID:         u.ID,
				TotalCharge:    900,
				PeriodStart:    &start,
				PeriodEnd:      &end,
				State:          invoice.StatePaid,
				FirstAttemptAt: &attempt,
				LastAttemptAt:  &lAttempt,
				Attempts:       23,
				StripeID:       sql.NullString{"Xcccs", true},
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
				UserID:         u.ID,
				TotalCharge:    -1,
				PeriodStart:    &start,
				PeriodEnd:      &end,
				State:          invoice.StatePaid,
				FirstAttemptAt: &attempt,
				LastAttemptAt:  &lAttempt,
				Attempts:       23,
				StripeID:       sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoice.ErrNegativeTotalCharge))
		})

		It("should not allow an empty state", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			attempt := start.Add(1 * time.Minute)
			lAttempt := attempt.Add(2 * time.Minute)
			i := &invoice.Invoice{
				UserID:         u.ID,
				TotalCharge:    -1,
				PeriodStart:    &start,
				PeriodEnd:      &end,
				FirstAttemptAt: &attempt,
				LastAttemptAt:  &lAttempt,
				Attempts:       23,
				StripeID:       sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoice.ErrEmptyState))
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
				UserID:         u.ID,
				TotalCharge:    900,
				PeriodStart:    &start,
				PeriodEnd:      &end,
				State:          invoice.StateUnpaid,
				FirstAttemptAt: &attempt,
				LastAttemptAt:  &lAttempt,
				Attempts:       23,
				StripeID:       sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the invoice in the database", func() {
			i2 := &invoice.Invoice{}
			err := db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())

			Expect(i2.State).To(Equal(invoice.StateUnpaid))
			Expect(i2.TotalCharge).To(Equal(int64(900)))
			Expect(i2.StripeID.String).To(Equal("Xcccs"))
			i2.State = invoice.StateFailed
			i2.TotalCharge = 93
			i2.StripeID.String = "Ok"

			err = i2.Update(db)
			Expect(err).To(BeNil())

			i3 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i3)
			Expect(err).To(BeNil())

			Expect(i3.State).To(Equal(invoice.StateFailed))
			Expect(i3.TotalCharge).To(Equal(int64(93)))
			Expect(i3.StripeID.String).To(Equal("Ok"))
		})

		It("should not allow a negative total charge", func() {
			i.TotalCharge = -93

			err := i.Update(db)
			Expect(err).To(Equal(invoice.ErrNegativeTotalCharge))
		})

		It("should not allow an empty state", func() {
			i.State = ""

			err := i.Update(db)
			Expect(err).To(Equal(invoice.ErrEmptyState))
		})
	})

	Describe("PaymentSucceeded", func() {
		var i *invoice.Invoice
		BeforeEach(func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			i = &invoice.Invoice{
				UserID:      u.ID,
				TotalCharge: 900,
				State:       invoice.StateUnpaid,
				Attempts:    23,
				StripeID:    sql.NullString{"Xcccs", true},
				PeriodStart: &start,
				PeriodEnd:   &end,
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should set State to Paid", func() {
			err := i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			Expect(i.State).To(Equal(invoice.StatePaid))
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
			Expect(i.State).To(Equal(invoice.StatePaid))
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(t.Unix()))
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt).ToNot(Equal(i2.LastAttemptAt))
		})

		It("should clear other states", func() {
			i.State = invoice.StateFailed
			err := i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			Expect(i.State).To(Equal(invoice.StatePaid))
			Expect(i.FirstAttemptAt).ToNot(BeNil())
			Expect(i.LastAttemptAt).ToNot(BeNil())
			Expect(i.FirstAttemptAt.Unix()).To(Equal(i.LastAttemptAt.Unix()))
		})

		It("should update others covered by this invoice", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			other := &invoice.Invoice{
				UserID:      u.ID,
				TotalCharge: 900,
				State:       invoice.StateDeferred,
				Attempts:    23,
				CoveredBy:   sql.NullInt64{i.ID, true},
				PeriodStart: &start,
				PeriodEnd:   &end,
			}
			err := other.Insert(db)
			Expect(err).To(BeNil())

			err = i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			other2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", other.ID).StructScan(other2)
			Expect(err).To(BeNil())
			Expect(other2.State).To(Equal(invoice.StatePaid))
		})

		It("should mark the billingprofile as state Paid if no other unpaid invoices remain", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StatePaymentDue, u.ID)
			Expect(err).To(BeNil())

			err = i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			b := &billingprofile.BillingProfile{}
			err = db.QueryRowx("SELECT * FROM billing_profiles WHERE user_id = $1", u.ID).StructScan(b)
			Expect(err).To(BeNil())
			Expect(b.State).To(Equal(billingprofile.StatePaid))
		})

		It("should not mark the billingprofile as state Paid if there are other unpaid invoices remaining", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			other := &invoice.Invoice{
				UserID:      u.ID,
				TotalCharge: 900,
				State:       invoice.StateUnpaid,
				Attempts:    23,
				PeriodStart: &start,
				PeriodEnd:   &end,
			}
			err := other.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StatePaymentDue, u.ID)
			Expect(err).To(BeNil())

			err = i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			b := &billingprofile.BillingProfile{}
			err = db.QueryRowx("SELECT * FROM billing_profiles WHERE user_id = $1", u.ID).StructScan(b)
			Expect(err).To(BeNil())
			Expect(b.State).To(Equal(billingprofile.StatePaymentDue))
		})

		It("should mark the billingprofile as state Paid if other unpaid invoices remain but have a total_charge of 0", func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			other := &invoice.Invoice{
				UserID:      u.ID,
				TotalCharge: 0,
				State:       invoice.StateNotPayable,
				Attempts:    23,
				PeriodStart: &start,
				PeriodEnd:   &end,
			}
			err := other.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StatePaymentDue, u.ID)
			Expect(err).To(BeNil())

			err = i.PaymentSucceeded(db)
			Expect(err).To(BeNil())

			b := &billingprofile.BillingProfile{}
			err = db.QueryRowx("SELECT * FROM billing_profiles WHERE user_id = $1", u.ID).StructScan(b)
			Expect(err).To(BeNil())
			Expect(b.State).To(Equal(billingprofile.StatePaid))
		})
	})

	Describe("PaymentFailed", func() {
		var i *invoice.Invoice
		BeforeEach(func() {
			start := time.Now()
			end := start.Add(2 * time.Hour)
			i = &invoice.Invoice{
				UserID:      u.ID,
				TotalCharge: 900,
				State:       invoice.StateUnpaid,
				Attempts:    23,
				StripeID:    sql.NullString{"Xcccs", true},
				PeriodStart: &start,
				PeriodEnd:   &end,
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should pass through the attemptCount", func() {
			appconfig.WebHostname = "nitrous.example"
			origMailer := invoice.Mailer
			fakeMailer := &FakeMailer{}
			invoice.Mailer = fakeMailer
			defer func() { invoice.Mailer = origMailer }()

			_, err := db.Exec("UPDATE billing_profiles SET last_four = '1901' WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())

			err = i.PaymentFailed(db, 2, false)
			Expect(err).To(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i2.State).To(Equal(invoice.StateUnpaid))
			Expect(i2.FirstAttemptAt).ToNot(BeNil())
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(i2.LastAttemptAt.Unix()))
			Expect(i2.Attempts).To(Equal(2))

			b := &billingprofile.BillingProfile{}
			err = db.QueryRowx("SELECT * FROM billing_profiles WHERE user_id = $1", u.ID).StructScan(b)
			Expect(err).To(BeNil())
			Expect(b.State).To(Equal(billingprofile.StatePaymentDue))

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"Steve" <foo@example.com>`}))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Subject).To(Equal("Nitrous payment unsuccessful"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello Steve,"))
			Expect(fakeMailer.Body).To(ContainSubstring("Unfortunately, we have been unable to successfully bill your provided credit card ending in 1901 to the value of $9.00."))
			Expect(fakeMailer.Body).To(ContainSubstring("We will retry billing your card very shortly, but in the meantime please verify your credit card at https://nitrous.example/app/#/billing."))
			Expect(fakeMailer.Body).To(ContainSubstring("If we are unable to bill your credit card your workspace will be **terminated soon**."))
			Expect(fakeMailer.Body).To(ContainSubstring("If you have any questions please email our support team at support@nitrous.io."))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello Steve,"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Unfortunately, we have been unable to successfully bill your provided credit card ending in 1901 to the value of $9.00."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`We will retry billing your card very shortly, but in the meantime please <a href="https://nitrous.example/app/#/billing">verify your credit card</a>.`))
			Expect(fakeMailer.HTML).To(ContainSubstring("If we are unable to bill your credit card your workspaces will be <strong>terminated soon</strong>."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`If you have any questions please email our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>.`))
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

		It("should set Failed, mark the billing profile as payment due and fire an email if closed out", func() {
			appconfig.WebHostname = "nitrous.example"
			origMailer := invoice.Mailer
			fakeMailer := &FakeMailer{}
			invoice.Mailer = fakeMailer
			defer func() { invoice.Mailer = origMailer }()

			_, err := db.Exec("UPDATE billing_profiles SET last_four = '1901' WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())

			t := time.Unix(20, 0)
			i.FirstAttemptAt = &t
			err = i.PaymentFailed(db, 2, true)
			Expect(err).To(BeNil())

			i2 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i2.FirstAttemptAt.Unix()).To(Equal(t.Unix()))
			Expect(i2.LastAttemptAt).ToNot(BeNil())
			Expect(i2.FirstAttemptAt).ToNot(Equal(i2.LastAttemptAt))
			Expect(i2.State).To(Equal(invoice.StateFailed))

			b := &billingprofile.BillingProfile{}
			err = db.QueryRowx("SELECT * FROM billing_profiles WHERE user_id = $1", u.ID).StructScan(b)
			Expect(err).To(BeNil())
			Expect(b.State).To(Equal(billingprofile.StateDelinquent))

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"Steve" <foo@example.com>`}))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Subject).To(Equal("Final Notice: Nitrous payment unsuccessful"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello Steve,"))
			Expect(fakeMailer.Body).To(ContainSubstring("Unfortunately, we have been unable to successfully bill your provided credit card ending in 1901 to the value of $9.00."))
			Expect(fakeMailer.Body).To(ContainSubstring("Your workspaces **will be terminated** very soon, please verify your credit card at https://nitrous.example/app/#/billing to restore service."))
			Expect(fakeMailer.Body).To(ContainSubstring("If you have any questions please email our support team at support@nitrous.io."))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello Steve,"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Unfortunately, we have been unable to successfully bill your provided credit card ending in 1901 to the value of $9.00."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`Your workspaces <strong>will be terminated</strong> very soon, please <a href="https://nitrous.example/app/#/billing">verify your credit card</a> to restore service.`))
			Expect(fakeMailer.HTML).To(ContainSubstring(`If you have any questions please email our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>.`))
		})
	})

})
