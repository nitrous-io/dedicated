package biller_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/biller"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
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

		startTime = time.Now().Add(-48 * time.Hour)
		endTime = startTime.Add(12 * time.Hour)

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
			Expect(err.Error()).To(Equal(fmt.Sprintf("biller: no usage from user %d", u.ID)))
			Expect(i).To(BeNil())

			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE user_id = $1;`, u.ID).Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})

		Describe("with transisitions", func() {
			var (
				hs1 *HostStack
				hs2 *HostStack
				h1  *host.Host
				h2  *host.Host
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

				_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", h1.ID)
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

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h1.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h1.ID, "running", "stopped", startTime.Add(2*time.Hour))
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "running", "stopped", startTime.Add(2*time.Hour), startTime.Add(4*time.Hour))
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h2.ID, "stopped", "running", startTime.Add(4*time.Hour))
				Expect(err).To(BeNil())
			})

			It("should create an invoice for the hosts", func() {
				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())

				// h1 running for 2hrs = 4c
				// h2 running for 10hrs = 5c
				// total = 9c
				Expect(i.Invoice.TotalCharge).To(Equal(int64(9)))
			})

			It("should create invoice items for each host", func() {
				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Items).To(HaveLen(2))

				var item1 invoiceitem.InvoiceItem
				var item2 invoiceitem.InvoiceItem
				for _, it := range i.Items {
					if it.HostID == h1.ID {
						item1 = it
					} else {
						item2 = it
					}
				}
				Expect(item1.Hours).To(Equal(2))
				Expect(item1.HourlyPrice).To(Equal(int64(2000)))
				Expect(item2.Hours).To(Equal(10))
				Expect(item2.HourlyPrice).To(Equal(int64(500)))
			})

			It("should update an invoice if it exists for the same period", func() {
				inv := &invoice.Invoice{
					UserID:           u.ID,
					TotalCharge:      67,
					PeriodStart:      &startTime,
					PeriodEnd:        &endTime,
					SuccessfulCharge: false,
				}
				err := inv.Insert(db)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())
				Expect(i.Invoice.ID).To(Equal(inv.ID))
				Expect(i.Invoice.SuccessfulCharge).To(BeFalse())
				Expect(i.Invoice.TotalCharge).To(Equal(int64(9)))
			})

			It("should update an invoice item if it exists", func() {
				inv := &invoice.Invoice{
					UserID:           u.ID,
					TotalCharge:      67,
					PeriodStart:      &startTime,
					PeriodEnd:        &endTime,
					SuccessfulCharge: false,
				}
				err := inv.Insert(db)
				Expect(err).To(BeNil())

				invItem := &invoiceitem.InvoiceItem{
					HostID:      h1.ID,
					InvoiceID:   inv.ID,
					Hours:       92,
					HourlyPrice: 30,
				}

				err = invItem.Insert(db)
				Expect(err).To(BeNil())

				i, err := b.CreateOrUpdateInvoice()
				Expect(err).To(BeNil())
				Expect(i).ToNot(BeNil())

				var invItem2 invoiceitem.InvoiceItem
				for _, item := range i.Items {
					if item.ID == invItem.ID {
						invItem2 = item
						break
					}
				}

				Expect(invItem2.Hours).To(Equal(2))
				Expect(invItem2.HourlyPrice).To(Equal(int64(2000)))
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
			}

			item1 := invoiceitem.InvoiceItem{
				Hours:       92,
				HourlyPrice: 1000,
			}

			item2 := invoiceitem.InvoiceItem{
				Hours:       20,
				HourlyPrice: 500,
			}

			i = &biller.InvoiceResult{
				Invoice: inv,
				Items:   []invoiceitem.InvoiceItem{item1, item2},
			}
		})

		It("should not continue if billing is not enabled", func() {
			u.BillingEnabled = false

			err := b.ProcessStripe(i)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(BeEmpty())
		})

		It("should not continue if billing is enabled but total charge is 0", func() {
			u.BillingEnabled = true

			i.Invoice.TotalCharge = 0
			err := b.ProcessStripe(i)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(BeEmpty())
		})

		Context("with a total charge", func() {
			BeforeEach(func() {
				u.BillingEnabled = true
				i.Invoice.TotalCharge = 19
			})

			It("should return an error if the billing profile does not exist", func() {
				err := b.ProcessStripe(i)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("biller: failure obtaining billing profile for user"))
			})

			It("should throw and error if the billing profile exists but without a StripeID", func() {
				_, err := db.Exec("UPDATE billing_profiles SET stripe_id = '' WHERE user_id = $1;", u.ID)
				Expect(err).To(BeNil())

				err = b.ProcessStripe(i)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("biller: failure obtaining billing profile for user"))
			})

			Context("with a valid billing profile", func() {
				BeforeEach(func() {
					_, err := db.Exec("UPDATE billing_profiles SET stripe_id = 'St17' WHERE user_id = $1;", u.ID)
					Expect(err).To(BeNil())
				})

				It("should return an error if the host in the usage cannot be matched", func() {
					i.Items[0].HostID = 995
					i.Items[1].HostID = 9991

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
						i.Items[0].HostID = h1.ID
						err = i.Items[0].Insert(db)
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
						i.Items[1].HostID = h2.ID
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
						Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
						var r1 map[string]string
						var r2 map[string]string
						for _, r := range fakeStripe.ReceivedRequests {
							if r["path"] != "/invoiceitems" {
								continue
							}
							if r["amount"] == "92" {
								r1 = r
							} else if r["amount"] == "10" {
								r2 = r
							}
						}
						Expect(r1["customer"]).To(Equal("St17"))
						Expect(r1["desc"]).To(Equal("Host 'my-host-1' 92 hours at 1.00c/hour"))

						Expect(r2["customer"]).To(Equal("St17"))
						Expect(r2["desc"]).To(Equal("Host 'my-host-2' 20 hours at 0.50c/hour"))

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
						}

						err := b.ProcessStripe(i)
						Expect(err).To(BeNil())
						Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
						r := fakeStripe.ReceivedRequests[1]
						Expect(r["path"]).ToNot(Equal("/invoices"))
					})
				})
			})
		})
	})

	Describe(".InvoiceItemsForUsages()", func() {
		It("should calculate the total charges and return invoiceItems", func() {
			usage1 := hosttransition.HostHourUsage{
				HostID:      56,
				UserID:      98,
				HourlyPrice: 500,   // 0.5cent/hr
				Seconds:     86400, // 24hrs
			} // total should be 12 cents
			usage2 := hosttransition.HostHourUsage{
				HostID:      53,
				UserID:      98,
				HourlyPrice: 2000,   // 2cent/hr
				Seconds:     172800, // 48hrs
			} // total should be 96 cents
			usage3 := hosttransition.HostHourUsage{
				HostID:      54,
				UserID:      98,
				HourlyPrice: 0,      // 0cent/hr
				Seconds:     172800, // 48hrs
			} // total should be 0 cents

			charge, items := biller.InvoiceItemsForUsages([]hosttransition.HostHourUsage{usage1, usage2, usage3})
			Expect(charge).To(Equal(int64(108)))

			eX1 := invoiceitem.InvoiceItem{
				HostID:      56,
				HourlyPrice: 500,
				Hours:       24,
			}
			eX2 := invoiceitem.InvoiceItem{
				HostID:      53,
				HourlyPrice: 2000,
				Hours:       48,
			}
			eX3 := invoiceitem.InvoiceItem{
				HostID:      54,
				HourlyPrice: 0,
				Hours:       48,
			}
			Expect(items).To(Equal([]invoiceitem.InvoiceItem{eX1, eX2, eX3}))
		})
	})

})
