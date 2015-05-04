package biller_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "biller")
}

var _ = Describe("biller", func() {

	var (
		fakeStripe *FakeStripe

		endTime   time.Time
		startTime time.Time

		u   *user.User
		db  *ndb.Database
		lgr *logger.Logger
		b   *biller.Biller
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

		lgr = &logger.Logger{Level: logger.LogLevelQuiet}

		fakeStripe = &FakeStripe{}
		stripe.SetBackend(fakeStripe)

		startTime = time.Date(2014, 1, 1, 0, 0, 0, 0, time.Local)
		endTime = time.Date(2014, 1, 31, 24, 59, 59, 1000000000, time.Local)

		b, err = biller.New(db, lgr, u, &startTime, &endTime)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("CreateOrUpdateInvoice()", func() {
		It("should not do anything if there are not transitions for processing", func() {
			i, err := b.CreateOrUpdateInvoice()
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(biller.ErrNoUsage))
			Expect(i).To(BeNil())

			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE user_id = $1;`, u.ID).Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})

		Describe("with transitions", func() {
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
					P: &plan.Plan{
						Slug:          "hs1Plan",
						Name:          "hs1Plan",
						MaxContainers: 1,
						Price:         500,
						BasePrice:     500, // plan base price should be .745c/hr (745 millicents)
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

				_, err = db.Exec("UPDATE hosts SET created_at = $1, terminated_at = $2, updated_at = now() WHERE id = $3;", startTime.Add(-24*time.Hour), startTime.Add(2*time.Hour), h1.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h1.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h1.ID, "running", "terminated", startTime.Add(2*time.Hour))
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "running", "terminated", startTime.Add(2*time.Hour), startTime.Add(4*time.Hour))
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h2.ID, "terminated", "running", startTime.Add(4*time.Hour))
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h3.ID, "born", "running", startTime)
				Expect(err).To(BeNil())
			})

			It("should error out if the final price exceeds the maximum allowed ratio to the plan price", func() {
				var prevPrice int
				Expect(db.QueryRow("SELECT price FROM plans WHERE id = $1", h1.PlanID).Scan(&prevPrice)).To(BeNil())

				_, err := db.Exec("UPDATE plans SET price = 1 WHERE id = $1", h1.PlanID)
				Expect(err).To(BeNil())

				defer func() {
					_, err = db.Exec("UPDATE plans SET price = $1 WHERE id = $2", prevPrice, h1.PlanID)
					Expect(err).To(BeNil())
				}()

				_, err = b.CreateOrUpdateInvoice()
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("exceeds plan price ratio"))
			})

			It("should not perform anything if the monthly price of a plan is out of sync with a host pricing", func() {
				prevPrice := h2.HourlyPrice
				h2.HourlyPrice = 200
				Expect(h2.Update(db)).To(BeNil())

				defer func() {
					h2.HourlyPrice = prevPrice
					Expect(h2.Update(db)).To(BeNil())
				}()

				_, err := b.CreateOrUpdateInvoice()
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("exceeds ratio"))
			})

			It("should create an invoice for the hosts", func() {
				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())

				// h1 running for 2hrs = 4c
				// h1 base price for 2hrs = 1490 milli-cents = 1.49c = 2c
				// h2 running for full month - 2hrs = 744 - 2 = capped to 672 = 336c
				// h2 running for full month = 672 = 2501c = $25.01 (plan capped to $25.00)
				// total = 2842c
				Expect(i.Invoice.TotalCharge).To(Equal(int64(2842)))
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))
			})

			It("should discount the total cost of the invoice if there are credits available", func() {
				_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 8);", u.ID)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())

				Expect(i.Invoice.AvailableCredit).To(Equal(sql.NullInt64{8, true}))
				Expect(i.Invoice.CreditAmount).To(Equal(int64(8)))
				Expect(i.Invoice.TotalCharge).To(Equal(int64(2834)))
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))

				c := &credit.Credit{}
				err = db.QueryRowx("SELECT * FROM credits WHERE user_id = $1 AND invoice_id = $2", u.ID, i.Invoice.ID).StructScan(c)
				Expect(err).To(BeNil())
				Expect(c.Amount).To(Equal(int64(-8)))
			})

			It("should create invoice items for each host", func() {
				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))
				Expect(i.Items).To(HaveLen(4))

				var item1, item2, item3 invoiceitem.InvoiceItem
				var ebsItem1 invoiceitem.InvoiceItem
				for _, it := range i.Items {
					if it.HostID.Int64 == h1.ID {
						if it.ItemType == invoiceitem.ItemTypeCompute {
							item1 = it
						} else {
							ebsItem1 = it
						}
					} else if it.HostID.Int64 == h2.ID {
						item2 = it
					} else {
						item3 = it
					}
				}
				Expect(item1.ItemType).To(Equal(invoiceitem.ItemTypeCompute))
				Expect(item1.Terminated).To(BeTrue())
				Expect(item1.Hours.Int64).To(Equal(int64(2)))
				Expect(item1.HourlyPrice.Int64).To(Equal(int64(2000)))

				Expect(ebsItem1.ItemType).To(Equal(invoiceitem.ItemTypeBaseEBS))
				Expect(ebsItem1.Terminated).To(BeTrue())
				Expect(ebsItem1.Hours).To(Equal(sql.NullInt64{2, true}))
				Expect(ebsItem1.HourlyPrice).To(Equal(sql.NullInt64{745, true}))

				Expect(item3.ItemType).To(Equal(invoiceitem.ItemTypeCompute))
				Expect(item2.Terminated).To(BeFalse())
				Expect(item2.Hours.Int64).To(Equal(int64(672)))
				Expect(item2.HourlyPrice.Int64).To(Equal(int64(500)))

				Expect(item3.ItemType).To(Equal(invoiceitem.ItemTypeCompute))
				Expect(item3.Terminated).To(BeFalse())
				Expect(item3.Hours.Int64).To(Equal(int64(672)))
				Expect(item3.HourlyPrice.Int64).To(Equal(int64(3722)))
			})

			It("should create credit invoice items for the invoice if a credit is applied", func() {
				_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 8);", u.ID)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))
				Expect(i.Items).To(HaveLen(5))

				var credit invoiceitem.InvoiceItem
				for _, it := range i.Items {
					if it.Credit.Valid {
						credit = it
						break
					}
				}
				Expect(credit.Credit.Int64).To(Equal(int64(8)))
			})

			It("should set the state as not payable if the total charge is zero", func() {
				_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 3000);", u.ID)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.TotalCharge).To(Equal(int64(0)))
				Expect(i.Invoice.State).To(Equal(invoice.StateNotPayable))
			})

			It("should update an invoice if it exists for the same period", func() {
				inv := &invoice.Invoice{
					UserID:      u.ID,
					TotalCharge: 67,
					PeriodStart: &startTime,
					PeriodEnd:   &endTime,
					State:       invoice.StateUnpaid,
				}
				err := inv.Insert(db)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.ID).To(Equal(inv.ID))
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))
				Expect(i.Invoice.TotalCharge).To(Equal(int64(2842)))
			})

			It("should cover additional credits if the total has increased", func() {
				_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 8);", u.ID)
				Expect(err).To(BeNil())

				inv := &invoice.Invoice{
					UserID:       u.ID,
					TotalCharge:  0,
					CreditAmount: 2,
					PeriodStart:  &startTime,
					PeriodEnd:    &endTime,
					State:        invoice.StateUnpaid,
				}
				err = inv.Insert(db)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.ID).To(Equal(inv.ID))
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))
				Expect(i.Invoice.TotalCharge).To(Equal(int64(2832)))
				Expect(i.Invoice.CreditAmount).To(Equal(int64(10)))
				Expect(i.Invoice.AvailableCredit).To(Equal(sql.NullInt64{10, true}))
			})

			It("should update an invoice item if it exists", func() {
				inv := &invoice.Invoice{
					UserID:      u.ID,
					TotalCharge: 67,
					PeriodStart: &startTime,
					PeriodEnd:   &endTime,
					State:       invoice.StateUnpaid,
				}
				err := inv.Insert(db)
				Expect(err).To(BeNil())

				invItem := &invoiceitem.InvoiceItem{
					HostID:      sql.NullInt64{h1.ID, true},
					InvoiceID:   inv.ID,
					Hours:       sql.NullInt64{92, true},
					HourlyPrice: sql.NullInt64{30, true},
					ItemType:    invoiceitem.ItemTypeCompute,
					PlanMemory:  sql.NullString{"1GB", true},
				}

				err = invItem.Insert(db)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))

				var invItem2 invoiceitem.InvoiceItem
				for _, item := range i.Items {
					if item.ID == invItem.ID {
						invItem2 = item
						break
					}
				}

				Expect(invItem2.Hours.Int64).To(Equal(int64(2)))
				Expect(invItem2.HourlyPrice.Int64).To(Equal(int64(2000)))

				ebsItem := &invoiceitem.InvoiceItem{}
				err = db.QueryRowx("SELECT * FROM invoice_items WHERE host_id = $1 AND item_type = $2", h1.ID, invoiceitem.ItemTypeBaseEBS).StructScan(ebsItem)
				Expect(err).To(BeNil())
				Expect(ebsItem.Hours.Int64).To(Equal(int64(2)))
				Expect(ebsItem.HourlyPrice.Int64).To(Equal(int64(745)))
			})

			It("should insert additional credit invoice items to cover new credit", func() {
				_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 8);", u.ID)
				Expect(err).To(BeNil())

				inv := &invoice.Invoice{
					UserID:       u.ID,
					TotalCharge:  0,
					CreditAmount: 2,
					PeriodStart:  &startTime,
					PeriodEnd:    &endTime,
					State:        invoice.StateUnpaid,
				}
				err = inv.Insert(db)
				Expect(err).To(BeNil())

				invItem := &invoiceitem.InvoiceItem{
					InvoiceID: inv.ID,
					Credit:    sql.NullInt64{2, true},
					ItemType:  invoiceitem.ItemTypeCredit,
				}

				err = invItem.Insert(db)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))

				var credit invoiceitem.InvoiceItem
				for _, it := range i.Items {
					if it.Credit.Valid && it.Credit.Int64 > 2 {
						credit = it
					}
				}
				Expect(credit.Credit.Int64).To(Equal(int64(8)))
			})

			It("should allow for running multiple times", func() {
				_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 8);", u.ID)
				Expect(err).To(BeNil())

				i1, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i1).ToNot(BeNil())
				Expect(i1.Invoice.State).To(Equal(invoice.StateUnpaid))
				Expect(i1.Items).To(HaveLen(5))

				i2, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i2).ToNot(BeNil())
				Expect(i2.Invoice.State).To(Equal(invoice.StateUnpaid))
				Expect(i2.Invoice.ID).To(Equal(i1.Invoice.ID))
				Expect(i2.Invoice.TotalCharge).To(Equal(i1.Invoice.TotalCharge))
				Expect(i2.Invoice.CreditAmount).To(Equal(i1.Invoice.CreditAmount))
				Expect(i2.Invoice.AvailableCredit).To(Equal(i1.Invoice.AvailableCredit))

				var credit invoiceitem.InvoiceItem
				items, err := invoiceitem.FindAllForInvoice(db, i2.Invoice.ID)
				for _, it := range items {
					if it.Credit.Valid {
						credit = it
						break
					}
				}
				Expect(credit.Credit.Int64).To(Equal(int64(8)))
			})
		})
	})

	Describe("ProcessStripe", func() {
		var (
			i *biller.InvoiceResult
		)

		BeforeEach(func() {
			inv := &invoice.Invoice{
				UserID:      u.ID,
				PeriodStart: &startTime,
				PeriodEnd:   &endTime,
				State:       invoice.StateUnpaid,
			}

			item1 := invoiceitem.InvoiceItem{
				Hours:       sql.NullInt64{92, true},
				HourlyPrice: sql.NullInt64{1000, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"1 GB", true},
			}

			item2 := invoiceitem.InvoiceItem{
				Hours:       sql.NullInt64{672, true},
				HourlyPrice: sql.NullInt64{3722, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"19 GB", true},
			}

			i = &biller.InvoiceResult{
				Invoice: inv,
				Items:   []invoiceitem.InvoiceItem{item1, item2},
			}
		})

		It("should return an error if the user has been deleted profile does not exist", func() {
			u.DeletedAt = pq.NullTime{time.Now(), true}
			err := b.ProcessStripe(i)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("biller: skipping stripe, user %d has been DELETED", u.ID)))
		})

		It("should return an error if the billing profile does not exist", func() {
			_, err := db.Exec("DELETE FROM billing_profiles WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())
			err = b.ProcessStripe(i)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("biller: skipping stripe, failure obtaining billing profile for user"))
		})

		It("should throw an error if the billing profile is in no_card state", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateNoCard, u.ID)
			Expect(err).To(BeNil())

			err = b.ProcessStripe(i)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("biller: skipping stripe, billing profile for user %d is in state no_card", u.ID)))
		})

		It("should throw an error if the billing profile is in exempt state", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateExempt, u.ID)
			Expect(err).To(BeNil())

			err = b.ProcessStripe(i)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("biller: skipping stripe, billing profile for user %d is in state exempt", u.ID)))
			Expect(i.Invoice.State).To(Equal(invoice.StateNotPayable))
		})

		It("should throw an error if the billing profile is in free state", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())

			err = b.ProcessStripe(i)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("biller: skipping stripe, billing profile for user %d is in state free", u.ID)))
			Expect(i.Invoice.State).To(Equal(invoice.StateNotPayable))
		})

		It("should throw an error if the billing profile exists but without a StripeID", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1, stripe_id = '' WHERE user_id = $2;", billingprofile.StatePaid, u.ID)
			Expect(err).To(BeNil())

			err = b.ProcessStripe(i)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("biller: skipping stripe, billing profile for user %d lacks StripeID", u.ID)))
		})

		Context("with a valid billing profile", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE billing_profiles SET state = $1, stripe_id = 'St17' WHERE user_id = $2;", billingprofile.StatePaid, u.ID)
				Expect(err).To(BeNil())

				i.Invoice.TotalCharge = biller.MinimumInvoiceAmount - 1
			})

			It("should not create invoice items but check on outstanding invoice if charge is 0", func() {
				fakeStripe.Responses = []string{
					`{"object": "invoice", "id": null}`,
				}

				i.Invoice.TotalCharge = 0
				err := b.ProcessStripe(i)
				Expect(err).To(BeNil())
				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				Expect(fakeStripe.ReceivedRequests[0]["path"]).To(Equal("/invoices"))
			})

			It("should return an error if the host in the usage cannot be matched", func() {
				i.Items[0].HostID = sql.NullInt64{995, true}
				i.Items[1].HostID = sql.NullInt64{9991, true}

				err := b.ProcessStripe(i)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("Could not locate host 995"))
			})

			Context("with hosts", func() {
				var (
					hs1 *HostStack
					hs2 *HostStack
					h1  *host.Host
					h2  *host.Host
				)

				BeforeEach(func() {
					err := i.Invoice.Insert(db)
					Expect(err).To(BeNil())
					i.Items[0].InvoiceID = i.Invoice.ID
					i.Items[1].InvoiceID = i.Invoice.ID

					hs1, err = CreateHostStack(db, &HostStack{
						H: &host.Host{
							Slug:        "my-host-1",
							HourlyPrice: 2000,
						},
						U: u,
					})
					Expect(err).To(BeNil())
					h1 = hs1.H
					i.Items[0].HostID = sql.NullInt64{h1.ID, true}
					err = i.Items[0].Insert(db)
					Expect(err).To(BeNil())

					hs2, err = CreateHostStack(db, &HostStack{
						H: &host.Host{
							Slug:        "my-host-2",
							HourlyPrice: 3722,
						},
						P: &plan.Plan{
							Slug:          "hs2Plan",
							Name:          "hs2Plan",
							MaxContainers: 1,
							Price:         2500,
						},
						U: u,
					})
					Expect(err).To(BeNil())
					h2 = hs2.H
					i.Items[1].HostID = sql.NullInt64{h2.ID, true}
					err = i.Items[1].Insert(db)
					Expect(err).To(BeNil())
				})

				It("should submit the invoice items to Stripe", func() {
					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
						`{"object": "invoiceitem", "id": "ID2"}`,
						`{"object": "invoice", "id": "ID0"}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).To(BeNil())
					rr := fakeStripe.ReceivedRequests
					Expect(rr).To(HaveLen(3))

					Expect(rr[0]["amount"]).To(Equal("92"))
					Expect(rr[0]["customer"]).To(Equal("St17"))
					Expect(rr[0]["desc"]).To(Equal("'my-host-1' compute (1 GB memory) - 92 hours at 1.00c/hour"))

					Expect(rr[1]["amount"]).To(Equal("2500"))
					Expect(rr[1]["customer"]).To(Equal("St17"))
					Expect(rr[1]["desc"]).To(Equal("'my-host-2' compute (19 GB memory) - 672 hours at 3.722c/hour"))

					type s struct {
						ID string `db:"stripe_id"`
					}
					sIDs := []s{}
					err = db.Select(&sIDs, "SELECT stripe_id FROM invoice_items WHERE id IN ($1, $2) ORDER BY stripe_id;", i.Items[0].ID, i.Items[1].ID)
					Expect(err).To(BeNil())
					Expect(sIDs).To(Equal([]s{{ID: "ID1"}, {ID: "ID2"}}))
				})

				It("should rollback on any failure", func() {
					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).ToNot(BeNil())
					Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))

					var count int
					err = db.QueryRowx("SELECT COUNT(*) FROM invoice_items WHERE id IN ($1, $2) AND stripe_id != ''", i.Items[0].ID, i.Items[1].ID).Scan(&count)
					Expect(err).To(BeNil())
					Expect(count).To(Equal(0))
				})

				It("should submit any invoice credit items", func() {
					itm3 := invoiceitem.InvoiceItem{
						InvoiceID: i.Invoice.ID,
						Credit:    sql.NullInt64{20, true},
						ItemType:  invoiceitem.ItemTypeCredit,
					}
					Expect(itm3.Insert(db)).To(BeNil())
					i.Items = append(i.Items, itm3)

					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
						`{"object": "invoiceitem", "id": "ID2"}`,
						`{"object": "invoiceitem", "id": "ID3"}`,
						`{"object": "invoice", "id": "ID0"}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).To(BeNil())
					rr := fakeStripe.ReceivedRequests
					Expect(rr).To(HaveLen(4))

					Expect(rr[2]["amount"]).To(Equal("-20"))
					Expect(rr[2]["customer"]).To(Equal("St17"))
					Expect(rr[2]["desc"]).To(Equal("Credit of $0.20"))

					var stripeID string
					err = db.QueryRow(`SELECT stripe_id FROM invoice_items WHERE id = $1`, itm3.ID).Scan(&stripeID)
					Expect(err).To(BeNil())
					Expect(stripeID).To(Equal("ID3"))
				})

				It("should perform an invoice update if one of the invoice items has a Stripe ID", func() {
					i.Items[1].StripeID = sql.NullString{"XcV", true}
					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
						`{"object": "invoiceitem", "id": "ID2"}`,
						`{"object": "invoice", "id": "ID0"}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).To(BeNil())
					Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
					gotUpdate := false
					for _, r := range fakeStripe.ReceivedRequests {
						if r["path"] == "/invoiceitems/XcV" {
							gotUpdate = true
						}
					}
					Expect(gotUpdate).To(BeTrue())
				})

				It("should rollback on a update failure", func() {
					i.Items[1].StripeID = sql.NullString{"XcV", true}
					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).ToNot(BeNil())
					Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))

					var count int
					err = db.QueryRowx("SELECT COUNT(*) FROM invoice_items WHERE id IN ($1, $2) AND stripe_id != ''", i.Items[0].ID, i.Items[1].ID).Scan(&count)
					Expect(err).To(BeNil())
					Expect(count).To(Equal(0))
				})

				It("should query for upcoming invoice amount when the invoice amount is under the threshold", func() {
					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
						`{"object": "invoiceitem", "id": "ID2"}`,
						`{"object": "invoice", "id": "ID0", "amount_due": 0}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).To(BeNil())
					Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
					r := fakeStripe.ReceivedRequests[2]
					Expect(r["path"]).To(Equal("/invoices"))
					Expect(r["customer"]).To(Equal("St17"))

					Expect(i.Invoice.StripeID.String).To(BeEmpty())
					Expect(i.Invoice.State).To(Equal(invoice.StateDeferred))
				})

				It("should continue to submit an invoice if the pending amount is greater", func() {
					fakeStripe.Responses = []string{
						`{"object": "invoiceitem", "id": "ID1"}`,
						`{"object": "invoiceitem", "id": "ID2"}`,
						`{"object": "invoice", "id": "ID0", "amount_due": 1}`,
						`{"object": "invoice", "id": "ID1"}`,
					}

					err := b.ProcessStripe(i)
					Expect(err).To(BeNil())
					Expect(fakeStripe.ReceivedRequests).To(HaveLen(4))
					r := fakeStripe.ReceivedRequests[3]
					Expect(r["path"]).To(Equal("/invoices"))
					Expect(r["customer"]).To(Equal("St17"))

					Expect(i.Invoice.StripeID.String).To(Equal("ID1"))
					Expect(i.Invoice.State).To(Equal(invoice.StateUnpaid))
				})

				Context("invoice is over threshold", func() {
					BeforeEach(func() {
						i.Invoice.TotalCharge = biller.MinimumInvoiceAmount
					})

					It("should submit the invoice", func() {
						fakeStripe.Responses = []string{
							`{"object": "invoiceitem", "id": "ID1"}`,
							`{"object": "invoiceitem", "id": "ID2"}`,
							`{"object": "invoice", "id": "ID0"}`,
						}

						err := b.ProcessStripe(i)
						Expect(err).To(BeNil())
						Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
						r := fakeStripe.ReceivedRequests[2]
						Expect(r["path"]).To(Equal("/invoices"))
						Expect(r["customer"]).To(Equal("St17"))

						Expect(i.Invoice.StripeID.String).To(Equal("ID0"))
					})

					It("should bubble the error when submitting an invoice", func() {
						fakeStripe.Responses = []string{
							`{"object": "invoiceitem", "id": "ID1"}`,
							`{"object": "invoiceitem", "id": "ID2"}`,
						}

						err := b.ProcessStripe(i)
						Expect(err).ToNot(BeNil())
					})

					It("should not submit the invoice if it has already been submitted", func() {
						i.Invoice.StripeID = sql.NullString{"Sv", true}
						fakeStripe.Responses = []string{
							`{"object": "invoiceitem", "id": "ID1"}`,
							`{"object": "invoiceitem", "id": "ID2"}`,
							`{"object": "inoice", "id": "Sv"}`,
						}

						err := b.ProcessStripe(i)
						Expect(err).To(BeNil())
						Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
						r := fakeStripe.ReceivedRequests[2]
						Expect(r["path"]).To(Equal("/invoices/Sv"))
					})

					Context("other invoices that are deferred", func() {
						var (
							oInv1 *invoice.Invoice
							oInv2 *invoice.Invoice
						)
						BeforeEach(func() {
							s1 := time.Now().Add(-900 * time.Hour)
							e1 := s1.Add(30 * time.Hour)
							s2 := time.Now().Add(-300 * time.Hour)
							e2 := s2.Add(30 * time.Hour)
							oInv1 = &invoice.Invoice{
								UserID:      u.ID,
								PeriodStart: &s1,
								PeriodEnd:   &e1,
								State:       invoice.StateUnpaid,
							}
							Expect(oInv1.Insert(db)).To(BeNil())
							_, err := db.Exec("INSERT INTO invoice_items (invoice_id, host_id, hours, hourly_price, stripe_id, item_type) values ($1, $2, $3, $4, $5, $6);",
								oInv1.ID, h1.ID, 90, 1500, "INV1", invoiceitem.ItemTypeCompute)
							Expect(err).To(BeNil())
							_, err = db.Exec("INSERT INTO invoice_items (invoice_id, host_id, hours, hourly_price, stripe_id, item_type) values ($1, $2, $3, $4, $5, $6);",
								oInv1.ID, h2.ID, 20, 1000, "INV2", invoiceitem.ItemTypeCompute)
							Expect(err).To(BeNil())

							oInv2 = &invoice.Invoice{
								UserID:      u.ID,
								PeriodStart: &s2,
								PeriodEnd:   &e2,
								StripeID:    sql.NullString{"oInv2", true},
								State:       invoice.StateUnpaid,
							}
							Expect(oInv2.Insert(db)).To(BeNil())
							_, err = db.Exec("INSERT INTO invoice_items (invoice_id, host_id, hours, hourly_price, stripe_id, item_type) values ($1, $2, $3, $4, $5, $6);",
								oInv2.ID, h1.ID, 90, 1500, "INV3", invoiceitem.ItemTypeCompute)
							Expect(err).To(BeNil())
							_, err = db.Exec("INSERT INTO invoice_items (invoice_id, host_id, hours, hourly_price, stripe_id, item_type) values ($1, $2, $3, $4, $5, $6);",
								oInv2.ID, h2.ID, 20, 1000, "INV4", invoiceitem.ItemTypeCompute)
							Expect(err).To(BeNil())
						})

						It("should mark invoices as covered by the invoice that contains line items from other invoices", func() {
							fakeStripe.Responses = []string{
								`{"object": "invoiceitem", "id": "ID1"}`,
								`{"object": "invoiceitem", "id": "ID2"}`,
								`{"object": "invoice", "id": "ID0", "lines": {"data": [{"id": "INV1"},{"id": "INV2"},{"id": "INV3"},{"id": "INV4"}]}}`,
							}

							err := b.ProcessStripe(i)
							Expect(err).To(BeNil())
							Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
							r := fakeStripe.ReceivedRequests[2]
							Expect(r["path"]).To(Equal("/invoices"))
							Expect(r["customer"]).To(Equal("St17"))

							oInv1X := &invoice.Invoice{}
							err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", oInv1.ID).StructScan(oInv1X)
							Expect(err).To(BeNil())

							oInv2X := &invoice.Invoice{}
							err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", oInv2.ID).StructScan(oInv2X)
							Expect(err).To(BeNil())

							Expect(oInv1X.CoveredBy.Int64).To(Equal(i.Invoice.ID))
							Expect(oInv2X.CoveredBy.Int64).To(Equal(i.Invoice.ID))

							i2 := &invoice.Invoice{}
							err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", i.Invoice.ID).StructScan(i2)
							Expect(err).To(BeNil())
							Expect(i2.CoveredBy.Valid).To(BeFalse())
						})

						It("should not touch invoices that are already covered", func() {
							s3 := time.Now().Add(-100 * time.Hour)
							e3 := s3.Add(30 * time.Hour)
							oInv3 := &invoice.Invoice{
								UserID:      u.ID,
								PeriodStart: &s3,
								PeriodEnd:   &e3,
								State:       invoice.StateUnpaid,
							}
							Expect(oInv3.Insert(db)).To(BeNil())
							oInv1.CoveredBy = sql.NullInt64{oInv3.ID, true}
							Expect(oInv1.Update(db)).To(BeNil())

							fakeStripe.Responses = []string{
								`{"object": "invoiceitem", "id": "ID1"}`,
								`{"object": "invoiceitem", "id": "ID2"}`,
								`{"object": "invoice", "id": "ID0", "lines": {"data": [{"id": "INV1"},{"id": "INV2"},{"id": "INV3"},{"id": "INV4"}]}}`,
							}

							err := b.ProcessStripe(i)
							Expect(err).To(BeNil())
							Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
							r := fakeStripe.ReceivedRequests[2]
							Expect(r["path"]).To(Equal("/invoices"))
							Expect(r["customer"]).To(Equal("St17"))

							oInv1X := &invoice.Invoice{}
							err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", oInv1.ID).StructScan(oInv1X)
							Expect(err).To(BeNil())

							oInv2X := &invoice.Invoice{}
							err = db.QueryRowx("SELECT * FROM invoices WHERE id = $1", oInv2.ID).StructScan(oInv2X)
							Expect(err).To(BeNil())

							Expect(oInv1X.CoveredBy.Int64).To(Equal(oInv3.ID))
							Expect(oInv2X.CoveredBy.Int64).To(Equal(i.Invoice.ID))
						})
					})
				})
			})
		})
	})

})
