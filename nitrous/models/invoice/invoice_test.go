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
				StripeID:         sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the invoice in the database", func() {
			i2 := &invoice.Invoice{}
			err := db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())

			Expect(i2.SuccessfulCharge).To(BeFalse())
			Expect(i2.TotalCharge).To(Equal(int64(900)))
			Expect(i2.StripeID.String).To(Equal("Xcccs"))
			i2.SuccessfulCharge = true
			i2.TotalCharge = 93
			i2.StripeID.String = "Ok"

			err = i2.Update(db)
			Expect(err).To(BeNil())

			i3 := &invoice.Invoice{}
			err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.ID).StructScan(i3)
			Expect(err).To(BeNil())

			Expect(i3.SuccessfulCharge).To(BeTrue())
			Expect(i3.TotalCharge).To(Equal(int64(93)))
			Expect(i3.StripeID.String).To(Equal("Ok"))
		})
	})

})
