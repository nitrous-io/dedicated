package stripehooks_test

import (
	"database/sql"
	"errors"
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
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/stripehooks")
}

var _ = Describe("controllers/stripehooks", func() {
	var (
		fakeStripe *test.FakeStripe
		db         *db.Database
		u          *user.User
		svr        *server.Server
		ft         *test.FakeTracker

		succeedFnCalled        bool
		succeedFnErr           error
		receivedSucceedInvoice *invoice.Invoice

		fsub *test.FakeSubscriptionProvider

		inv *invoice.Invoice
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)

		fakeStripe = &test.FakeStripe{}
		stripe.SetBackend(fakeStripe)

		succeedFnCalled = false
		succeedFnErr = nil
		stripehooks.HandlePaymentSucceededFn = func(env *middleware.Env, inv *invoice.Invoice) error {
			receivedSucceedInvoice = inv
			succeedFnCalled = true
			return succeedFnErr
		}

		fsub = &test.FakeSubscriptionProvider{}
		stripehooks.SetSubscriptionProvider(fsub)

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
			Name:     "Steve",
		}
		err = test.WrapInTransaction(db, u.Insert)
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
			req, err := test.NewBodyRequest("POST", "/api/v0/stripe_hook", jsonStr)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("immediately returns after receiving data", func() {
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "hello"}`)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("should verify the event with Stripe and return an error", func() {
			fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV"}`}

			succeedFnCalled = false
			res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			req := fakeStripe.ReceivedRequests[0]
			Expect(req["method"]).To(Equal("GET"))
			Expect(req["path"]).To(Equal("/events/evt_158gxvAOjoYioo6erVsU1zBV"))

			// These are false because the event type is neither payment
			// succeeded or payment failed.
			Expect(succeedFnCalled).To(BeFalse())
			Expect(fsub.HandleSubscriptionInvoiceFailedCalled).To(BeFalse())
		})

		Describe("with charges", func() {

			It("should handle successful charge", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
				}

				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "charge.succeeded"}`)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(fsub.HandleChargeSucceededCalled).To(BeTrue())

				e := fsub.HandleChargeSucceededEvent
				Expect(e.GetObjValue("source", "customer")).To(Equal("cus_123"))
			})

			It("should handle failed charge", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.failed", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
				}

				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "charge.failed"}`)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(fsub.HandleChargeFailedCalled).To(BeTrue())

				e := fsub.HandleChargeFailedEvent
				Expect(e.GetObjValue("source", "customer")).To(Equal("cus_123"))
			})

			It("should return an error if the HandleChargeSucceeded returns an error", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
				}

				fsub.HandleChargeSucceededError = errors.New("Woops!")
				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "charge.succeeded"}`)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				e := fsub.HandleChargeSucceededEvent
				Expect(e.GetObjValue("source", "customer")).To(Equal("cus_123"))
			})

			It("should return an error if the HandleChargeFailed returns an error", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.failed", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
				}

				fsub.HandleChargeFailedError = errors.New("Woops!")
				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "charge.failed"}`)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				e := fsub.HandleChargeFailedEvent
				Expect(e.GetObjValue("source", "customer")).To(Equal("cus_123"))
			})
		})

		Describe("with invoices", func() {

			It("should not handle invoices that are not found in the database and return an error", func() {
				fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded", "data": {"object": {"id": "invalid-invoice-id"}}}`}

				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				Expect(succeedFnCalled).To(BeFalse())
			})

			It("should handle the payment as succeeded if invoice is paid", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_succeeded", "data": {"object": {"id": "Xcccs"}}}`,
					`{"object": "invoice", "id": "Sv", "paid": true}`,
				}

				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(succeedFnCalled).To(BeTrue())
				Expect(receivedSucceedInvoice).To(Equal(inv))
			})

			It("should return an error if succeed function returns an error", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_succeeded", "data": {"object": {"id": "Xcccs"}}}`,
					`{"object": "invoice", "id": "Sv", "paid": true}`,
				}

				succeedFnErr = errors.New("Woops!")
				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_succeeded"}`)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				Expect(succeedFnCalled).To(BeTrue())
				Expect(receivedSucceedInvoice).To(Equal(inv))
			})

			It("should call HandleSubscriptionInvoiceFailed if the invoice failed", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": 123455}}}`,
					`{"object": "invoice", "id": "Sv", "paid": false, "attempt_count": 2, "next_payment_attempt": 192075622}`,
				}

				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(fsub.HandleSubscriptionInvoiceFailedCalled).To(BeTrue())
				Expect(fsub.HandleSubscriptionInvoiceFailedInvoiceID).To(Equal("Xcccs"))
			})

			It("should return an error if the HandleSubscriptionInvoiceFailed return an error", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": 123455}}}`,
					`{"object": "invoice", "id": "Sv", "paid": false, "attempt_count": 2, "next_payment_attempt": 192075622}`,
				}

				fsub.HandleSubscriptionInvoiceFailedError = errors.New("Woops!")
				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				Expect(fsub.HandleSubscriptionInvoiceFailedCalled).To(BeTrue())
				Expect(fsub.HandleSubscriptionInvoiceFailedInvoiceID).To(Equal("Xcccs"))
			})

			It("should log to mixpanel if the invoice failed", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "invoice.payment_failed", "data": {"object": {"id": "Xcccs", "attempt_count": 2, "next_payment_attempt": 123455}}}`,
					`{"object": "invoice", "id": "Sv", "paid": false, "attempt_count": 2, "next_payment_attempt": 192075622}`,
				}

				inv.Attempts = 2
				fsub.HandleSubscriptionInvoiceFailedReturn = inv
				res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "invoice.payment_failed"}`)
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(fsub.HandleSubscriptionInvoiceFailedCalled).To(BeTrue())
				Expect(fsub.HandleSubscriptionInvoiceFailedInvoiceID).To(Equal("Xcccs"))

				var (
					evs []string
					ok  bool
				)
				Eventually(func() bool {
					evs, ok = ft.Events[u.ID]
					return ok
				}).Should(BeTrue())

				Expect(evs).To(Equal([]string{"Customer Card Declined 2"}))
			})

			It("should publish a EventInvoiceUpdated to the user", func() {
				hs1, err := test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:        "my-host-1",
						HourlyPrice: 2000,
					},
					U: u,
				})
				Expect(err).To(BeNil())
				h1 := hs1.H

				// Insert a subscription
				bp, err := u.BillingProfile(db)
				Expect(err).To(BeNil())

				s := &subscription.Subscription{
					PlanID:           h1.PlanID,
					BillingProfileID: bp.ID,
					State:            "active",
					StripeID:         sql.NullString{"1", true},
					Slug:             "SLUG",
				}
				err = test.WrapInTransaction(db, s.Insert)
				Expect(err).To(BeNil())

				// Update invoice's subscription_id
				inv.SubscriptionID = sql.NullInt64{s.ID, true}
				err = inv.Update(db)
				Expect(err).To(BeNil())

				// Update host's subscription_id
				err = h1.UpdateColumn(db, "subscription_id", sql.NullInt64{s.ID, true})
				Expect(err).To(BeNil())

				// Insert an invoice item
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

				evt, err := test.WaitForEvent(ch, controllers.EventInvoiceUpdated, 50*time.Millisecond)
				Expect(err).To(BeNil())

				evtData, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				inv, ok := evtData["invoice"].(*serializers.Invoice)
				Expect(ok).To(BeTrue())
				Expect(inv.ID).To(Equal(inv.ID))
				Expect(inv.Items[0].ID).To(Equal(itm1.ID))
				Expect(inv.Items[0].Description).To(ContainSubstring(h1.Slug))
			})

			Describe("with subscriptions", func() {
				var (
					pl  *plan.Plan
					sub *subscription.Subscription
				)

				BeforeEach(func() {
					pl = &plan.Plan{
						Slug:          "hs1Plan",
						Name:          "hs1Plan",
						MaxContainers: 1,
						Price:         1499,
						StripePlanID:  sql.NullString{"stripe-plan-id", true},
						Hidden:        true,
					}
					Expect(pl.Insert(db)).To(BeNil())

					bp, err := u.BillingProfile(db)
					Expect(err).To(BeNil())

					sub = &subscription.Subscription{
						StripeID:         sql.NullString{"abc1", true},
						PlanID:           pl.ID,
						BillingProfileID: bp.ID,
						Slug:             "SLUG",
					}
					Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
				})

				It("should return an error if HandleSubscriptionUpdated failed", func() {
					fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.updated", "data": {"object": {"id": "sub_1"}}}`}

					fsub.HandleSubscriptionUpdatedError = errors.New("Woops")

					res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.updated"}`)
					Expect(res.Code).To(Equal(http.StatusInternalServerError))

					Expect(fsub.HandleSubscriptionUpdatedCalled).To(BeTrue())
					Expect(fsub.HandleSubscriptionUpdatedSubID).To(Equal("sub_1"))
				})

				It("should return an error if HandleSubscriptionDeleted failed", func() {
					fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.deleted", "data": {"object": {"id": "sub_1"}}}`}

					fsub.HandleSubscriptionDeletedError = errors.New("Woops")

					res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.deleted"}`)
					Expect(res.Code).To(Equal(http.StatusInternalServerError))

					Expect(fsub.HandleSubscriptionDeletedCalled).To(BeTrue())
					Expect(fsub.HandleSubscriptionDeletedSubID).To(Equal("sub_1"))
				})

				It("should publish an event if the function succeeds", func() {
					fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.updated", "data": {"object": {"id": "sub_1"}}}`}

					fsub.HandleSubscriptionUpdatedError = nil
					fsub.HandleSubscriptionUpdatedReturn = sub

					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.updated"}`)
					Expect(res.Code).To(Equal(http.StatusOK))

					Expect(fsub.HandleSubscriptionUpdatedCalled).To(BeTrue())
					Expect(fsub.HandleSubscriptionUpdatedSubID).To(Equal("sub_1"))

					evt, err := test.WaitForEvent(ch, controllers.EventSubscriptionUpdated, 250*time.Millisecond)
					Expect(err).To(BeNil())

					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["subscription"].(*serializers.Subscription)
					Expect(ok).To(BeTrue())
					Expect(s.Slug).To(Equal(sub.Slug))
					Expect(s.HostSlug).To(BeEmpty())
					Expect(s.Plan).ToNot(BeNil())
					Expect(s.Plan.Slug).To(Equal("hs1Plan"))
				})

				It("should publish the host slug if available", func() {
					hs, err := test.CreateHostStack(db, &test.HostStack{
						H: &host.Host{
							Slug:        "my-host-1",
							HourlyPrice: 2000,
						},
						P: pl,
						U: u,
					})
					Expect(err).To(BeNil())

					_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", sub.ID, hs.H.ID)
					Expect(err).To(BeNil())

					fakeStripe.Responses = []string{`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.updated", "data": {"object": {"id": "sub_1"}}}`}

					fsub.HandleSubscriptionUpdatedError = nil
					fsub.HandleSubscriptionUpdatedReturn = sub

					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					res := doRequest(`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "type": "customer.subscription.updated"}`)
					Expect(res.Code).To(Equal(http.StatusOK))

					Expect(fsub.HandleSubscriptionUpdatedCalled).To(BeTrue())
					Expect(fsub.HandleSubscriptionUpdatedSubID).To(Equal("sub_1"))

					evt, err := test.WaitForEvent(ch, controllers.EventSubscriptionUpdated, 250*time.Millisecond)
					Expect(err).To(BeNil())

					evtData, _ := evt.Data.(map[string]interface{})
					s, ok := evtData["subscription"].(*serializers.Subscription)
					Expect(ok).To(BeTrue())
					Expect(s.Slug).To(Equal(sub.Slug))
					Expect(s.HostSlug).To(Equal("my-host-1"))
					Expect(s.Plan).ToNot(BeNil())
					Expect(s.Plan.Slug).To(Equal("hs1Plan"))
				})
			})
		})
	})

	Describe("SerializeInvoices()", func() {
		var (
			h1               *host.Host
			itm1, itm2, itm3 *invoiceitem.InvoiceItem
		)

		BeforeEach(func() {
			hs1, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h1 = hs1.H

			// Insert a subscription
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			s1 := &subscription.Subscription{
				PlanID:           h1.PlanID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, s1.Insert)
			Expect(err).To(BeNil())

			// Update invoice's subscription_id
			inv.SubscriptionID = sql.NullInt64{s1.ID, true}
			err = inv.Update(db)
			Expect(err).To(BeNil())

			// Update host's subscription_id
			err = h1.UpdateColumn(db, "subscription_id", sql.NullInt64{s1.ID, true})
			Expect(err).To(BeNil())

			// Insert invoice items
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
				HostID:      sql.NullInt64{h1.ID, true},
				Hours:       sql.NullInt64{20, true},
				HourlyPrice: sql.NullInt64{1000, true},
				Terminated:  true,
				ItemType:    invoiceitem.ItemTypeCompute,
				PlanMemory:  sql.NullString{"1 GB", true},
			}
			Expect(itm3.Insert(db)).To(BeNil())
		})

		It("should detail the correct amount of invoice items", func() {
			sInv, err := stripehooks.SerializeInvoice(db, inv)
			Expect(err).To(BeNil())
			Expect(sInv.Items).To(HaveLen(2))
			Expect(sInv.TotalCredit).To(Equal(int64(200)))
		})

		It("should provide information about the hosts", func() {
			sInv, err := stripehooks.SerializeInvoice(db, inv)
			Expect(err).To(BeNil())
			Expect(sInv.Items[0].Terminated).To(BeTrue())
			Expect(sInv.Items[1].Terminated).To(BeFalse())

			Expect(sInv.Items[0].Description).To(Equal(invoiceitem.ItemDescription(itm3, h1)))
			Expect(sInv.Items[1].Description).To(Equal(invoiceitem.ItemDescription(itm1, h1)))
		})
	})
})
