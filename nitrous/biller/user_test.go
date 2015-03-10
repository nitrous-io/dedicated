package biller_test

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("biller/user", func() {

	var (
		endTime   time.Time
		startTime time.Time

		u  *user.User
		db *ndb.Database
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

		startTime = time.Date(2014, 1, 1, 0, 0, 0, 0, time.Local)
		endTime = time.Date(2014, 1, 31, 24, 59, 59, 1000000000, time.Local)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("InvoiceItemsForUser()", func() {
		var (
			hs1 *HostStack
			hs2 *HostStack
			hs3 *HostStack
			h1  *host.Host
			h2  *host.Host
			h3  *host.Host
		)

		BeforeEach(func() {
			var err error
			hs1, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h1 = hs1.H

			_, err = db.Exec("UPDATE hosts SET terminated_at = $1, updated_at = now() WHERE id = $2;", endTime, h1.ID)
			Expect(err).To(BeNil())

			hs2, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 500,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h2 = hs2.H

			hs3, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-3",
					HourlyPrice: 3722, // total cost for host will theoretically be $25.01 but plan price is 25.00
				},
				P: &plan.Plan{
					Slug:          "hs3Plan",
					Name:          "hs3Plan",
					MaxContainers: 1,
					Price:         2500,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h3 = hs3.H

			createHostEndingTransition(db, h1.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
			createHostTransition(db, h1.ID, "running", "terminated", startTime.Add(2*time.Hour))
			createHostEndingTransition(db, h2.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
			createHostEndingTransition(db, h2.ID, "running", "terminated", startTime.Add(2*time.Hour), startTime.Add(4*time.Hour))
			createHostTransition(db, h2.ID, "terminated", "running", startTime.Add(4*time.Hour))
			createHostTransition(db, h3.ID, "born", "running", startTime)
		})

		It("should return the invoiceitems for the user", func() {
			res, err := biller.InvoiceItemsForUser(db, nil, u.ID, startTime, endTime)
			items := res.Items
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(3))

			var i1, i2, i3 invoiceitem.InvoiceItem
			for _, i := range items {
				if i.HostID.Int64 == h1.ID {
					i1 = i
				} else if i.HostID.Int64 == h2.ID {
					i2 = i
				} else {
					i3 = i
				}
			}

			Expect(i1.ItemType).To(Equal(invoiceitem.ItemTypeCompute))
			Expect(i1.Terminated).To(BeTrue())
			Expect(i1.HourlyPrice).To(Equal(sql.NullInt64{2000, true}))
			Expect(i1.Hours).To(Equal(sql.NullInt64{2, true}))

			Expect(i2.ItemType).To(Equal(invoiceitem.ItemTypeCompute))
			Expect(i2.Terminated).To(BeFalse())
			Expect(i2.HourlyPrice).To(Equal(sql.NullInt64{500, true}))
			Expect(i2.Hours).To(Equal(sql.NullInt64{672, true}))

			Expect(i3.ItemType).To(Equal(invoiceitem.ItemTypeCompute))
			Expect(i3.Terminated).To(BeFalse())
			Expect(i3.HourlyPrice).To(Equal(sql.NullInt64{3722, true}))
			Expect(i3.Hours).To(Equal(sql.NullInt64{672, true}))
		})

		It("should not incur charges to the user for stopped instances", func() {
			createHostEndingTransition(db, h1.ID, "terminated", "stopped", startTime.Add(2*time.Hour), startTime.Add(8*time.Hour))
			createHostTransition(db, h1.ID, "stopped", "terminated", startTime.Add(8*time.Hour))

			res, err := biller.InvoiceItemsForUser(db, nil, u.ID, startTime, endTime)
			items := res.Items
			Expect(err).To(BeNil())

			var i1 invoiceitem.InvoiceItem
			for _, i := range items {
				if i.HostID.Int64 == h1.ID {
					i1 = i
					break
				}
			}

			Expect(i1.Terminated).To(BeTrue())
			Expect(i1.HourlyPrice).To(Equal(sql.NullInt64{2000, true}))
			Expect(i1.Hours).To(Equal(sql.NullInt64{2, true}))
		})

		It("should not include hosts not belonging to the user", func() {
			u2 := &user.User{
				Username: "fooey",
				Email:    "fooey@example.com",
				Password: "123456",
			}
			err := WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET user_id = $1 WHERE id = $2", u2.ID, h1.ID)
			Expect(err).To(BeNil())

			res, err := biller.InvoiceItemsForUser(db, nil, u.ID, startTime, endTime)
			Expect(err).To(BeNil())
			items := res.Items
			Expect(items).To(HaveLen(2))

			Expect([]int64{items[0].HostID.Int64, items[1].HostID.Int64}).To(ConsistOf([]int64{h2.ID, h3.ID}))
		})

		It("should provide the total charge", func() {
			res, err := biller.InvoiceItemsForUser(db, nil, u.ID, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(res.TotalCharge).To(Equal(int64(2500 + 336 + 4)))
		})
	})

	Describe("InvoicesItemsForHost()", func() {
		It("should return an error if the host does not exist", func() {
			ch, items, err := biller.InvoicesItemsForHost(db, nil, -1, startTime, endTime)
			Expect(err).ToNot(BeNil())
			Expect(ch).To(Equal(int64(0)))
			Expect(items).To(BeEmpty())
		})

		It("should return no charge and empty items if the host lacks transitions", func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			ch, items, err := biller.InvoicesItemsForHost(db, nil, hs1.H.ID, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(ch).To(Equal(int64(0)))
			Expect(items).To(BeEmpty())
		})

		It("should return no charge and a invoice item if the host was already terminated", func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET terminated_at = $1, updated_at = now() WHERE id = $2;", endTime, hs1.H.ID)
			Expect(err).To(BeNil())

			createHostTransition(db, hs1.H.ID, "running", "terminated", startTime)

			ch, items, err := biller.InvoicesItemsForHost(db, nil, hs1.H.ID, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(ch).To(Equal(int64(0)))
			Expect(items).To(HaveLen(1))
			Expect(items[0].HostID).To(Equal(sql.NullInt64{hs1.H.ID, true}))
			Expect(items[0].Hours).To(Equal(sql.NullInt64{0, true}))
			Expect(items[0].HourlyPrice).To(Equal(sql.NullInt64{2000, true}))
		})

		It("should collate all running and stopped usage", func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			createHostEndingTransition(db, hs1.H.ID, "born", "running", startTime, startTime.Add(7*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "running", "stopped", startTime.Add(7*time.Hour), startTime.Add(14*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "stopped", "running", startTime.Add(14*time.Hour), startTime.Add(20*time.Hour))
			createHostTransition(db, hs1.H.ID, "running", "terminated", startTime.Add(20*time.Hour))

			ch, items, err := biller.InvoicesItemsForHost(db, nil, hs1.H.ID, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(ch).To(Equal(int64(26)))
			Expect(items).To(HaveLen(1))
			Expect(items[0].HostID).To(Equal(sql.NullInt64{hs1.H.ID, true}))
			Expect(items[0].Hours).To(Equal(sql.NullInt64{13, true}))
			Expect(items[0].HourlyPrice).To(Equal(sql.NullInt64{2000, true}))
		})

		It("should cap the hours to 28 days", func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 200,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			createHostEndingTransition(db, hs1.H.ID, "born", "running", startTime, startTime.Add(7*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "running", "stopped", startTime.Add(7*time.Hour), startTime.Add(14*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "stopped", "running", startTime.Add(14*time.Hour), startTime.Add(20*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "running", "terminated", startTime.Add(20*time.Hour), startTime.Add(21*time.Hour))
			createHostTransition(db, hs1.H.ID, "terminated", "running", startTime.Add(21*time.Hour))

			ch, items, err := biller.InvoicesItemsForHost(db, nil, hs1.H.ID, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(ch).To(Equal(int64(135)))
			Expect(items).To(HaveLen(1))
			Expect(items[0].HostID).To(Equal(sql.NullInt64{hs1.H.ID, true}))
			Expect(items[0].Hours).To(Equal(sql.NullInt64{invoiceitem.MaxHours, true}))
			Expect(items[0].HourlyPrice).To(Equal(sql.NullInt64{200, true}))
		})

		It("should cap the total price to the plan price", func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 3700,
				},
				P: &plan.Plan{
					Price: 2200,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			createHostEndingTransition(db, hs1.H.ID, "born", "running", startTime, startTime.Add(7*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "running", "stopped", startTime.Add(7*time.Hour), startTime.Add(14*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "stopped", "running", startTime.Add(14*time.Hour), startTime.Add(20*time.Hour))
			createHostEndingTransition(db, hs1.H.ID, "running", "terminated", startTime.Add(20*time.Hour), startTime.Add(21*time.Hour))
			createHostTransition(db, hs1.H.ID, "terminated", "running", startTime.Add(21*time.Hour))

			ch, items, err := biller.InvoicesItemsForHost(db, nil, hs1.H.ID, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(ch).To(Equal(int64(2200)))
			Expect(items).To(HaveLen(1))
			Expect(items[0].HostID).To(Equal(sql.NullInt64{hs1.H.ID, true}))
			Expect(items[0].Hours).To(Equal(sql.NullInt64{invoiceitem.MaxHours, true}))
			Expect(items[0].HourlyPrice).To(Equal(sql.NullInt64{3700, true}))
		})
	})
})

func createHostTransition(d db.QueryReceiver, hostID int64, previousState, state string, createdAt time.Time) {
	_, err := d.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", hostID, previousState, state, createdAt)
	Expect(err).To(BeNil())
}

func createHostEndingTransition(d db.QueryReceiver, hostID int64, previousState, state string, createdAt, endedAt time.Time) {
	_, err := d.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", hostID, previousState, state, createdAt, endedAt)
	Expect(err).To(BeNil())
}
