package stripehooks_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/controllers/stripehooks"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
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
		svr        *server.Server

		succeedFnCalled        bool
		receivedSucceedInvoice *invoice.Invoice

		failedFnCalled             bool
		receivedFailedInvoice      *invoice.Invoice
		receivedFailedAttemptCount int
		receivedFailedMoreAttempts bool

		inv *invoice.Invoice
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)

		fakeStripe = &FakeStripe{}
		stripe.SetBackend(fakeStripe)

		stripehooks.HandlePaymentSucceededFn = func(env *middleware.Env, inv *invoice.Invoice) {
			receivedSucceedInvoice = inv
			succeedFnCalled = true
		}
		stripehooks.HandlePaymentFailedFn = func(env *middleware.Env, inv *invoice.Invoice, attemptCount int, moreAttempts bool) {
			receivedFailedInvoice = inv
			receivedFailedAttemptCount = attemptCount
			receivedFailedMoreAttempts = moreAttempts
			failedFnCalled = true
		}

		u := &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
			Name:     "Steve",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		inv = &invoice.Invoice{
			UserID:      u.ID,
			TotalCharge: 900,
			Attempts:    23,
			StripeID:    sql.NullString{"Xcccs", true},
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

		It("should handle the payment as succeeded if invoice.payment_succeeded", func() {
			fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_succeeded", "data": {"object": {"id": "Xcccs"}}}`}

			succeedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() bool {
				return succeedFnCalled
			}).Should(BeTrue())

			Expect(receivedSucceedInvoice).To(Equal(inv))
		})

		It("should handle the payment as failed if invoice.payment_failed", func() {
			fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": 123455}}}`}

			failedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() bool {
				return failedFnCalled
			}).Should(BeTrue())

			Expect(receivedFailedInvoice).To(Equal(inv))
			Expect(receivedFailedAttemptCount).To(Equal(2))
			Expect(receivedFailedMoreAttempts).To(BeTrue())
		})

		It("should handle the payment next_payment_attempt being null with a invoice.payment_failed", func() {
			fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": null}}}`}

			failedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			Eventually(func() bool {
				return failedFnCalled
			}).Should(BeTrue())

			Expect(receivedFailedMoreAttempts).To(BeFalse())
		})
	})
})
