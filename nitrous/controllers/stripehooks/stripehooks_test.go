package stripehooks_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/stripehooks"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/stripe/stripe-go"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/stripehooks")
}

var _ = Describe("controllers/stripehooks", func() {
	var (
		fakeStripe *FakeStripe
		db         *db.Database
		u          *user.User
		svr        *server.Server

		succeedFnCalled        bool
		receivedSucceedInvoice *invoice.Invoice

		failedFnCalled                  bool
		receivedFailedInvoice           *invoice.Invoice
		receivedFailedAttemptCount      int
		receivedFailedAttemptsExhausted bool

		inv *invoice.Invoice
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil, &dns.FakeProvider{})

		fakeStripe = &FakeStripe{}
		stripe.SetBackend(fakeStripe)

		stripehooks.HandlePaymentSucceededFn = func(env *middleware.Env, inv *invoice.Invoice) {
			receivedSucceedInvoice = inv
			succeedFnCalled = true
		}
		stripehooks.HandlePaymentFailedFn = func(env *middleware.Env, inv *invoice.Invoice, attemptCount int, attemptsExhausted bool) {
			receivedFailedInvoice = inv
			receivedFailedAttemptCount = attemptCount
			receivedFailedAttemptsExhausted = attemptsExhausted
			failedFnCalled = true
		}

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
			Name:     "Steve",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		start := time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)
		inv = &invoice.Invoice{
			UserID:      u.ID,
			TotalCharge: 900,
			Attempts:    23,
			State:       invoice.StatePaid,
			StripeID:    sql.NullString{"Xcccs", true},
			PeriodStart: &start,
			PeriodEnd:   &end,
		}
		err = inv.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/stripe_hook", func() {
		doRequest := func(jsonStr string) *httptest.ResponseRecorder {
			req, err := NewBodyRequest("POST", "/api/v0/stripe_hook", jsonStr)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("immediately returns after receiving data", func() {
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "hello"}`)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("should verify the event with Stripe", func() {
			fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV"}`}

			succeedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() int {
				return len(fakeStripe.ReceivedRequests)
			}).Should(Equal(1))

			req := fakeStripe.ReceivedRequests[0]
			Expect(req["method"]).To(Equal("GET"))
			Expect(req["path"]).To(Equal("/events/evt_158gxvAOjoYioo6erVsU1zBV"))

			// These are false because the event type is neither payment
			// succeeded or payment failed.
			Expect(succeedFnCalled).To(BeFalse())
			Expect(failedFnCalled).To(BeFalse())
		})

		It("should not handle invoices that are not found in the database", func() {
			fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded", "data": {"object": {"id": "invalid-invoice-id"}}}`}

			succeedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() int {
				return len(fakeStripe.ReceivedRequests)
			}).Should(Equal(1))

			Expect(succeedFnCalled).To(BeFalse())
		})

		It("should handle the payment as succeeded if invoice is paid", func() {
			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_succeeded", "data": {"object": {"id": "Xcccs"}}}`,
				`{"object": "invoice", "id": "Sv", "paid": true}`,
			}

			succeedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() bool {
				return succeedFnCalled
			}).Should(BeTrue())

			Expect(receivedSucceedInvoice).To(Equal(inv))
		})

		It("should handle the payment as failed if invoice is not paid", func() {
			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": 123455}}}`,
				`{"object": "invoice", "id": "Sv", "paid": false, "attempt_count": 2, "next_payment_attempt": 192075622}`,
			}

			failedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() bool {
				return failedFnCalled
			}).Should(BeTrue())

			Expect(receivedFailedInvoice).To(Equal(inv))
			Expect(receivedFailedAttemptCount).To(Equal(2))
			Expect(receivedFailedAttemptsExhausted).To(BeFalse())
		})

		It("should return true for attempts exhausted when NextAttempt is 0", func() {
			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": null}}}`,
				`{"object": "invoice", "id": "Sv", "paid": false, "attempt_count": 2, "next_payment_attempt": null}`,
			}

			failedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() bool {
				return failedFnCalled
			}).Should(BeTrue())

			Expect(receivedFailedAttemptsExhausted).To(BeTrue())
		})

		It("should publish a EventInvoiceUpdated to the user", func(done Done) {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h1 := hs1.H
			itm1 := invoiceitem.InvoiceItem{
				InvoiceID:   inv.ID,
				HostID:      sql.NullInt64{h1.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{2000, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"2 GB", true},
			}
			Expect(itm1.Insert(db)).To(BeNil())

			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_succeeded", "data": {"object": {"id": "Xcccs"}}}`,
				`{"object": "invoice", "id": "Sv", "paid": true}`,
			}

			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			succeedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)

			Expect(res.Code).To(Equal(http.StatusOK))

			var e *controllers.Event
			timeout := time.After(1000 * time.Millisecond)
			for {
				if e != nil && e.Name == controllers.EventInvoiceUpdated {
					break
				}
				select {
				case ev := <-ch:
					e = ev.(*controllers.Event)
				case <-timeout:
					Fail("expected EventInvoiceUpdated but timed out")
				}
			}

			evtData, _ := e.Data.(map[string]interface{})
			inv, ok := evtData["invoice"].(*serializers.Invoice)
			Expect(ok).To(BeTrue())
			Expect(inv.ID).To(Equal(inv.ID))
			Expect(inv.Items[0].ID).To(Equal(itm1.ID))
			Expect(inv.Items[0].Description).To(ContainSubstring(h1.Slug))
			close(done)
		})
	})

	Describe("SerializeInvoices()", func() {
		var (
			h1, h2           *host.Host
			itm1, itm2, itm3 *invoiceitem.InvoiceItem
		)

		BeforeEach(func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h1 = hs1.H
			t := time.Now()
			hs2, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:         "my-host-2",
					HourlyPrice:  2000,
					TerminatedAt: &t,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h2 = hs2.H
			itm1 = &invoiceitem.InvoiceItem{
				InvoiceID:   inv.ID,
				HostID:      sql.NullInt64{h1.ID, true},
				Hours:       sql.NullInt64{24, true},
				HourlyPrice: sql.NullInt64{2000, true},
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"2 GB", true},
			}
			Expect(itm1.Insert(db)).To(BeNil())
			itm2 = &invoiceitem.InvoiceItem{
				InvoiceID: inv.ID,
				Credit:    sql.NullInt64{200, true},
				ItemType:  invoiceitem.ItemTypeCredit,
			}
			Expect(itm2.Insert(db)).To(BeNil())
			itm3 = &invoiceitem.InvoiceItem{
				InvoiceID:   inv.ID,
				HostID:      sql.NullInt64{h2.ID, true},
				Hours:       sql.NullInt64{20, true},
				HourlyPrice: sql.NullInt64{1000, true},
				Terminated:  true,
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			Expect(itm3.Insert(db)).To(BeNil())
		})

		It("should detail the correct amount of invoice items", func() {
			sInv, err := stripehooks.SerializeInvoice(db, inv, nil)
			Expect(err).To(BeNil())
			Expect(sInv.Items).To(HaveLen(2))
			Expect(sInv.TotalCredit).To(Equal(int64(200)))
		})

		It("should provide information about the hosts", func() {
			sInv, err := stripehooks.SerializeInvoice(db, inv, nil)
			Expect(err).To(BeNil())
			Expect(sInv.Items[0].Terminated).To(BeTrue())
			Expect(sInv.Items[1].Terminated).To(BeFalse())

			Expect(sInv.Items[0].Description).To(Equal(invoiceitem.ItemDescription(itm3, h2)))
			Expect(sInv.Items[1].Description).To(Equal(invoiceitem.ItemDescription(itm1, h1)))
		})
	})
})
