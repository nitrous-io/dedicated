package invoicemanualattempt_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoicemanualattempt"
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

	Describe("CreateForInvoice()", func() {
		It("should store a successful attempt in the DB", func() {
			err := invoicemanualattempt.CreateForInvoice(db, in.ID, true, "")
			Expect(err).To(BeNil())

			at1 := &invoicemanualattempt.InvoiceManualAttempt{}
			err = db.QueryRowx("SELECT * FROM invoice_manual_attempts WHERE invoice_id = $1", in.ID).StructScan(at1)
			Expect(err).To(BeNil())
			Expect(at1.Success).To(BeTrue())
			Expect(at1.ErrorMessage).To(Equal(sql.NullString{"", false}))
		})

		It("should store a failed attempt in the DB", func() {
			err := invoicemanualattempt.CreateForInvoice(db, in.ID, false, "Something bad")
			Expect(err).To(BeNil())

			at1 := &invoicemanualattempt.InvoiceManualAttempt{}
			err = db.QueryRowx("SELECT * FROM invoice_manual_attempts WHERE invoice_id = $1", in.ID).StructScan(at1)
			Expect(err).To(BeNil())
			Expect(at1.Success).To(BeFalse())
			Expect(at1.ErrorMessage).To(Equal(sql.NullString{"Something bad", true}))
		})
	})

})
