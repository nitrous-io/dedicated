package stripememo_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"
	"github.com/nitrous-io/dedicated/nitrous/models/user"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/stripememo")
}

var _ = Describe("models/stripememo", func() {
	var (
		db         *db.Database
		fakeStripe *test.FakeStripe
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		fakeStripe = &test.FakeStripe{}
		stripe.SetBackend(fakeStripe)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Context("Invoices()", func() {
		var (
			bp *billingprofile.BillingProfile
		)

		BeforeEach(func() {
			u := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
		})

		It("does not query Stripe if there are recent invoices", func() {
			_, err := db.Exec(`INSERT INTO stripe_invoices
			(billing_profile_id, invoice_json, invoiced_at) VALUES
			($1, $2, now())
			`, bp.ID, test.StripeInvoiceResponse)
			Expect(err).To(BeNil())
			invs, err := stripememo.Invoices(db, bp)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
			Expect(invs).To(HaveLen(1))
			Expect(invs[0].UserID).To(Equal(bp.UserID))
			si, err := invs[0].StripeInvoice()
			Expect(err).To(BeNil())
			Expect(si).NotTo(BeNil())
		})

		It("does query Stripe if there are invoices but from more than 24 hours ago", func() {
			fakeStripe.Responses = []string{test.StripeInvoiceListResponse}
			_, err := db.Exec(`INSERT INTO stripe_invoices
			(billing_profile_id, invoice_json, created_at) VALUES
			($1, $2, now() - interval '25 hours')
			`, bp.ID, test.StripeInvoiceResponse)
			invs, err := stripememo.Invoices(db, bp)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(invs).To(HaveLen(1))
			Expect(invs[0].UserID).To(Equal(bp.UserID))
		})

		It("writes Stripe responses to db when querying Stripe", func() {
			fakeStripe.Responses = []string{test.StripeInvoiceListResponse}
			_, err := stripememo.Invoices(db, bp)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

			var writtenInvs []*stripememo.Invoice
			err = db.Select(&writtenInvs, `SELECT * FROM stripe_invoices`)
			Expect(err).To(BeNil())
			Expect(writtenInvs).To(HaveLen(1))
			Expect(writtenInvs[0].BillingProfileID).To(Equal(bp.ID))
			Expect(writtenInvs[0].InvoicedAt.Equal(time.Unix(1448338447, 0))).To(BeTrue())

			var fromJSON stripe.Invoice
			err = json.Unmarshal([]byte(writtenInvs[0].InvoiceJSON), &fromJSON)
			Expect(err).To(BeNil())
			Expect(fromJSON.Amount).To(Equal(int64(3000)))
		})

		It("removes previous entries from db when querying Stripe", func() {
			fakeStripe.Responses = []string{test.StripeInvoiceListResponse}
			_, err := db.Exec(`INSERT INTO stripe_invoices
			(billing_profile_id, invoice_json, created_at) VALUES
			($1, $2, now() - interval '25 hours')
			`, bp.ID, test.StripeInvoiceResponse)
			_, err = stripememo.Invoices(db, bp)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

			var writtenInvs []*stripememo.Invoice
			err = db.Select(&writtenInvs, `SELECT * FROM stripe_invoices`)
			Expect(err).To(BeNil())
			Expect(writtenInvs).To(HaveLen(1))
			Expect(writtenInvs[0].CreatedAt).To(BeTemporally("~", time.Now(), 5*time.Second))
		})
	})

	Context("fetchedInvoices()", func() {
		It("returns slice of invoices with both InvoiceJSON and StripeInvoice() populated", func() {
			fakeStripe.Responses = []string{test.StripeInvoiceListResponse}
			invs, err := stripememo.FetchedInvoices(&billingprofile.BillingProfile{
				StripeID: sql.NullString{"abc", true},
			})
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(err).To(BeNil())
			Expect(invs).To(HaveLen(1))

			inv := invs[0]
			si, err := inv.StripeInvoice()
			Expect(err).To(BeNil())
			Expect(si.ID).To(Equal("in_17AYBv2eZvKYlo2C8sW8Cs4R"))
			Expect(si.Customer.ID).To(Equal("cus_7PN4EHPJGbC4D1"))

			var fromJSON stripe.Invoice
			err = json.Unmarshal([]byte(inv.InvoiceJSON), &fromJSON)
			Expect(err).To(BeNil())
			Expect(fromJSON.ID).To(Equal(si.ID))
			Expect(fromJSON.Customer.ID).To(Equal(si.Customer.ID))
		})
	})
})
