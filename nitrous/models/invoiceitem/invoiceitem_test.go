package invoiceitem_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
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
			State:       invoice.StateUnpaid,
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
				Hours:       sql.NullInt64{900, true},
				HourlyPrice: sql.NullInt64{1231, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"2 GB", true},
			}
		})

		It("should describe the host usage", func() {
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' compute (2 GB memory) - 900 hours at 1.231c/hour"))
		})

		It("should be able to display terminated state", func() {
			inv.Terminated = true
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' compute (2 GB memory) - 900 hours at 1.231c/hour (terminated)"))
		})

		It("should handle up to 3 decimal places precision", func() {
			inv.HourlyPrice = sql.NullInt64{1, true}
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' compute (2 GB memory) - 900 hours at 0.001c/hour"))
		})

		It("should trim to 2 decimal points if needed", func() {
			inv.HourlyPrice = sql.NullInt64{100, true}
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' compute (2 GB memory) - 900 hours at 0.10c/hour"))
		})

		It("should be a credit description when the item is a Credit", func() {
			inv.ItemType = invoiceitem.ItemTypeCredit
			inv.Credit = sql.NullInt64{900, true}
			Expect(invoiceitem.ItemDescription(inv, nil)).To(Equal("Credit of $9.00"))
		})

		It("should describe the base price", func() {
			inv.ItemType = invoiceitem.ItemTypeBaseEBS
			inv.PlanStorage = sql.NullString{"20 GB", true}
			inv.Hours = sql.NullInt64{invoiceitem.MaxHours, true}
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' base (20 GB storage) - 672 hours at 1.231c/hour"))
		})

		It("should include the pro rata in the base price if the hours are lower", func() {
			inv.ItemType = invoiceitem.ItemTypeBaseEBS
			inv.PlanStorage = sql.NullString{"20 GB", true}
			inv.Hours = sql.NullInt64{90, true}
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' base (20 GB storage) - 90 hours at 1.231c/hour"))
		})

		It("should the terminated state in the base price description", func() {
			inv.ItemType = invoiceitem.ItemTypeBaseEBS
			inv.PlanStorage = sql.NullString{"20 GB", true}
			inv.Hours = sql.NullInt64{90, true}
			inv.Terminated = true
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' base (20 GB storage) - 90 hours at 1.231c/hour (terminated)"))
		})

		It("should strip migrated- prefix from a host slug", func() {
			hst.Slug = "migrated-ABCDEF"
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' compute (2 GB memory) - 900 hours at 1.231c/hour"))
		})

		It("should strip migrated- prefix and change terminated state if migrated & terminated", func() {
			hst.Slug = "migrated-ABCDEF"
			inv.Terminated = true
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("'ABCDEF' compute (2 GB memory) - 900 hours at 1.231c/hour (previous)"))
		})

		It("should return the Description column if populated", func() {
			hst.Slug = "migrated-ABCDEF"
			inv.Terminated = true
			inv.Description = sql.NullString{"A description from Stripe", true}
			Expect(invoiceitem.ItemDescription(inv, hst)).To(Equal("A description from Stripe"))
		})
	})

	Describe("HourlyCents()", func() {
		It("should convert milli-cents to cents", func() {
			i := invoiceitem.InvoiceItem{
				HourlyPrice: sql.NullInt64{500, true},
			}
			Expect(i.HourlyCents()).To(Equal(0.5))
		})
	})

	Describe("TotalCharge()", func() {
		It("should be negative form of the Credit value", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType: invoiceitem.ItemTypeCredit,
				Credit:   sql.NullInt64{900, true},
			}
			Expect(inv.TotalCharge(nil)).To(Equal(int64(-900)))
		})
		It("should be the original cost if under the plan price for a Compute type", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType:    invoiceitem.ItemTypeCompute,
				Hours:       sql.NullInt64{2, true},
				HourlyPrice: sql.NullInt64{2000, true},
			}
			Expect(inv.TotalCharge(&plan.Plan{Price: 9})).To(Equal(int64(4)))
		})
		It("should be the capped to the cost of the plan for a Compute type", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType:    invoiceitem.ItemTypeCompute,
				Hours:       sql.NullInt64{20, true},
				HourlyPrice: sql.NullInt64{2000, true},
			}
			Expect(inv.TotalCharge(&plan.Plan{Price: 9})).To(Equal(int64(9)))
		})
		It("should be the original cost if under the base price plan price for a BaseEBS type", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType:    invoiceitem.ItemTypeBaseEBS,
				Hours:       sql.NullInt64{2, true},
				HourlyPrice: sql.NullInt64{2000, true},
			}
			Expect(inv.TotalCharge(&plan.Plan{BasePrice: 8})).To(Equal(int64(4)))
		})
		It("should be the capped to the cost of the base price plan for a BaseEBS type", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType:    invoiceitem.ItemTypeBaseEBS,
				Hours:       sql.NullInt64{20, true},
				HourlyPrice: sql.NullInt64{2000, true},
			}
			Expect(inv.TotalCharge(&plan.Plan{BasePrice: 8})).To(Equal(int64(8)))
		})
	})

	Describe("UncappedTotalCharge", func() {
		It("should multiply and convert to cents", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType:    invoiceitem.ItemTypeCompute,
				Hours:       sql.NullInt64{2, true},
				HourlyPrice: sql.NullInt64{2000, true},
			}
			Expect(inv.UncappedTotalCharge()).To(Equal(int64(4)))
		})

		It("should round up to the nearest cent", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType:    invoiceitem.ItemTypeCompute,
				Hours:       sql.NullInt64{1, true},
				HourlyPrice: sql.NullInt64{500, true},
			}
			Expect(inv.UncappedTotalCharge()).To(Equal(int64(1)))
		})

		It("should be negative form of the Credit value", func() {
			inv := &invoiceitem.InvoiceItem{
				ItemType: invoiceitem.ItemTypeCredit,
				Credit:   sql.NullInt64{900, true},
			}
			Expect(inv.UncappedTotalCharge()).To(Equal(int64(-900)))
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
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			err := i1.Insert(db)
			Expect(err).To(BeNil())

			i2 := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"4 GB", true},
			}
			err = i2.Insert(db)
			Expect(err).To(BeNil())

			in2 := &invoice.Invoice{
				UserID:      u.ID,
				PeriodStart: in.PeriodStart,
				PeriodEnd:   in.PeriodEnd,
				State:       invoice.StateUnpaid,
			}
			err = in2.Insert(db)
			Expect(err).To(BeNil())

			n1 := &invoiceitem.InvoiceItem{
				InvoiceID:   in2.ID,
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"3 GB", true},
			}
			err = n1.Insert(db)
			Expect(err).To(BeNil())

			res, err := invoiceitem.FindAllForInvoice(db, in.ID)
			Expect(err).To(BeNil())
			Expect(res).To(Equal([]invoiceitem.InvoiceItem{*i1, *i2}))
		})
	})

	Describe("Insert()", func() {
		It("persists the invoice item in the database", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"1 GB", true},
				HostID:      sql.NullInt64{h.ID, true},
				Terminated:  true,
				Hours:       sql.NullInt64{24, true},
				Credit:      sql.NullInt64{20, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
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

		It("should expect a ItemType is provided", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID: in.ID,
				ItemType:  "",
				StripeID:  sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrInvalidItemType))
		})

		It("should expect a Credit be greater than 0 if the type is a credit", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeCredit,
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				Credit:      sql.NullInt64{-20, true},
				StripeID:    sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrCreditInvalid))
		})

		It("should require PlanMemory to be valid if the type is a Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				StripeID:    sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrPlanMemoryMissing))
		})

		It("should require PlanMemory to be valid if the type is a free Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeFreeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				StripeID:    sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrPlanMemoryMissing))
		})

		It("should require Hours to be valid if the type is a Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				StripeID:    sql.NullString{"Xcccs", true},
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrHoursInvalid))
		})

		It("should require Hours to be valid if the type is a Free Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeFreeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				StripeID:    sql.NullString{"Xcccs", true},
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrHoursInvalid))
		})

		It("should require HourlyPrice to be valid if the type is a Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:  in.ID,
				ItemType:   invoiceitem.ItemTypeCompute,
				HostID:     sql.NullInt64{h.ID, true},
				Hours:      sql.NullInt64{24, true},
				StripeID:   sql.NullString{"Xcccs", true},
				PlanMemory: sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrHourlyPriceInvalid))
		})

		It("should not require HourlyPrice to be valid if the type is a Free Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:  in.ID,
				ItemType:   invoiceitem.ItemTypeFreeCompute,
				HostID:     sql.NullInt64{h.ID, true},
				Hours:      sql.NullInt64{24, true},
				StripeID:   sql.NullString{"Xcccs", true},
				PlanMemory: sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should expect Hours to be greater than zero", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{-1, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				StripeID:    sql.NullString{"Xcccs", true},
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrHoursNotPositive))
		})

		It("should expect HourlyPrice to be not negative if the type is a Compute", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{-1, true},
				StripeID:    sql.NullString{"Xcccs", true},
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrHourlyPriceNegative))
		})

		It("should require PlanStorage to be valid if the type is a Base EBS", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID: in.ID,
				ItemType:  invoiceitem.ItemTypeBaseEBS,
				HostID:    sql.NullInt64{h.ID, true},
				StripeID:  sql.NullString{"Xcccs", true},
			}
			err := i.Insert(db)
			Expect(err).To(Equal(invoiceitem.ErrPlanStorageMissing))
		})

		It("should insert a Base EBS if it has a valid storage value", func() {
			i := &invoiceitem.InvoiceItem{
				InvoiceID:   in.ID,
				ItemType:    invoiceitem.ItemTypeBaseEBS,
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{2, true},
				StripeID:    sql.NullString{"Xcccs", true},
				PlanStorage: sql.NullString{"200 DB", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())

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
				ItemType:    invoiceitem.ItemTypeCompute,
				HostID:      sql.NullInt64{h.ID, true},
				Hours:       sql.NullInt64{24, true},
				Terminated:  true,
				Credit:      sql.NullInt64{20, true},
				HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
				StripeID:    sql.NullString{"Xcccs", true},
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the item in the database", func() {
			i2 := &invoiceitem.InvoiceItem{}
			err := db.QueryRowx("SELECT * FROM invoice_items WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())

			Expect(i2.Terminated).To(BeTrue())
			Expect(i2.Hours.Int64).To(Equal(int64(24)))
			Expect(i2.HourlyPrice.Int64).To(Equal(h.HourlyPrice))
			Expect(i2.StripeID.String).To(Equal("Xcccs"))
			i2.Hours = sql.NullInt64{95, true}
			i2.HourlyPrice = sql.NullInt64{5000, true}
			i2.Credit = sql.NullInt64{10, true}
			i2.StripeID.String = "Ok"
			i2.Terminated = false

			err = i2.Update(db)
			Expect(err).To(BeNil())

			i3 := &invoiceitem.InvoiceItem{}
			err = db.QueryRowx("SELECT * FROM invoice_items WHERE id = $1", i.ID).StructScan(i3)
			Expect(err).To(BeNil())

			Expect(i3.Hours.Int64).To(Equal(int64(95)))
			Expect(i3.HourlyPrice.Int64).To(Equal(int64(5000)))
			Expect(i3.Credit.Int64).To(Equal(int64(10)))
			Expect(i3.StripeID.String).To(Equal("Ok"))
			Expect(i3.Terminated).To(BeFalse())
		})

		It("should expect a Type is provided", func() {
			i.ItemType = ""
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrInvalidItemType))
		})

		It("should expect a Credit be greater than 0 if the type is a Credit", func() {
			i.ItemType = invoiceitem.ItemTypeCredit
			i.HostID = sql.NullInt64{}
			i.Credit = sql.NullInt64{-8, true}
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrCreditInvalid))
		})

		It("should require PlanMemory to be valid if the type is a Compute", func() {
			i.PlanMemory = sql.NullString{"", false}
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrPlanMemoryMissing))
		})

		It("should expect Hours to be valid if type is a Compute", func() {
			i.Hours = sql.NullInt64{}
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrHoursInvalid))
		})

		It("should require HourlyPrice to be valid if type is a Compute", func() {
			i.HourlyPrice = sql.NullInt64{}
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrHourlyPriceInvalid))
		})

		It("should expect Hours to be greater than zero if type is a Compute", func() {
			i.Hours = sql.NullInt64{-1, true}
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrHoursNotPositive))
		})

		It("should expect Hours to be not negative if type is a Compute", func() {
			i.HourlyPrice = sql.NullInt64{-1, true}
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrHourlyPriceNegative))
		})

		It("should require PlanStorage to be valid if the type is a Base EBS", func() {
			i.ItemType = invoiceitem.ItemTypeBaseEBS
			err := i.Update(db)
			Expect(err).To(Equal(invoiceitem.ErrPlanStorageMissing))
		})
	})

})
