package stripememo_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"

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

	Context("memoInvoices()", func() {
		It("returns an Invoice with both InvoiceJSON and Invoice populated", func() {
			fakeStripe.Responses = []string{test.StripeInvoiceResponse}
			invs, err := stripememo.MemoInvoices(&billingprofile.BillingProfile{
				StripeID: sql.NullString{"abc", true},
			})
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(err).To(BeNil())
			Expect(invs).To(HaveLen(1))

			inv := invs[0]
			Expect(inv.Invoice.ID).To(Equal("in_17AYBv2eZvKYlo2C8sW8Cs4R"))
			Expect(inv.Invoice.Customer.ID).To(Equal("cus_7PN4EHPJGbC4D1"))

			var fromJSON stripe.Invoice
			err = json.Unmarshal([]byte(inv.InvoiceJSON), &fromJSON)
			Expect(err).To(BeNil())
			Expect(fromJSON.ID).To(Equal(inv.Invoice.ID))
			Expect(fromJSON.Customer.ID).To(Equal(inv.Invoice.Customer.ID))
		})
	})
})
