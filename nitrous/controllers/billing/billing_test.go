package billing_test

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/billing")
}

var _ = Describe("controllers/billing", func() {
	var (
		db  *db.Database
		u   *user.User
		t   *token.Token
		svr *server.Server
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)

		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "foobarbazqux",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/billing/current_usage", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", "/api/v0/billing/current_usage", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should return an empty items with no host transitions", func() {
			j := map[string]interface{}{}
			res := doRequest()
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j["items"]).To(BeEmpty())
		})

		It("should display the available credits", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 850);", u.ID)
			Expect(err).To(BeNil())

			var j struct {
				AvailableCreditsDollars float64 `json:"available_credits_dollars"`
			}
			res := doRequest()
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.AvailableCreditsDollars).To(Equal(8.50))
		})

		It("should transform this month's transition info into invoice items", func() {
			startTime := time.Now().Add(-2 * time.Second)
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h1 := hs1.H

			_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", h1.ID)
			Expect(err).To(BeNil())

			hs2, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 500,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h2 := hs2.H

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h1.ID, "born", "running", startTime, startTime.Add(1*time.Second))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h1.ID, "running", "stopped", startTime.Add(1*time.Second))
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "born", "running", startTime, startTime.Add(1*time.Second))
			Expect(err).To(BeNil())

			var j struct {
				Items              []serializers.InvoiceItem `json:"items"`
				TotalCharge        int64                     `json:"total_charge"`
				TotalChargeDollars float64                   `json:"total_charge_dollars"`
			}
			res := doRequest()
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.TotalCharge).To(Equal(int64(3)))
			Expect(j.TotalChargeDollars).To(Equal(0.03))
			Expect(j.Items).To(HaveLen(2))

			var itm1 serializers.InvoiceItem
			var itm2 serializers.InvoiceItem
			for _, itm := range j.Items {
				if itm.HostID == h1.ID {
					itm1 = itm
				} else {
					itm2 = itm
				}
			}

			Expect(itm1.HourlyCents).To(Equal(2.0))
			Expect(itm1.Hours).To(Equal(int64(1)))

			Expect(itm2.HourlyCents).To(Equal(0.5))
			Expect(itm2.Hours).To(Equal(int64(1)))
		})
	})

	Describe("GET /api/v0/billing/invoices", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", "/api/v0/billing/invoices", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should return an empty items with no invoices", func() {
			j := map[string]interface{}{}
			res := doRequest()
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j["invoices"]).To(BeEmpty())
		})

		Context("with invoices", func() {
			var (
				hs1, hs2, hs3 *HostStack
				h1, h2, h3    *host.Host
				i1, i2, i3    *invoice.Invoice
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

				hs3, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:        "my-host-3",
						HourlyPrice: 500,
					},
					U: u,
				})
				Expect(err).To(BeNil())
				h3 = hs3.H

				pS1 := time.Now()
				pE1 := pS1.Add(-2 * time.Minute)
				i1 = &invoice.Invoice{
					UserID:           u.ID,
					SuccessfulCharge: false,
					PeriodStart:      &pS1,
					PeriodEnd:        &pE1,
					TotalCharge:      290,
				}
				Expect(i1.Insert(db)).To(BeNil())

				pS2 := time.Now().Add(10 * time.Minute)
				pE2 := pS1.Add(-2 * time.Minute)
				i2 = &invoice.Invoice{
					UserID:           u.ID,
					SuccessfulCharge: true,
					PeriodStart:      &pS2,
					PeriodEnd:        &pE2,
					TotalCharge:      300,
				}
				Expect(i2.Insert(db)).To(BeNil())

				pS3 := time.Now().Add(10 * time.Minute)
				pE3 := pS1.Add(-2 * time.Minute)
				i3 = &invoice.Invoice{
					UserID:           u.ID,
					SuccessfulCharge: false,
					PeriodStart:      &pS3,
					PeriodEnd:        &pE3,
					TotalCharge:      100,
				}
				Expect(i3.Insert(db)).To(BeNil())
			})

			It("should list out the invoices", func() {
				var j map[string][]serializers.Invoice
				res := doRequest()
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				invoices := j["invoices"]
				Expect(invoices).To(HaveLen(3))
				Expect(invoices[0].ID).To(Equal(i3.ID))
				Expect(invoices[0].UserID).To(Equal(i3.UserID))
				Expect(invoices[0].PeriodStart.Unix()).To(Equal(i3.PeriodStart.Unix()))
				Expect(invoices[0].PeriodEnd.Unix()).To(Equal(i3.PeriodEnd.Unix()))
				Expect(invoices[0].SuccessfulCharge).To(Equal(i3.SuccessfulCharge))
				Expect(invoices[0].Items).To(BeEmpty())

				Expect(invoices[1].ID).To(Equal(i2.ID))
				Expect(invoices[1].UserID).To(Equal(i2.UserID))
				Expect(invoices[1].PeriodStart.Unix()).To(Equal(i2.PeriodStart.Unix()))
				Expect(invoices[1].PeriodEnd.Unix()).To(Equal(i2.PeriodEnd.Unix()))
				Expect(invoices[1].SuccessfulCharge).To(Equal(i2.SuccessfulCharge))
				Expect(invoices[1].Items).To(BeEmpty())

				Expect(invoices[2].ID).To(Equal(i1.ID))
				Expect(invoices[2].UserID).To(Equal(i1.UserID))
				Expect(invoices[2].PeriodStart.Unix()).To(Equal(i1.PeriodStart.Unix()))
				Expect(invoices[2].PeriodEnd.Unix()).To(Equal(i1.PeriodEnd.Unix()))
				Expect(invoices[2].SuccessfulCharge).To(Equal(i1.SuccessfulCharge))
				Expect(invoices[2].Items).To(BeEmpty())
			})

			It("should be able to list the invoice items", func() {
				itm1 := invoiceitem.InvoiceItem{
					InvoiceID:   i1.ID,
					HostID:      sql.NullInt64{h1.ID, true},
					Hours:       sql.NullInt64{24, true},
					HourlyPrice: sql.NullInt64{2000, true},
				}
				Expect(itm1.Insert(db)).To(BeNil())
				itm2 := invoiceitem.InvoiceItem{
					InvoiceID:   i1.ID,
					HostID:      sql.NullInt64{h2.ID, true},
					Hours:       sql.NullInt64{48, true},
					HourlyPrice: sql.NullInt64{1000, true},
				}
				Expect(itm2.Insert(db)).To(BeNil())

				itm3 := invoiceitem.InvoiceItem{
					InvoiceID:   i2.ID,
					HostID:      sql.NullInt64{h2.ID, true},
					Hours:       sql.NullInt64{10, true},
					HourlyPrice: sql.NullInt64{3000, true},
				}
				Expect(itm3.Insert(db)).To(BeNil())

				itm4 := invoiceitem.InvoiceItem{
					InvoiceID:   i3.ID,
					HostID:      sql.NullInt64{h1.ID, true},
					Hours:       sql.NullInt64{8, true},
					HourlyPrice: sql.NullInt64{1000, true},
				}
				Expect(itm4.Insert(db)).To(BeNil())
				itm5 := invoiceitem.InvoiceItem{
					InvoiceID:   i3.ID,
					HostID:      sql.NullInt64{h2.ID, true},
					Hours:       sql.NullInt64{12, true},
					HourlyPrice: sql.NullInt64{500, true},
				}
				Expect(itm5.Insert(db)).To(BeNil())
				itm6 := invoiceitem.InvoiceItem{
					InvoiceID:   i3.ID,
					HostID:      sql.NullInt64{h3.ID, true},
					Hours:       sql.NullInt64{6, true},
					HourlyPrice: sql.NullInt64{2000, true},
				}
				Expect(itm6.Insert(db)).To(BeNil())

				var j map[string][]serializers.Invoice
				res := doRequest()
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				invoices := j["invoices"]

				Expect(invoices[0].Items).To(HaveLen(3))
				Expect(invoices[0].Items[0].InvoiceID).To(Equal(i3.ID))
				Expect(invoices[0].Items[0].HostID).To(Equal(h3.ID))
				Expect(invoices[0].Items[0].Hours).To(Equal(int64(6)))
				Expect(invoices[0].Items[0].HourlyCents).To(Equal(2.00))
				Expect(invoices[0].Items[0].TotalCharge).To(Equal(int64(12)))
				Expect(invoices[0].Items[0].HostSlug).To(Equal("my-host-3"))
				Expect(invoices[0].Items[1].InvoiceID).To(Equal(i3.ID))
				Expect(invoices[0].Items[1].HostID).To(Equal(h2.ID))
				Expect(invoices[0].Items[1].Hours).To(Equal(int64(12)))
				Expect(invoices[0].Items[1].HourlyCents).To(Equal(0.50))
				Expect(invoices[0].Items[1].TotalCharge).To(Equal(int64(6)))
				Expect(invoices[0].Items[1].HostSlug).To(Equal("my-host-2"))
				Expect(invoices[0].Items[2].InvoiceID).To(Equal(i3.ID))
				Expect(invoices[0].Items[2].HostID).To(Equal(h1.ID))
				Expect(invoices[0].Items[2].Hours).To(Equal(int64(8)))
				Expect(invoices[0].Items[2].HourlyCents).To(Equal(1.00))
				Expect(invoices[0].Items[2].TotalCharge).To(Equal(int64(8)))
				Expect(invoices[0].Items[2].HostSlug).To(Equal("my-host-1"))

				Expect(invoices[1].Items).To(HaveLen(1))
				Expect(invoices[1].Items[0].InvoiceID).To(Equal(i2.ID))
				Expect(invoices[1].Items[0].HostID).To(Equal(h2.ID))
				Expect(invoices[1].Items[0].Hours).To(Equal(int64(10)))
				Expect(invoices[1].Items[0].HourlyCents).To(Equal(3.00))
				Expect(invoices[1].Items[0].TotalCharge).To(Equal(int64(30)))
				Expect(invoices[1].Items[0].HostSlug).To(Equal("my-host-2"))

				Expect(invoices[2].Items).To(HaveLen(2))
				Expect(invoices[2].Items[0].InvoiceID).To(Equal(i1.ID))
				Expect(invoices[2].Items[0].HostID).To(Equal(h2.ID))
				Expect(invoices[2].Items[0].Hours).To(Equal(int64(48)))
				Expect(invoices[2].Items[0].HourlyCents).To(Equal(1.00))
				Expect(invoices[2].Items[0].TotalCharge).To(Equal(int64(48)))
				Expect(invoices[2].Items[0].HostSlug).To(Equal("my-host-2"))
				Expect(invoices[2].Items[1].InvoiceID).To(Equal(i1.ID))
				Expect(invoices[2].Items[1].HostID).To(Equal(h1.ID))
				Expect(invoices[2].Items[1].Hours).To(Equal(int64(24)))
				Expect(invoices[2].Items[1].HourlyCents).To(Equal(2.00))
				Expect(invoices[2].Items[1].TotalCharge).To(Equal(int64(48)))
				Expect(invoices[2].Items[1].HostSlug).To(Equal("my-host-1"))
			})
		})
	})
})
