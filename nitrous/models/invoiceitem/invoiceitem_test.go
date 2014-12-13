package invoiceitem_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/invoiceitem")
}

var _ = Describe("models/invoiceitem", func() {
	var (
		db *db.Database
		u  *user.User
		h  *host.Host
		hs *HostStack
		in *invoice.Invoice
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

		hs, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug: "foo",
			},
			U: u,
		})
		Expect(err).To(BeNil())
		h = hs.H

		start := time.Now()
		end := start.Add(2 * time.Hour)
		in = &invoice.Invoice{
			UserID:      u.ID,
			PeriodStart: &start,
			PeriodEnd:   &end,
		}
		err = in.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("ItemDescription", func() {
		var (
			hst *host.Host
			inv *invoiceitem.InvoiceItem
		)
		BeforeEach(func() {
			hst = &host.Host{
				Slug: "ABCDEF",
			}
			inv = &invoiceitem.InvoiceItem{
				Hours:       900,
				HourlyPrice: 1231,
			}
		})

		It("should describe the host usage", func() {
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("Application 'ABCDEF' 900 hours at 1.23c/hour"))
		})

		It("should be able to display terminated state", func() {
			hst.State = host.StateTerminated
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("Application 'ABCDEF' (terminated) 900 hours at 1.23c/hour"))
		})
	})

	Describe(".FindAllForInvoice", func() {
		It("should return empty set if none was find", func() {
			res, err := invoiceitem.FindAllForInvoice(db, 902378)
			Expect(err).To(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should find all invoice items for the specified ID", func() {
			i1 := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				HostID:      h.ID,
				Hours:       24,
				HourlyPrice: h.HourlyPrice,
			}
			err := i1.Insert(db)
			Expect(err).To(BeNil())

			i2 := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				HostID:      h.ID,
				Hours:       24,
				HourlyPrice: h.HourlyPrice,
			}
			err = i2.Insert(db)
			Expect(err).To(BeNil())

			in2 := &invoice.Invoice{
				UserID:      u.ID,
				PeriodStart: in.PeriodStart,
				PeriodEnd:   in.PeriodEnd,
			}
			err = in2.Insert(db)
			Expect(err).To(BeNil())

			n1 := &invoiceitem.InvoiceItem{
				InvoiceID:   in2.ID,
				HostID:      h.ID,
				Hours:       24,
				HourlyPrice: h.HourlyPrice,
			}
			err = n1.Insert(db)
			Expect(err).To(BeNil())

			res, err := invoiceitem.FindAllForInvoice(db, in.ID)
			Expect(err).To(BeNil())
			Expect(res).To(Equal([]invoiceitem.InvoiceItem{*i1, *i2}))
		})
	})

	Describe("HourlyCents()", func() {
		It("should convert milli-cents to cents", func() {
			i := invoiceitem.InvoiceItem{
				HourlyPrice: 500,
			}
			Expect(i.HourlyCents()).To(Equal(0.5))
		})
	})

	Describe("Insert()", func() {
		It("persists the invoice item in the database", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				HostID:      h.ID,
				Hours:       24,
				HourlyPrice: h.HourlyPrice,
				StripeID:    sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())

			// Timestamps are set.
			Expect(i.CreatedAt).NotTo(BeNil())
			Expect(i.UpdatedAt).NotTo(BeNil())

			i2 := &invoiceitem.InvoiceItem{}
			err = db.QueryRowx("SELECT * FROM invoice_items WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))
		})
	})

	Describe("Update()", func() {
		var i *invoiceitem.InvoiceItem
		BeforeEach(func() {
			i = &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				HostID:      h.ID,
				Hours:       24,
				HourlyPrice: h.HourlyPrice,
				StripeID:    sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the item in the database", func() {
			i2 := &invoiceitem.InvoiceItem{}
			err := db.QueryRowx("SELECT * FROM invoice_items WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())

			Expect(i2.Hours).To(Equal(24))
			Expect(i2.HourlyPrice).To(Equal(h.HourlyPrice))
			Expect(i2.StripeID.String).To(Equal("Xcccs"))
			i2.Hours = 95
			i2.HourlyPrice = 5000
			i2.StripeID.String = "Ok"

			err = i2.Update(db)
			Expect(err).To(BeNil())

			i3 := &invoiceitem.InvoiceItem{}
			err = db.QueryRowx("SELECT * FROM invoice_items WHERE id = $1", i.ID).StructScan(i3)
			Expect(err).To(BeNil())

			Expect(i3.Hours).To(Equal(95))
			Expect(i3.HourlyPrice).To(Equal(int64(5000)))
			Expect(i3.StripeID.String).To(Equal("Ok"))
		})
	})

})
