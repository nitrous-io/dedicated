package subscriptions_test

import (
	"database/sql"
	"errors"
	"net/http"
	"testing"
	"time"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "subscriptions")
}

var _ = Describe("subscriptions", func() {
	var (
		db *ndb.Database

		hs *test.HostStack
		u  *user.User
		pl *plan.Plan

		fakeStripe *test.FakeStripe

		subProvider *subscriptions.SubscriptionProvider
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		fakeStripe = &test.FakeStripe{}
		stripe.SetBackend(fakeStripe)

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		_, err = db.Exec(`UPDATE billing_profiles SET state=$1, stripe_id=$2 WHERE user_id=$3;`, billingprofile.StatePaid, "stripe-1", u.ID)
		Expect(err).To(BeNil())

		pl = &plan.Plan{
			Slug:          "hs1Plan",
			Name:          "hs1Plan",
			MaxContainers: 1,
			Price:         1499,
			StripePlanID:  sql.NullString{"stripe-plan-id", true},
		}
		Expect(pl.Insert(db)).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 2000,
			},
			P: pl,
			U: u,
		})
		Expect(err).To(BeNil())

		subProvider = &subscriptions.SubscriptionProvider{}
	})

	Describe("Subscribe()", func() {
		It("should return an error if the plan is not subscribable", func() {
			p := &plan.Plan{}
			s, err := subProvider.Subscribe(db, hs.H, p, u)
			Expect(err).To(Equal(subscriptions.ErrNotSubscribablePlan))
			Expect(s).To(BeNil())
		})

		It("should return an error if the billing profile is in an incomptable state", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateDelinquent, u.ID)
			Expect(err).To(BeNil())

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(Equal(subscriptions.ErrInvalidBillingState))
			Expect(s).To(BeNil())
		})

		It("should not create a subscription if it was unable to submit the Subscription", func() {
			fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
			fakeStripe.Responses = []string{
				`{"object": null, "id": null}`,
			}

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(Equal(fakeStripe.Errors[0]))
			Expect(s).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["method"]).To(Equal("POST"))
			Expect(r["path"]).To(Equal("/customers/stripe-1/subscriptions"))
			Expect(r["plan"]).To(Equal("stripe-plan-id"))

			_, err = findByHostID(db, hs.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should submit the subscription and create the model object", func() {
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419
				}`,
			}

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

			s, err = findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl.ID))
			Expect(s.StripeID.String).To(Equal("abc1"))
			Expect(s.State).To(Equal("active"))
			Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418419, 0)))
			Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010419, 0)))
			Expect(s.CancelPending).To(BeFalse())

			h, err := host.Find(db, u, hs.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.SubscriptionID.Int64).To(Equal(s.ID))
		})

		It("should roll back the credit invoice item if it was unable to submit the Subscription", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 2000);", u.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil, &stripe.Error{}, nil}
			fakeStripe.Responses = []string{
				`{
					"object": "invoiceitem",
					"id": "item-1"
				}`,
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419
				}`,
				`{
					"deleted": true,
					"id": "item-1"
				}`,
			}

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(Equal(fakeStripe.Errors[1]))
			Expect(s).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("POST"))
			Expect(r1["path"]).To(Equal("/invoiceitems"))
			Expect(r1["customer"]).To(Equal("stripe-1"))
			Expect(r1["amount"]).To(Equal("-1499"))
			Expect(r1["desc"]).To(Equal("Credit of $14.99"))
			r2 := fakeStripe.ReceivedRequests[1]
			Expect(r2["method"]).To(Equal("POST"))
			Expect(r2["path"]).To(Equal("/customers/stripe-1/subscriptions"))
			Expect(r2["plan"]).To(Equal("stripe-plan-id"))
			r3 := fakeStripe.ReceivedRequests[2]
			Expect(r3["method"]).To(Equal("DELETE"))
			Expect(r3["path"]).To(Equal("/invoiceitems/item-1"))

			_, err = findByHostID(db, hs.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))

			a, err := credit.AvailableCredits(db, u.ID)
			Expect(err).To(BeNil())
			Expect(a).To(Equal(int64(2000)))
		})

		It("should keep the credit invoice item if it was able to submit the Subscription", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 2000);", u.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil, nil, nil}
			fakeStripe.Responses = []string{
				`{
					"object": "invoiceitem",
					"id": "item-1"
				}`,
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419
				}`,
			}

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("POST"))
			Expect(r1["path"]).To(Equal("/invoiceitems"))
			Expect(r1["customer"]).To(Equal("stripe-1"))
			Expect(r1["amount"]).To(Equal("-1499"))
			Expect(r1["desc"]).To(Equal("Credit of $14.99"))
			r2 := fakeStripe.ReceivedRequests[1]
			Expect(r2["method"]).To(Equal("POST"))
			Expect(r2["path"]).To(Equal("/customers/stripe-1/subscriptions"))
			Expect(r2["plan"]).To(Equal("stripe-plan-id"))

			s, err = findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())

			a, err := credit.AvailableCredits(db, u.ID)
			Expect(err).To(BeNil())
			Expect(a).To(Equal(int64(501)))
		})

		It("should use the minimum credit amount if the plan is greater", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 100);", u.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil, nil}
			fakeStripe.Responses = []string{
				`{
					"object": "invoiceitem",
					"id": "item-1"
				}`,
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419
				}`,
			}

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("POST"))
			Expect(r1["path"]).To(Equal("/invoiceitems"))
			Expect(r1["customer"]).To(Equal("stripe-1"))
			Expect(r1["amount"]).To(Equal("-100"))
			Expect(r1["desc"]).To(Equal("Credit of $1.00"))
			r2 := fakeStripe.ReceivedRequests[1]
			Expect(r2["method"]).To(Equal("POST"))
			Expect(r2["path"]).To(Equal("/customers/stripe-1/subscriptions"))
			Expect(r2["plan"]).To(Equal("stripe-plan-id"))

			s, err = findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())

			a, err := credit.AvailableCredits(db, u.ID)
			Expect(err).To(BeNil())
			Expect(a).To(Equal(int64(0)))
		})

		It("should use the exempt prefix if the user is exempt", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateExempt, u.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil, nil}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419
				}`,
			}

			s, err := subProvider.Subscribe(db, hs.H, pl, u)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["method"]).To(Equal("POST"))
			Expect(r["path"]).To(Equal("/customers/stripe-1/subscriptions"))
			Expect(r["plan"]).To(Equal("stripe-plan-id-exempt"))

			s, err = findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(s).ToNot(BeNil())
		})
	})

	Describe("Cancel()", func() {
		It("should submit a request to Stripe to cancel the subscription at period end", func() {
			fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
			fakeStripe.Responses = []string{
				`{
          "cancelled": true,
				  "id": "sub1"
				}`,
			}

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{
				BillingProfileID: bp.ID,
				StripeID:         sql.NullString{"sub1", true},
			}

			err = subProvider.Cancel(db, sub)
			Expect(err).To(Equal(fakeStripe.Errors[0]))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("DELETE"))
			Expect(r1["path"]).To(Equal("/customers/stripe-1/subscriptions/sub1"))
			Expect(r1["at_period_end"]).To(Equal("true"))
		})
	})

	Describe("HandleSubscriptionUpdated()", func() {
		var sub *subscription.Subscription
		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub = &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
		})

		It("should return an error if it could not locate the subscription", func() {
			err := subProvider.HandleSubscriptionUpdated(db, "not-exist")
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should return an error if it could not communicate with Stripe", func() {
			fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419
				}`,
			}

			err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(Equal(fakeStripe.Errors[0]))
		})

		It("should update the state and period dates", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "updating",
				  "current_period_start": 1434418422,
				  "current_period_end": 1437010429
				}`,
			}

			err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(BeNil())

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl.ID))
			Expect(s.StripeID.String).To(Equal("abc1"))
			Expect(s.State).To(Equal("updating"))
			Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418422, 0)))
			Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010429, 0)))
			Expect(s.CancelPending).To(BeFalse())
		})

		It("should null out the period start & end if it is null", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "updating",
				  "current_period_start": null,
				  "current_period_end": null
				}`,
			}

			err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(BeNil())

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl.ID))
			Expect(s.StripeID.String).To(Equal("abc1"))
			Expect(s.State).To(Equal("updating"))
			Expect(s.CurrentPeriodStart.Valid).To(BeFalse())
			Expect(s.CurrentPeriodEnd.Valid).To(BeFalse())
			Expect(s.CancelPending).To(BeFalse())
		})

		It("should set the CancelPending if a Cancel is expected", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
				  "current_period_start": 1434418419,
				  "current_period_end": 1437010419,
          "canceled_at": 1437010422
				}`,
			}

			err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(BeNil())

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl.ID))
			Expect(s.StripeID.String).To(Equal("abc1"))
			Expect(s.State).To(Equal("active"))
			Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418419, 0)))
			Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010419, 0)))
			Expect(s.CancelPending).To(BeTrue())
			Expect(s.CancelledAt.Valid).To(BeFalse())
		})
	})

	Describe("HandleSubscriptionDeleted()", func() {
		var sub *subscription.Subscription

		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub = &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
		})

		It("should return an error if it could not locate the subscription", func() {
			err := subProvider.HandleSubscriptionDeleted(db, "not-exist")
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should update the state of the subscription", func() {
			err := subProvider.HandleSubscriptionDeleted(db, "abc1")
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{}
			err = db.Get(sub, "SELECT * FROM subscriptions WHERE stripe_id = 'abc1';")
			Expect(err).To(BeNil())
			Expect(sub.CancelPending).To(BeFalse())
			Expect(sub.CancelledAt.Time).To(BeTemporally("~", time.Now(), time.Second))
			Expect(sub.State).To(Equal(subscription.StateDeleted))
		})

		It("should terminate the host assigned to the subscription", func() {
			_, err := db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			prov := &user.User{
				Username: "host-provisioner",
				Password: "123456",
				Email:    "hp@example.com",
			}
			err = test.WrapInTransaction(db, prov.Insert)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE users SET service_account = true WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			e := errors.New("Woops!")
			termCalled := false
			subProvider.HostStopFunc = func(db *ndb.Database, h *host.Host, headers http.Header, ev hostcontrol.EventFunc, initiatorID int64) error {
				Expect(ev).To(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(headers.Get(constants.NitrousUUIDHeader)).To(Equal("subscription_terminated"))
				Expect(initiatorID).To(Equal(prov.ID))
				termCalled = true
				return e
			}

			err = subProvider.HandleSubscriptionDeleted(db, "abc1")
			Expect(err).To(Equal(e))
			Expect(termCalled).To(BeTrue())

			sub := &subscription.Subscription{}
			err = db.Get(sub, "SELECT * FROM subscriptions WHERE stripe_id = 'abc1';")
			Expect(err).To(BeNil())
			Expect(sub.CancelPending).To(BeFalse())
			Expect(sub.CancelledAt.Time).To(BeTemporally("~", time.Now(), time.Second))
			Expect(sub.State).To(Equal(subscription.StateDeleted))
		})
	})

	Describe("HandleSubscriptionInvoiceCreated()", func() {
		var sub *subscription.Subscription

		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub = &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
		})

		It("should return an error if an invoice already exists", func() {
			_, err := db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end, stripe_id) VALUES ($1, 'paid', now(), now(), 'inv1')", u.ID)
			Expect(err).To(BeNil())

			err = subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(Equal(subscriptions.ErrInvoiceAlreadyCreated))
		})

		It("should return an error if it cannot fetch the invoice", func() {
			fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
			fakeStripe.Responses = []string{
				`{
				  "id": "invoice-1",
				  "object": "invoice",
				}`,
			}
			err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(Equal(fakeStripe.Errors[0]))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("GET"))
			Expect(r1["path"]).To(Equal("/invoices/inv1"))
		})

		It("should create the invoice with invoice items", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempts": 2,
          "stripe_id": "str-invoice-1",
          "paid": true,
          "lines": {
            "data": [
              {
                "id": "stripe-item-1",
                "object": "line_item",
                "type": "subscription",
                "livemode": true,
                "amount": 1499,
                "currency": "usd",
                "proration": false,
                "period": {
                  "start": 1434418419,
                  "end": 1437010419
                },
                "plan": {
									"interval": "month",
									"name": "ABC1",
									"created": 1434516288,
									"amount": 1499,
									"currency": "usd",
									"id": "abc1",
									"object": "plan",
									"livemode": false,
									"interval_count": 1,
									"trial_period_days": null
								}
							}
            ],
            "total_count": 1,
            "object": "list",
            "url": "/v1/invoices/inv1/lines"
          }
				}`,
			}
			err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("GET"))
			Expect(r1["path"]).To(Equal("/invoices/inv1"))

			inv, err := invoice.FindByStripeID(db, "inv1")
			Expect(err).To(BeNil())
			Expect(inv.UserID).To(Equal(u.ID))
			Expect(inv.SubscriptionID.Int64).To(Equal(sub.ID))
			Expect(inv.TotalCharge).To(Equal(int64(1499)))

			items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(1))
			Expect(items[0].ItemType).To(Equal(invoiceitem.ItemTypeSubscription))
			Expect(items[0].StripeID.String).To(Equal("stripe-item-1"))
			Expect(items[0].TotalStripeCharge.Int64).To(Equal(int64(1499)))
			Expect(items[0].Description.String).To(Equal("Subscription to ABC1 ($14.99/month)"))
		})

		It("should create a credit invoice item if there are negative Stripe items", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempts": 2,
          "stripe_id": "str-invoice-1",
          "paid": true,
          "lines": {
            "data": [
              {
                "id": "stripe-item-1",
                "object": "line_item",
                "type": "subscription",
                "livemode": true,
                "amount": 1499,
                "currency": "usd",
                "proration": false,
                "period": {
                  "start": 1434418419,
                  "end": 1437010419
                }
              },
              {
                "id": "stripe-item-2",
                "object": "line_item",
                "type": "subscription",
                "livemode": true,
                "amount": -499,
                "currency": "usd",
                "proration": false,
                "period": {
                  "start": 1434418419,
                  "end": 1437010419
                }
              }
            ],
            "total_count": 1,
            "object": "list",
            "url": "/v1/invoices/inv1/lines"
          }
				}`,
			}
			err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())

			inv, err := invoice.FindByStripeID(db, "inv1")
			Expect(err).To(BeNil())

			items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(2))
			Expect(items[0].ItemType).To(Equal(invoiceitem.ItemTypeSubscription))
			Expect(items[0].StripeID.String).To(Equal("stripe-item-1"))
			Expect(items[0].TotalStripeCharge.Int64).To(Equal(int64(1499)))

			Expect(items[1].ItemType).To(Equal(invoiceitem.ItemTypeCredit))
			Expect(items[1].StripeID.String).To(Equal("stripe-item-2"))
			Expect(items[1].TotalStripeCharge.Int64).To(Equal(int64(-499)))
			Expect(items[1].Credit.Int64).To(Equal(int64(499)))
		})

		It("should create a credit for the pending invoice if it has not been paid", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 100);", u.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil, nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempts": 2,
          "stripe_id": "str-invoice-1",
          "paid": false,
          "lines": {
            "data": [
              {
                "id": "stripe-item-1",
                "object": "line_item",
                "type": "subscription",
                "livemode": true,
                "amount": 1499,
                "currency": "usd",
                "proration": false,
                "period": {
                  "start": 1434418419,
                  "end": 1437010419
                }
              }
            ],
            "total_count": 1,
            "object": "list",
            "url": "/v1/invoices/inv1/lines"
          }
				}`,
				`{
          "id": "stripe-item-2",
          "object": "line_item",
          "amount": -100
        }`,
			}
			err = subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
			r := fakeStripe.ReceivedRequests[1]
			Expect(r["method"]).To(Equal("POST"))
			Expect(r["path"]).To(Equal("/invoiceitems"))
			Expect(r["customer"]).To(Equal("stripe-1"))
			Expect(r["amount"]).To(Equal("-100"))

			inv, err := invoice.FindByStripeID(db, "inv1")
			Expect(err).To(BeNil())
			Expect(inv.TotalCharge).To(Equal(int64(1399)))

			items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(2))

			Expect(items[1].ItemType).To(Equal(invoiceitem.ItemTypeCredit))
			Expect(items[1].StripeID.String).To(Equal("stripe-item-2"))
			Expect(items[1].TotalStripeCharge.Int64).To(Equal(int64(-100)))
			Expect(items[1].Credit.Int64).To(Equal(int64(100)))
		})
	})
})

func findByID(d ndb.QueryReceiver, id int64) (*subscription.Subscription, error) {
	sub := &subscription.Subscription{}
	err := d.Get(sub, "SELECT subscriptions.* FROM subscriptions WHERE id = $1;", id)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func findByHostID(d ndb.QueryReceiver, id int64) (*subscription.Subscription, error) {
	sub := &subscription.Subscription{}
	err := d.Get(sub, "SELECT subscriptions.* FROM subscriptions INNER JOIN hosts ON subscriptions.id = hosts.subscription_id WHERE hosts.id = $1;", id)
	if err != nil {
		return nil, err
	}
	return sub, nil
}
