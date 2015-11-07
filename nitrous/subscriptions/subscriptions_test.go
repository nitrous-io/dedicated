package subscriptions_test

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/coupon"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/segment"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stripe/stripe-go/event"
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
		bp *billingprofile.BillingProfile
		pl *plan.Plan

		fakeMailer *test.FakeMailer
		fakeStripe *test.FakeStripe

		subProvider      *subscriptions.SubscriptionProvider
		stripeCustomerID string
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

		stripeCustomerID = "cus-1"
		_, err = db.Exec(`UPDATE billing_profiles SET state=$1, stripe_id=$2 WHERE user_id=$3;`, billingprofile.StatePaid, stripeCustomerID, u.ID)
		Expect(err).To(BeNil())

		bp, err = u.BillingProfile(db)
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

		_, err = db.Exec("UPDATE hosts SET idle_notices_sent = $1, idle_next_check = NULL WHERE id = $2", host.IDLE_NOTICES_FIRST_WARNING, hs.H.ID)
		Expect(err).To(BeNil())

		fakeMailer = &test.FakeMailer{}
		subProvider = &subscriptions.SubscriptionProvider{
			Mailer: fakeMailer,
		}
	})

	AfterEach(func() {
		db.Close()
	})

	Describe("Subscribe()", func() {
		var (
			o *org.Org
		)

		BeforeEach(func() {
			o = nil
		})

		sharedExamples := func() {
			It("should return an error if the plan is not subscribable", func() {
				p := &plan.Plan{}
				s, err := subProvider.Subscribe(db, hs.H, p, u, o)
				Expect(err).To(Equal(subscriptions.ErrNotSubscribablePlan))
				Expect(s).To(BeNil())
			})

			It("should return an error if the billing profile is ineligible", func() {
				bp.State = billingprofile.StateDelinquent
				Expect(bp.Update(db)).To(BeNil())

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(Equal(subscriptions.ErrInvalidBillingState))
				Expect(s).To(BeNil())
			})

			It("should not create a subscription if it was unable to submit the Subscription", func() {
				fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
				fakeStripe.Responses = []string{
					`{"object": null, "id": null}`,
				}

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(Equal(fakeStripe.Errors[0]))
				Expect(s).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST"))
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
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

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

				s, err = findByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(s.PlanID).To(Equal(pl.ID))
				Expect(s.StripeID.String).To(Equal("abc1"))
				Expect(s.State).To(Equal("active"))
				Expect(s.Slug).To(HaveLen(10))
				Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418419, 0)))
				Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010419, 0)))
				Expect(s.CancelPending).To(BeFalse())

				h, err := host.Find(db, u, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.SubscriptionID.Int64).To(Equal(s.ID))
				Expect(h.IdleNoticesSent).To(Equal(host.IDLE_NOTICES_NO_WARNING))
				Expect(h.IdleNextCheck.Valid).To(BeFalse())
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

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(Equal(fakeStripe.Errors[1]))
				Expect(s).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
				r1 := fakeStripe.ReceivedRequests[0]
				Expect(r1["method"]).To(Equal("POST"))
				Expect(r1["path"]).To(Equal("/invoiceitems"))
				Expect(r1["customer"]).To(Equal(stripeCustomerID))
				Expect(r1["amount"]).To(Equal("-1499"))
				Expect(r1["desc"]).To(Equal("Credit of $14.99"))
				r2 := fakeStripe.ReceivedRequests[1]
				Expect(r2["method"]).To(Equal("POST"))
				Expect(r2["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
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

			It("should handle card expired", func() {
				fakeStripe.Errors = []*stripe.Error{&stripe.Error{Code: stripe.ExpiredCard}}
				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				}

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(Equal(subscriptions.ErrCardExpired))
				Expect(s).To(BeNil())

				_, err = findByHostID(db, hs.H.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
			})

			It("should handle card declined", func() {
				fakeStripe.Errors = []*stripe.Error{&stripe.Error{Code: stripe.CardDeclined}}
				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				}

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(Equal(subscriptions.ErrCardDeclined))
				Expect(s).To(BeNil())

				_, err = findByHostID(db, hs.H.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
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

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
				r1 := fakeStripe.ReceivedRequests[0]
				Expect(r1["method"]).To(Equal("POST"))
				Expect(r1["path"]).To(Equal("/invoiceitems"))
				Expect(r1["customer"]).To(Equal(stripeCustomerID))
				Expect(r1["amount"]).To(Equal("-1499"))
				Expect(r1["desc"]).To(Equal("Credit of $14.99"))
				r2 := fakeStripe.ReceivedRequests[1]
				Expect(r2["method"]).To(Equal("POST"))
				Expect(r2["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
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

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
				r1 := fakeStripe.ReceivedRequests[0]
				Expect(r1["method"]).To(Equal("POST"))
				Expect(r1["path"]).To(Equal("/invoiceitems"))
				Expect(r1["customer"]).To(Equal(stripeCustomerID))
				Expect(r1["amount"]).To(Equal("-100"))
				Expect(r1["desc"]).To(Equal("Credit of $1.00"))
				r2 := fakeStripe.ReceivedRequests[1]
				Expect(r2["method"]).To(Equal("POST"))
				Expect(r2["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
				Expect(r2["plan"]).To(Equal("stripe-plan-id"))

				s, err = findByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				a, err := credit.AvailableCredits(db, u.ID)
				Expect(err).To(BeNil())
				Expect(a).To(Equal(int64(0)))
			})

			It("uses a Stripe plan ID with a '-exempt' suffix if the user is exempt", func() {
				bp.State = billingprofile.StateExempt
				Expect(bp.Update(db)).To(BeNil())

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

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST"))
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
				Expect(r["plan"]).To(Equal("stripe-plan-id-exempt"))

				s, err = findByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())
			})

			It("sends user's coupon to Stripe, then marks it as used", func() {
				cp := &coupon.Coupon{
					UserID: u.ID,
					Code:   "test-code",
				}
				Expect(cp.Insert(db)).To(BeNil())

				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				}

				subscriptions.SetCouponGetFn(func(code string, params *stripe.CouponParams) (*stripe.Coupon, error) {
					if code == cp.Code {
						return &stripe.Coupon{}, nil
					}
					return nil, fmt.Errorf("Wrong code")
				})

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST"))
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
				Expect(r["coupon"]).To(Equal("test-code"))

				// Coupon should be used.
				_, err = coupon.Find(db, u.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
				cp2 := &coupon.Coupon{}
				err = db.Get(cp2, `SELECT * FROM coupons WHERE id = $1;`, cp.ID)
				Expect(err).To(BeNil())
				Expect(cp2.UsedAt.Valid).To(BeTrue())
				Expect(cp2.UsedAt.Time).ToNot(BeZero())
			})

			It("does not send invalid coupon to Stripe nor mark it as used", func() {
				cp := &coupon.Coupon{
					UserID: u.ID,
					Code:   "invalid-code",
				}
				Expect(cp.Insert(db)).To(BeNil())

				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				}

				subscriptions.SetCouponGetFn(func(code string, params *stripe.CouponParams) (*stripe.Coupon, error) {
					return nil, fmt.Errorf("Nope, try again")
				})

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["coupon"]).To(BeEmpty())

				// Coupon should not be used.
				cp2 := &coupon.Coupon{}
				err = db.Get(cp2, `SELECT * FROM coupons WHERE id = $1;`, cp.ID)
				Expect(err).To(BeNil())
				Expect(cp2.UsedAt.Valid).To(BeFalse())
			})

			It("does not send coupon to Stripe for free plans", func() {
				pl.Price = 0

				cp := &coupon.Coupon{
					UserID: u.ID,
					Code:   "test-code",
				}
				Expect(cp.Insert(db)).To(BeNil())

				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419,
					"plan": { "id": "stripe-plan-id" }
				}`,
				}

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST"))
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
				_, ok := r["coupon"]
				Expect(ok).To(BeFalse())

				// Coupon should be unused.
				cp2, err := coupon.Find(db, u.ID)
				Expect(err).To(BeNil())
				Expect(cp2.UsedAt.Valid).To(BeFalse())
				Expect(cp2.UsedAt.Time).To(BeZero())
			})

			It("sends correct trial end param to Stripe for plans with free trial and updates trial_eligible to false", func() {
				pl.TrialPeriodDays = 3

				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419,
					"trial_end": 1441234567,
					"plan": { "id": "stripe-plan-id" }
				}`,
				}

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST"))
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))
				trialEndStr, ok := r["trial_end"]
				Expect(ok).To(BeTrue())
				trialEnd, err := strconv.Atoi(trialEndStr)
				Expect(err).NotTo(HaveOccurred())
				Expect(trialEnd).To(BeNumerically("~", time.Now().Add(72*time.Hour).Unix(), 10)) // 10 seconds within current time

				s, err = findByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(s.TrialEnd.Time).To(BeTemporally("==", time.Unix(1441234567, 0)))

				updatedBp, err := billingprofile.FindByID(db, bp.ID)
				Expect(err).To(BeNil())

				Expect(updatedBp.TrialEligible).To(BeFalse())
			})

			It("should not send trial end param to Stripe if subscribing to a plan without free trial and should update trial_eligible to false", func() {
				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419,
					"trial_end": null,
					"plan": { "id": "stripe-plan-id" }
				}`,
				}

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST"))
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions", stripeCustomerID)))

				s, err = findByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(s.TrialEnd.Valid).To(BeFalse())

				latestBp, err := billingprofile.FindByID(db, bp.ID)
				Expect(err).To(BeNil())

				Expect(latestBp.TrialEligible).To(BeFalse())
			})
		}

		Context("as a user", func() {
			sharedExamples()

			It("updates user subscribing to free plan with new segment", func() {
				fakeStripe.Responses = []string{
					`{
					"object": "subscription",
					"id": "abc1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				}

				freeSID, err := segment.FreeUserDefaultID(db)
				Expect(err).To(BeNil())

				defaultSID, err := segment.UserDefaultID(db)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE users SET segment_id = $1 WHERE id = $2",
					freeSID, u.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE plans SET price = 0, base_price = 0 WHERE id = $1", pl.ID)
				Expect(err).To(BeNil())
				pl, err = plan.FindByID(db, pl.ID)
				Expect(err).To(BeNil())

				s, err := subProvider.Subscribe(db, hs.H, pl, u, o)
				Expect(err).To(BeNil())
				Expect(s).ToNot(BeNil())

				u2, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u2.SegmentID).To(Equal(defaultSID))
			})
		})

		Context("as a organisation", func() {
			BeforeEach(func() {
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(t *sqlx.Tx) error {
					Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
					return nil
				})
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).To(BeNil())

				// corrupt the user's billing profile so we ensure the operation runs over the org BP
				Expect(bp.OrgID.Valid).To(BeFalse())
				bp.State = billingprofile.StateNoCard
				bp.StripeID = sql.NullString{stripeCustomerID + "-disabled", true}
				Expect(bp.Update(db)).To(BeNil())

				bp, err = billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())
				bp.State = billingprofile.StatePaid
				bp.StripeID = sql.NullString{stripeCustomerID, true}
				Expect(bp.Update(db)).To(BeNil())
			})

			sharedExamples()
		})
	})

	Describe("Cancel()", func() {
		It("submits a request to Stripe to cancel the subscription at period end", func() {
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
				PlanID:           pl.ID,
				Slug:             "SLUG",
			}

			err = subProvider.Cancel(db, sub)
			Expect(err).To(Equal(fakeStripe.Errors[0]))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("DELETE"))
			Expect(r1["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/sub1", stripeCustomerID)))
			Expect(r1["at_period_end"]).To(Equal("true"))
		})

		It("should cancel immediately if the plan is free", func() {
			_, err := db.Exec("UPDATE plans SET price = 0, base_price = 0 WHERE id = $1;", pl.ID)
			Expect(err).To(BeNil())
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
				PlanID:           pl.ID,
				Slug:             "SLUG",
			}

			err = subProvider.Cancel(db, sub)
			Expect(err).To(Equal(fakeStripe.Errors[0]))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("DELETE"))
			Expect(r1["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/sub1", stripeCustomerID)))
			Expect(r1["at_period_end"]).To(BeEmpty())

			// and update user segment back to free
			sid, err := segment.FreeUserDefaultID(db)
			Expect(err).To(BeNil())
			Expect(u.SegmentID).NotTo(Equal(sid))
			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.SegmentID).To(Equal(sid))
		})

		It("should cancel immediately if on free trial", func() {
			_, err := db.Exec("UPDATE plans SET price = 123, base_price = 456 WHERE id = $1;", pl.ID)
			Expect(err).To(BeNil())

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
				PlanID:           pl.ID,
				Slug:             "SLUG",
				TrialEnd:         pq.NullTime{time.Now().Add(24 * time.Hour), true},
			}

			err = subProvider.Cancel(db, sub)
			Expect(err).To(Equal(fakeStripe.Errors[0]))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("DELETE"))
			Expect(r1["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/sub1", stripeCustomerID)))
			Expect(r1["at_period_end"]).To(BeEmpty())
		})
	})

	Describe("Reactivate()", func() {
		var sub *subscription.Subscription

		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub = &subscription.Subscription{
				BillingProfileID: bp.ID,
				StripeID:         sql.NullString{"sub1", true},
				PlanID:           pl.ID,
				Slug:             "SLUG",
				CancelPending:    true,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
			Expect(err).To(BeNil())
		})

		It("submits a request to Stripe to update the subscription to set cancel_at_period_end=false", func() {
			fakeStripe.Responses = []string{
				`{
					"cancel_at_period_end": false,
					"id": "sub1"
				}`,
			}

			err := subProvider.Reactivate(db, sub)
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("POST"))
			Expect(r1["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/sub1", stripeCustomerID)))
		})

		It("should return error if cancel_at_period_end is not set to false after updating", func() {
			fakeStripe.Responses = []string{
				`{
					"cancel_at_period_end": true,
					"id": "sub1"
				}`,
			}

			err := subProvider.Reactivate(db, sub)
			Expect(err).To(Equal(fmt.Errorf("An error occured when re-activating Stripe subscription %d. Expected Stripe response cancel_at_period_end=false, got cancel_at_period_end=true.", sub.ID)))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("POST"))
			Expect(r1["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/sub1", stripeCustomerID)))
		})

		It("should return error from stripe if customer's subscription does not exist", func() {
			fakeStripe.Errors = []*stripe.Error{
				&stripe.Error{
					Type:  "invalid_request_error",
					Msg:   fmt.Sprintf("Customer %v does not have a subscription with ID sub1", stripeCustomerID),
					Param: "subscription",
				},
			}

			err := subProvider.Reactivate(db, sub)
			Expect(err).To(Equal(fakeStripe.Errors[0]))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("POST"))
			Expect(r1["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/sub1", stripeCustomerID)))
		})
	})

	Describe("Upgrade()", func() {
		var (
			sub     *subscription.Subscription
			newPlan *plan.Plan
		)

		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			// Create an existing subscription for host.
			sub = &subscription.Subscription{
				StripeID:         sql.NullString{"stripe-sub-1", true},
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				Slug:             "SLUG",
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			newPlan = &plan.Plan{
				Slug:          "new-plan",
				MaxContainers: 2,
				Price:         2499,
				StripePlanID:  sql.NullString{"stripe-plan-2", true},
			}
			Expect(newPlan.Insert(db)).To(BeNil())

			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "stripe-sub-1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				`{
					"object": "invoice",
					"id": "invoice-1",
					"status": "active",
					"amount_due": 2099
				}`,
				`{
					"object": "invoice",
					"id": "invoice-1",
					"paid": true
				}`,
			}
		})

		It("returns an error if the user's billing profile is ineligible", func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())
			bp.State = billingprofile.StateDelinquent
			Expect(bp.Update(db)).To(BeNil())

			err = subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(Equal(subscriptions.ErrInvalidBillingState))
		})

		It("returns an error if plan cannot be upgraded to", func() {
			newPlan.Price = 0

			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(Equal(subscriptions.ErrCannotUpgrade))
		})

		It("returns an error if new plan is not of the same type", func() {
			newPlan.Org = true

			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(Equal(subscriptions.ErrCannotUpgrade))
		})

		It("submits a request to Stripe to update subscription", func() {
			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["method"]).To(Equal("POST")) // https://stripe.com/docs/api/curl#update_subscription
			Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/%s", stripeCustomerID, sub.StripeID.String)))
			Expect(r["plan"]).To(Equal(newPlan.StripePlanID.String))
			_, ok := r["coupon"]
			Expect(ok).To(BeFalse())
			r2 := fakeStripe.ReceivedRequests[1]
			Expect(r2["method"]).To(Equal("POST"))
			Expect(r2["path"]).To(Equal("/invoices"))
			Expect(r2["customer"]).To(Equal(stripeCustomerID))
			r3 := fakeStripe.ReceivedRequests[2]
			Expect(r3["method"]).To(Equal("POST"))
			Expect(r3["path"]).To(Equal("/invoices/invoice-1/pay"))
		})

		Context("comping hosting mode", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE plans SET hosting_mode_plan_id = $1 WHERE id=$2;`, newPlan.ID, pl.ID)
				Expect(err).To(BeNil())
			})

			It("should add a coupon for comped hosting mode if required", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET comp_hosting_mode_before = now() WHERE user_id=$1;`, u.ID)
				Expect(err).To(BeNil())

				err = subProvider.Upgrade(db, sub, newPlan, u)
				Expect(err).To(BeNil())

				// only one request since we don't create invoice or pay it
				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST")) // https://stripe.com/docs/api/curl#update_subscription
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/%s", stripeCustomerID, sub.StripeID.String)))
				Expect(r["plan"]).To(Equal(newPlan.StripePlanID.String))
				Expect(r["coupon"]).To(Equal("comped-hosting-mode"))
			})

			It("should not add a coupon for comped hosting mode if prior to subscription creation", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET comp_hosting_mode_before  ='2014-01-01' WHERE user_id=$1;`, u.ID)
				Expect(err).To(BeNil())

				err = subProvider.Upgrade(db, sub, newPlan, u)
				Expect(err).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("POST")) // https://stripe.com/docs/api/curl#update_subscription
				Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/%s", stripeCustomerID, sub.StripeID.String)))
				Expect(r["plan"]).To(Equal(newPlan.StripePlanID.String))
				_, ok := r["coupon"]
				Expect(ok).To(BeFalse())
			})
		})

		It("should handle exempt plan subscriptions", func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())
			bp.State = billingprofile.StateExempt
			Expect(bp.Update(db)).To(BeNil())

			err = subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["method"]).To(Equal("POST")) // https://stripe.com/docs/api/curl#update_subscription
			Expect(r["path"]).To(Equal(fmt.Sprintf("/customers/%s/subscriptions/%s", stripeCustomerID, sub.StripeID.String)))
			Expect(r["plan"]).To(Equal(newPlan.StripePlanID.String + "-exempt"))
		})

		It("should handle if it was unable to pay the invoice", func() {
			fakeStripe.Errors = []*stripe.Error{nil, nil, test.StripeCardDeclined}
			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(Equal(subscriptions.ErrCardDeclined))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(3))
			h, err := findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(h.PlanID).ToNot(Equal(newPlan.ID))

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).ToNot(Equal(newPlan.ID))
		})

		It("should should not pay the invoice if it is under 100 cents", func() {
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "stripe-sub-1",
					"status": "active",
					"current_period_start": 1434418419,
					"current_period_end": 1437010419
				}`,
				`{
					"object": "invoice",
					"id": "invoice-1",
					"status": "active",
					"amount_due": 20
				}`,
				`{
					"object": "invoice",
					"id": "invoice-1",
					"paid": true
				}`,
			}
			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
		})

		It("create the subscription even where there is no oustanding invoice", func() {
			fakeStripe.Errors = []*stripe.Error{
				nil,
				&stripe.Error{
					Type: stripe.ErrorType(stripe.InvalidRequest),
					Msg:  "Nothing to invoice for customer",
				},
			}

			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(BeNil())
			Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))

			s, err := findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(newPlan.ID))
			Expect(s.State).To(Equal("active"))
			Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418419, 0)))
			Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010419, 0)))
		})

		It("updates the subscription in the database", func() {
			err := subProvider.Upgrade(db, sub, newPlan, u)
			Expect(err).To(BeNil())

			s, err := findByHostID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(newPlan.ID))
			Expect(s.State).To(Equal("active"))
			Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418419, 0)))
			Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010419, 0)))
		})

		It("should update the subscription's host with new plan details", func() {
			_, err := db.Exec("UPDATE hosts SET free = TRUE, auto_shutdown = TRUE, max_containers = 1,  plan_id = $1, subscription_id = $2 WHERE id = $3;", sub.PlanID, sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			pl2 := &plan.Plan{
				Slug:          "newStripePlan",
				Name:          "newStripePlan",
				MaxContainers: 19,
				AutoShutdown:  false,
				Price:         1899,
				StripePlanID:  sql.NullString{"newPlan", true},
			}
			Expect(pl2.Insert(db)).To(BeNil())

			pt2 := &providertemplate.ProviderTemplate{
				Name:     "newPlan",
				Disabled: false,
				PlanID:   pl2.ID,
				RegionID: hs.PT.RegionID,
				Provider: hs.PT.Provider,
			}
			Expect(pt2.Insert(db)).To(BeNil())

			err = subProvider.Upgrade(db, sub, pl2, u)
			Expect(err).To(BeNil())

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl2.ID))

			h, err := host.FindByIdentifier(db, hs.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.PlanID).To(Equal(pl2.ID))
			Expect(h.MaxContainers.Int64).To(Equal(int64(19)))
			Expect(h.AutoShutdown).To(BeFalse())
			Expect(h.Size.String).To(Equal("newStripePlan"))
			Expect(h.Free).To(BeFalse())
		})

		It("should create a respec entry for the host", func() {
			_, err := db.Exec("UPDATE hosts SET free = TRUE, auto_shutdown = TRUE, max_containers = 1, plan_id = $1, subscription_id = $2 WHERE id = $3;", sub.PlanID, sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			pl2 := &plan.Plan{
				Slug:          "newStripePlan",
				Name:          "newStripePlan",
				MaxContainers: 19,
				AutoShutdown:  false,
				Price:         1899,
				StripePlanID:  sql.NullString{"newPlan", true},
			}
			Expect(pl2.Insert(db)).To(BeNil())

			pt2 := &providertemplate.ProviderTemplate{
				Name:     "newPlan",
				Disabled: false,
				PlanID:   pl2.ID,
				RegionID: hs.PT.RegionID,
				Provider: hs.PT.Provider,
			}
			Expect(pt2.Insert(db)).To(BeNil())

			err = subProvider.Upgrade(db, sub, pl2, u)
			Expect(err).To(BeNil())

			rs, err := respec.Find(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(rs.State).To(Equal(respec.StatePending))
			Expect(rs.ProviderTemplateID).To(Equal(pt2.ID))
		})

		It("creates a respec to a provider template for the same region that satisfies the required plan and has the same hotswap value, even if it is disabled", func() {
			pl2 := &plan.Plan{
				Slug:          "newStripePlan",
				Name:          "newStripePlan",
				MaxContainers: 19,
				AutoShutdown:  false,
				Price:         1899,
				StripePlanID:  sql.NullString{"newPlan", true},
			}
			Expect(pl2.Insert(db)).To(BeNil())

			pt2 := &providertemplate.ProviderTemplate{
				Name:     "newPlan",
				Disabled: true,
				PlanID:   pl2.ID,
				RegionID: hs.PT.RegionID,
				Provider: hs.PT.Provider,
				Hotswap:  hs.PT.Hotswap,
			}
			Expect(pt2.Insert(db)).To(BeNil())

			err := subProvider.Upgrade(db, sub, pl2, u)
			Expect(err).To(BeNil())

			rs, err := respec.Find(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(rs.State).To(Equal(respec.StatePending))
			Expect(rs.ProviderTemplateID).To(Equal(pt2.ID))
		})

		It("updates host to plan specs even if unable to respec due to missing provider template", func() {
			_, err := db.Exec("UPDATE hosts SET free = TRUE, auto_shutdown = TRUE, max_containers = 1, plan_id = $1, subscription_id = $2 WHERE id = $3;", sub.PlanID, sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			pl2 := &plan.Plan{
				Slug:          "newStripePlan",
				Name:          "newStripePlan",
				MaxContainers: 19,
				AutoShutdown:  false,
				Price:         1899,
				StripePlanID:  sql.NullString{"newPlan", true},
			}
			Expect(pl2.Insert(db)).To(BeNil())

			// note: no provider template pointing to pl2

			err = subProvider.Upgrade(db, sub, pl2, u)
			Expect(err).To(BeNil())

			_, err = respec.Find(db, hs.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))

			h, err := host.FindByIdentifier(db, hs.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.MaxContainers.Int64).To(Equal(int64(19)))
			Expect(h.AutoShutdown).To(BeFalse())
			Expect(h.Size.String).To(Equal("newStripePlan"))
		})
	})

	Describe("SubscribeOrgPlan()", func() {
		var (
			o *org.Org
		)

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())
			err = db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())
			err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
			Expect(err).To(BeNil())

			// corrupt the user's billing profile so we ensure the operation runs over the org BP
			Expect(bp.OrgID.Valid).To(BeFalse())
			bp.State = billingprofile.StateNoCard
			bp.StripeID = sql.NullString{stripeCustomerID + "-disabled", true}
			Expect(bp.Update(db)).To(BeNil())

			bp, err = billingprofile.FindForOrg(db, o.ID)
			Expect(err).To(BeNil())
			bp.State = billingprofile.StatePaid
			bp.StripeID = sql.NullString{stripeCustomerID, true}
			Expect(bp.Update(db)).To(BeNil())
		})

		It("should return an error if the plan is not subscribable", func() {
			p := &plan.Plan{}
			s, err := subProvider.SubscribeOrgPlan(db, p, o)
			Expect(err).To(Equal(subscriptions.ErrNotSubscribablePlan))
			Expect(s).To(BeNil())
		})

		It("should return an error if the billing profile is ineligible", func() {
			bp.State = billingprofile.StateDelinquent
			Expect(bp.Update(db)).To(BeNil())

			s, err := subProvider.SubscribeOrgPlan(db, pl, o)
			Expect(err).To(Equal(subscriptions.ErrInvalidBillingState))
			Expect(s).To(BeNil())
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
				Slug:             "SLUG",
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
		})

		It("should return an error if it could not locate the subscription", func() {
			sub, err := subProvider.HandleSubscriptionUpdated(db, "not-exist")
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(sub).To(BeNil())
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

			sub, err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(Equal(fakeStripe.Errors[0]))
			Expect(sub).To(BeNil())
		})

		It("should update the state and period/trial dates", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "updating",
					"current_period_start": 1434418422,
					"current_period_end": 1437010429,
					"trial_end": 1441234567,
					"plan": { "id": "stripe-plan-id" }
				}`,
			}

			sub, err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(BeNil())

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl.ID))
			Expect(s.StripeID.String).To(Equal("abc1"))
			Expect(s.State).To(Equal("updating"))
			Expect(s.CurrentPeriodStart.Time).To(BeTemporally("==", time.Unix(1434418422, 0)))
			Expect(s.CurrentPeriodEnd.Time).To(BeTemporally("==", time.Unix(1437010429, 0)))
			Expect(s.TrialEnd.Time).To(BeTemporally("==", time.Unix(1441234567, 0)))
			Expect(s.CancelPending).To(BeFalse())
			Expect(sub).To(Equal(s))
		})

		It("should null out the period/trial start & end if it is null", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"object": "subscription",
					"id": "abc1",
					"status": "updating",
					"current_period_start": null,
					"current_period_end": null,
					"trial_end": null,
					"plan": { "id": "stripe-plan-id" }
				}`,
			}

			sub, err := subProvider.HandleSubscriptionUpdated(db, "abc1")
			Expect(err).To(BeNil())

			s, err := findByID(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(s.PlanID).To(Equal(pl.ID))
			Expect(s.StripeID.String).To(Equal("abc1"))
			Expect(s.State).To(Equal("updating"))
			Expect(s.CurrentPeriodStart.Valid).To(BeFalse())
			Expect(s.CurrentPeriodEnd.Valid).To(BeFalse())
			Expect(s.TrialEnd.Valid).To(BeFalse())
			Expect(s.CancelPending).To(BeFalse())
			Expect(sub).To(Equal(s))
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
					"canceled_at": 1437010422,
					"plan": { "id": "stripe-plan-id" }
				}`,
			}

			sub, err := subProvider.HandleSubscriptionUpdated(db, "abc1")
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
			Expect(sub).To(Equal(s))
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
				Slug:             "SLUG",
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
		})

		It("should return an error if it could not locate the subscription", func() {
			sub, err := subProvider.HandleSubscriptionDeleted(db, "not-exist")
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(sub).To(BeNil())
		})

		It("should update the state of the subscription", func() {
			s, err := subProvider.HandleSubscriptionDeleted(db, "abc1")
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{}
			err = db.Get(sub, "SELECT * FROM subscriptions WHERE stripe_id = 'abc1';")
			Expect(err).To(BeNil())
			Expect(sub.CancelPending).To(BeFalse())
			Expect(sub.CancelledAt.Time).To(BeTemporally("~", time.Now(), time.Second))
			Expect(sub.State).To(Equal(subscription.StateDeleted))
			Expect(sub).To(Equal(s))
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
				Expect(headers.Get(constants.NitrousUUIDHeader)).To(Equal(fmt.Sprintf("subscription_terminated_%d", sub.ID)))
				Expect(initiatorID).To(Equal(prov.ID))
				termCalled = true
				return e
			}

			s, err := subProvider.HandleSubscriptionDeleted(db, "abc1")
			Expect(err).To(BeNil())
			Expect(termCalled).To(BeTrue())

			sub := &subscription.Subscription{}
			err = db.Get(sub, "SELECT * FROM subscriptions WHERE stripe_id = 'abc1';")
			Expect(err).To(BeNil())
			Expect(sub.CancelPending).To(BeFalse())
			Expect(sub.CancelledAt.Time).To(BeTemporally("~", time.Now(), time.Second))
			Expect(sub.State).To(Equal(subscription.StateDeleted))
			Expect(sub).To(Equal(s))
		})
	})

	Describe("HandleChargeSucceeded()", func() {
		var stripeID sql.NullString

		BeforeEach(func() {
			stripeID = sql.NullString{"cus_123", true}
			_, err := db.Exec(`UPDATE billing_profiles SET stripe_id=$1 WHERE user_id=$2;`, stripeID, u.ID)
			Expect(err).To(BeNil())
		})

		It("resets failed_attempts counter to zero", func() {
			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
			}

			e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE billing_profiles SET failed_attempts=1 WHERE stripe_id=$1;`, stripeID)
			Expect(err).To(BeNil())
			var attempts int
			err = db.Get(&attempts, `SELECT failed_attempts FROM billing_profiles WHERE stripe_id=$1;`, stripeID)
			Expect(err).To(BeNil())
			Expect(attempts).To(Equal(1))

			err = subProvider.HandleChargeSucceeded(db, e)
			Expect(err).To(BeNil())

			err = db.Get(&attempts, `SELECT failed_attempts FROM billing_profiles WHERE stripe_id=$1;`, stripeID)
			Expect(err).To(BeNil())
			Expect(attempts).To(Equal(0))
		})

		It("returns error if customer key does not exist in Stripe response", func() {
			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"id":"abc"}}}}`,
			}

			e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
			Expect(err).To(BeNil())

			err = subProvider.HandleChargeSucceeded(db, e)
			Expect(err.Error()).To(ContainSubstring("Stripe failed to provide customer: "))
			Expect(err.Error()).To(ContainSubstring("source:map[id:abc]"))
		})

		It("returns error if source key does not exist in Stripe response", func() {
			fakeStripe.Responses = []string{
				`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs"}}}`,
			}

			e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
			Expect(err).To(BeNil())

			err = subProvider.HandleChargeSucceeded(db, e)
			Expect(err.Error()).To(ContainSubstring("Stripe failed to provide source: "))
			Expect(err.Error()).To(ContainSubstring("map[id:Xcccs]"))
		})

		Describe("HandleChargeFailed()", func() {
			var (
				stripeID sql.NullString

				fakeHelpScout *test.FakeHelpScout
			)

			fakeHelpScout = &test.FakeHelpScout{}
			subscriptions.HelpScout = fakeHelpScout

			BeforeEach(func() {
				stripeID = sql.NullString{"cus_123", true}
				_, err := db.Exec(`UPDATE billing_profiles SET stripe_id=$1 WHERE user_id=$2;`, stripeID, u.ID)
				Expect(err).To(BeNil())
			})

			It("returns error if customer key does not exist in Stripe response", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"id":"abc"}}}}`,
				}

				e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
				Expect(err).To(BeNil())

				err = subProvider.HandleChargeSucceeded(db, e)
				Expect(err.Error()).To(ContainSubstring("Stripe failed to provide customer: "))
				Expect(err.Error()).To(ContainSubstring("source:map[id:abc]"))
			})

			It("returns error if source key does not exist in Stripe response", func() {
				fakeStripe.Responses = []string{
					`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs"}}}`,
				}

				e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
				Expect(err).To(BeNil())

				err = subProvider.HandleChargeFailed(db, e)
				Expect(err.Error()).To(ContainSubstring("Stripe failed to provide source: "))
				Expect(err.Error()).To(ContainSubstring("map[id:Xcccs]"))
			})

			Context("when first attempt", func() {
				It("increments failed_attempts counter by one and does not mark user as suspicious or send a failed charge ticket to nitrous support", func() {
					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					var attempts int
					err = db.Get(&attempts, `SELECT failed_attempts FROM billing_profiles WHERE stripe_id=$1;`, stripeID)
					Expect(err).To(BeNil())
					Expect(attempts).To(Equal(1))

					suspicious := &sql.NullString{}
					err = db.QueryRow(`SELECT suspicious FROM users WHERE id=$1;`, u.ID).Scan(suspicious)
					Expect(err).To(BeNil())
					Expect(suspicious.Valid).To(BeFalse())

					Eventually(func() int {
						return fakeHelpScout.CreateConversationCalled
					}).Should(Equal(0))
				})
			})

			Context("when max failed attempts", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET failed_attempts=$1 WHERE stripe_id=$2;`, billingprofile.MaxFailedAttempts-1, stripeID)
					Expect(err).To(BeNil())
				})

				It("marks user as suspicious and sends a failed charge ticket to nitrous support if user has no invoice record", func() {
					envvars.HostHostname = "nitrous.io"

					_, err := db.Exec(`DELETE FROM invoices WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())

					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					var attempts int
					err = db.Get(&attempts, `SELECT failed_attempts FROM billing_profiles WHERE stripe_id=$1;`, stripeID)
					Expect(err).To(BeNil())
					Expect(attempts).To(Equal(billingprofile.MaxFailedAttempts))

					usr := &user.User{}
					err = db.QueryRowx(`SELECT id, username, email, suspicious FROM users WHERE id=$1;`, u.ID).StructScan(usr)
					Expect(err).To(BeNil())
					Expect(usr.Suspicious.Valid).To(BeTrue())
					Expect(usr.Suspicious.String).To(Equal("failed_charge"))

					Eventually(func() int {
						return fakeHelpScout.CreateConversationCalled
					}).Should(Equal(1))

					Expect(fakeHelpScout.Customer).To(Equal(helpscout.Person{Type: "customer", Email: subscriptions.HelpscoutProxyEmail}))
					Expect(fakeHelpScout.Subject).To(Equal("[ALERT] Stripe - Multiple Failed Charge Attempts"))
					Expect(fakeHelpScout.Mailbox).To(Equal(helpscout.MailboxRef{ID: int(envvars.HelpScoutMailboxID), Name: (envvars.HelpScoutMailboxName)}))

					Expect(fakeHelpScout.Threads).To(HaveLen(1))
					Expect(fakeHelpScout.Threads[0].Body).To(ContainSubstring("<strong>User</strong>:"))
					Expect(fakeHelpScout.Threads[0].Body).To(ContainSubstring(fmt.Sprintf("ID: %v", usr.ID)))
					Expect(fakeHelpScout.Threads[0].Body).To(ContainSubstring(fmt.Sprintf("Username: %s", usr.Username)))
					Expect(fakeHelpScout.Threads[0].Body).To(ContainSubstring(fmt.Sprintf("Email: %s", usr.Email)))
				})

				It("marks user as suspicious and sends a failed charge ticket to nitrous support if user's invoice state is unpaid", func() {
					start := time.Now().Add(-900 * time.Hour)
					end := start.Add(30 * time.Hour)

					inv := &invoice.Invoice{
						UserID:      u.ID,
						TotalCharge: int64(99),
						State:       invoice.StateUnpaid,
						PeriodStart: &start,
						PeriodEnd:   &end,
						// StripeID should be empty.
					}
					Expect(inv.Insert(db)).To(BeNil())

					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					var attempts int
					err = db.Get(&attempts, `SELECT failed_attempts FROM billing_profiles WHERE stripe_id=$1;`, stripeID)
					Expect(err).To(BeNil())
					Expect(attempts).To(Equal(billingprofile.MaxFailedAttempts))

					usr := &user.User{}
					err = db.QueryRowx(`SELECT id, username, email, suspicious FROM users WHERE id=$1;`, u.ID).StructScan(usr)
					Expect(err).To(BeNil())
					Expect(usr.Suspicious.Valid).To(BeTrue())
					Expect(usr.Suspicious.String).To(Equal("failed_charge"))

					Eventually(func() int {
						return fakeHelpScout.CreateConversationCalled
					}).Should(Equal(2))
				})

				It("does not mark user as suspicious or send a failed charge ticket to nitrous support if user has paid before", func() {
					start := time.Now().Add(-900 * time.Hour)
					end := start.Add(30 * time.Hour)

					inv1 := &invoice.Invoice{
						UserID:      u.ID,
						TotalCharge: int64(99),
						State:       invoice.StateUnpaid,
						PeriodStart: &start,
						PeriodEnd:   &end,
						// StripeID should be empty.
					}
					Expect(inv1.Insert(db)).To(BeNil())

					inv2 := &invoice.Invoice{
						UserID:      u.ID,
						TotalCharge: int64(99),
						State:       invoice.StatePaid,
						PeriodStart: &start,
						PeriodEnd:   &end,
					}
					Expect(inv2.Insert(db)).To(BeNil())

					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					suspicious := &sql.NullString{}
					err = db.QueryRow(`SELECT suspicious FROM users WHERE id=$1;`, u.ID).Scan(suspicious)
					Expect(err).To(BeNil())
					Expect(suspicious.Valid).To(BeFalse())

					Eventually(func() int {
						return fakeHelpScout.CreateConversationCalled
					}).Should(Equal(2))
				})

				It("should append to existing suspicious column value when marking user as suspicious", func() {
					_, err := db.Exec(`DELETE FROM invoices WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE users SET suspicious=$1 WHERE id=$2;`, sql.NullString{"a_spy", true}, u.ID)
					Expect(err).To(BeNil())

					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					var attempts int
					err = db.Get(&attempts, `SELECT failed_attempts FROM billing_profiles WHERE stripe_id=$1;`, stripeID)
					Expect(err).To(BeNil())
					Expect(attempts).To(Equal(billingprofile.MaxFailedAttempts))

					usr := &user.User{}
					err = db.QueryRowx(`SELECT id, username, email, suspicious FROM users WHERE id=$1;`, u.ID).StructScan(usr)
					Expect(err).To(BeNil())
					Expect(usr.Suspicious.Valid).To(BeTrue())
					Expect(usr.Suspicious.String).To(Equal("a_spy,failed_charge"))
				})

				It("sends a failed charge ticket to nitrous support with staging label if staging environment", func() {
					_, err := db.Exec(`DELETE FROM invoices WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())
					envvars.HostHostname = "n2ostaging.com"

					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					Expect(fakeHelpScout.Subject).To(Equal("[STAGING] [ALERT] Stripe - Multiple Failed Charge Attempts"))
				})

				It("sends a failed charge ticket to nitrous support with dev label if development environment", func() {
					_, err := db.Exec(`DELETE FROM invoices WHERE user_id=$1;`, u.ID)
					Expect(err).To(BeNil())
					envvars.HostHostname = "n2odev.com"

					fakeStripe.Responses = []string{
						`{"id": "evt_158gxvAOjoYioo6erVsU1zBV", "api_version": "2014-12-08", "object": "event", "type": "charge.succeeded", "data": {"object": {"id": "Xcccs", "source": {"customer":"cus_123"}}}}`,
					}

					e, err := event.Get("evt_158gxvAOjoYioo6erVsU1zBV")
					Expect(err).To(BeNil())

					err = subProvider.HandleChargeFailed(db, e)
					Expect(err).To(BeNil())

					Expect(fakeHelpScout.Subject).To(Equal("[DEV] [ALERT] Stripe - Multiple Failed Charge Attempts"))
				})

			})
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
				Slug:             "SLUG",
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
		})

		It("should return an error if an invoice already exists", func() {
			_, err := db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end, stripe_id, subscription_id) VALUES ($1, 'paid', now(), now(), 'inv1', $2)", u.ID, sub.ID)
			Expect(err).To(BeNil())

			inv, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(Equal(subscriptions.ErrInvoiceAlreadyCreated))
			Expect(inv).To(BeNil())
		})

		It("should not return an error if an invoice already exists but has no subscription", func() {
			_, err := db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end, stripe_id) VALUES ($1, 'paid', now(), now(), 'inv1')", u.ID)
			Expect(err).To(BeNil())

			inv, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())
			Expect(inv).NotTo(BeNil())
		})

		It("should return an error if it cannot fetch the invoice", func() {
			fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
			fakeStripe.Responses = []string{
				`{
				  "id": "invoice-1",
				  "object": "invoice",
				}`,
			}
			inv, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(Equal(fakeStripe.Errors[0]))
			Expect(inv).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("GET"))
			Expect(r1["path"]).To(Equal("/invoices/inv1"))
		})

		It("should not create the invoice if it is purely for proration", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				"id": "inv1",
				"object": "invoice",
				"subscription": null,
				"total": 1499,
				"amount_due": 1499,
				"period_start": 1434418419,
				"period_end": 1437010419,
				"attempts": 2,
				"stripe_id": "str-invoice-1",
				"paid": true,
				"lines": {
				"data": [
				{
					"id": "sub_1",
					"object": "line_item",
					"type": "subscription",
					"livemode": true,
					"amount": 1499,
					"currency": "usd",
					"proration": true,
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
			_, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("GET"))
			Expect(r1["path"]).To(Equal("/invoices/inv1"))

			_, err = invoice.FindByStripeID(db, "inv1")
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should create the invoice with invoice items", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				"id": "inv1",
				"object": "invoice",
				"subscription": "abc1",
				"total": 1499,
				"amount_due": 1499,
				"period_start": 1434418419,
				"period_end": 1437010419,
				"attempts": 2,
				"stripe_id": "str-invoice-1",
				"paid": true,
				"lines": {
				"data": [
				{
					"id": "sub_1",
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
			i, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
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
			Expect(inv).To(Equal(i))
			Expect(inv.BillingProfileID.Int64).To(Equal(bp.ID))

			items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(1))
			Expect(items[0].ItemType).To(Equal(invoiceitem.ItemTypeSubscription))
			Expect(items[0].StripeID.String).To(Equal("inv1-sub_1"))
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
							"id": "sub_1",
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
								"type": "invoiceitem",
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
			i, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())

			inv, err := invoice.FindByStripeID(db, "inv1")
			Expect(err).To(BeNil())
			Expect(inv).To(Equal(i))

			items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(2))
			Expect(items[0].ItemType).To(Equal(invoiceitem.ItemTypeSubscription))
			Expect(items[0].StripeID.String).To(Equal("inv1-sub_1"))
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
				"amount_due": 1499,
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
			i, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
			Expect(err).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
			r := fakeStripe.ReceivedRequests[1]
			Expect(r["method"]).To(Equal("POST"))
			Expect(r["path"]).To(Equal("/invoiceitems"))
			Expect(r["customer"]).To(Equal(stripeCustomerID))
			Expect(r["amount"]).To(Equal("-100"))

			inv, err := invoice.FindByStripeID(db, "inv1")
			Expect(err).To(BeNil())
			Expect(inv.TotalCharge).To(Equal(int64(1399)))
			Expect(inv).To(Equal(i))

			items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(2))

			Expect(items[1].ItemType).To(Equal(invoiceitem.ItemTypeCredit))
			Expect(items[1].StripeID.String).To(Equal("stripe-item-2"))
			Expect(items[1].TotalStripeCharge.Int64).To(Equal(int64(-100)))
			Expect(items[1].Credit.Int64).To(Equal(int64(100)))
		})

		Describe("special case of invoices with both subscription and deferred invoice items", func() {
			var sub *subscription.Subscription

			BeforeEach(func() {
				// Create existing subscription, deferred invoices and invoice items.
				bp, err := u.BillingProfile(db)
				Expect(err).To(BeNil())
				sub = &subscription.Subscription{
					StripeID:         sql.NullString{"sub_1", true},
					PlanID:           pl.ID,
					BillingProfileID: bp.ID,
					Slug:             "SUB1",
				}
				Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())
				s := time.Now().Add(-900 * time.Hour)
				e := s.Add(30 * time.Hour)

				inv := &invoice.Invoice{
					UserID:      u.ID,
					TotalCharge: int64(99),
					State:       invoice.StateDeferred,
					PeriodStart: &s,
					PeriodEnd:   &e,
					// StripeID should be empty.
				}
				Expect(inv.Insert(db)).To(BeNil())
				invItem := &invoiceitem.InvoiceItem{
					InvoiceID:   inv.ID,
					HostID:      sql.NullInt64{hs.H.ID, true},
					ItemType:    invoiceitem.ItemTypeCompute,
					StripeID:    sql.NullString{"stripe-item-2", true},
					Hours:       sql.NullInt64{2, true},
					HourlyPrice: sql.NullInt64{1874, true},
					PlanMemory:  sql.NullString{"2GB", true},
				}
				Expect(invItem.Insert(db)).To(BeNil())

				inv2 := &invoice.Invoice{
					UserID:      u.ID,
					TotalCharge: int64(50),
					State:       invoice.StateDeferred,
					PeriodStart: &s,
					PeriodEnd:   &e,
					// StripeID should be empty.
				}
				Expect(inv2.Insert(db)).To(BeNil())
				invItem2 := &invoiceitem.InvoiceItem{
					InvoiceID:   inv2.ID,
					HostID:      sql.NullInt64{hs.H.ID, true},
					ItemType:    invoiceitem.ItemTypeCompute,
					StripeID:    sql.NullString{"stripe-item-3", true},
					Hours:       sql.NullInt64{1, true},
					HourlyPrice: sql.NullInt64{1874, true},
					PlanMemory:  sql.NullString{"2GB", true},
				}
				Expect(invItem2.Insert(db)).To(BeNil())

				fakeStripe.Errors = []*stripe.Error{nil}
				fakeStripe.Responses = []string{
					`{
						"id": "inv1",
						"object": "invoice",
						"subscription": "sub_1",
						"total": 1648,
						"amount_due": 1648,
						"period_start": 1434418419,
						"period_end": 1437010419,
						"attempts": 2,
						"stripe_id": "str-invoice-1",
						"paid": true,
						"lines": {
							"data": [
								{
									"id": "sub_1",
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
								},
								{
									"id": "stripe-item-2",
									"object": "line_item",
									"type": "invoiceitem",
									"livemode": true,
									"amount": 99,
									"currency": "usd",
									"proration": false,
									"period": {
										"start": 1434418419,
										"end": 1437010419
									}
								},
								{
									"id": "stripe-item-3",
									"object": "line_item",
									"type": "invoiceitem",
									"livemode": true,
									"amount": 50,
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

			})

			It("creates a new paid invoice with an invoice item for the subscription, and deletes the deferred invoice and invoice items", func() {
				i, err := subProvider.HandleSubscriptionInvoiceCreated(db, "inv1")
				Expect(err).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
				r := fakeStripe.ReceivedRequests[0]
				Expect(r["method"]).To(Equal("GET"))
				Expect(r["path"]).To(Equal("/invoices/inv1"))

				inv, err := invoice.FindByStripeID(db, "inv1")
				Expect(err).To(BeNil())
				Expect(inv.State).To(Equal(invoice.StatePaid))
				Expect(inv.TotalCharge).To(Equal(int64(1499 + 99 + 50)))
				Expect(inv.StripeID.String).To(Equal("inv1"))
				Expect(inv.SubscriptionID.Int64).To(Equal(sub.ID))
				Expect(inv).To(Equal(i))
				Expect(inv.BillingProfileID.Int64).To(Equal(bp.ID))

				items, err := invoiceitem.FindAllForInvoice(db, inv.ID)
				Expect(err).To(BeNil())
				Expect(items).To(HaveLen(3))

				computeItemStripeIDs := []string{}
				for _, item := range items {
					switch item.ItemType {
					case invoiceitem.ItemTypeCompute:
						computeItemStripeIDs = append(computeItemStripeIDs, item.StripeID.String)
					case invoiceitem.ItemTypeSubscription:
						Expect(item.StripeID.String).To(Equal("inv1-sub_1"))
						Expect(item.TotalStripeCharge.Int64).To(Equal(int64(1499)))
					}
				}
				Expect(computeItemStripeIDs).To(ConsistOf("stripe-item-2", "stripe-item-3"))
			})
		})
	})

	Describe("HandleSubscriptionInvoiceFailed", func() {
		var (
			sub *subscription.Subscription
			inv *invoice.Invoice
			bp  *billingprofile.BillingProfile

			hstName string
		)

		BeforeEach(func() {
			var err error
			bp, err = u.BillingProfile(db)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE billing_profiles SET last_four = '2334' WHERE id = $1;", bp.ID)
			Expect(err).To(BeNil())

			sub = &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				Slug:             "SLUG",
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())

			s := time.Now().Add(-900 * time.Hour)
			e := s.Add(30 * time.Hour)
			inv = &invoice.Invoice{
				UserID:         u.ID,
				PeriodStart:    &s,
				PeriodEnd:      &e,
				State:          invoice.StateUnpaid,
				SubscriptionID: sql.NullInt64{sub.ID, true},
				StripeID:       sql.NullString{"inv_ABCD1", true},
				TotalCharge:    1499,
			}

			Expect(inv.Insert(db)).To(BeNil())

			// mock invoice was created 6 days earlier (to satisfy subscriptions.minTimeBeforeFinalAttempt)
			_, err = db.Exec("UPDATE invoices SET created_at = now() - interval '6 days' WHERE id = $1;", inv.ID)

			hstName = envvars.WebHostname
			envvars.WebHostname = "nitrous.example"
		})

		AfterEach(func() {
			envvars.WebHostname = hstName
			subscriptions.SetCouponGetFn(subscriptions.DefaultCouponGetFn)
		})

		It("should return an error if the invoice cannot be fetched", func() {
			_, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_NO")
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("should return an error if the invoice has already been paid", func() {
			_, err := db.Exec("UPDATE invoices SET state = $1 WHERE id = $2;", invoice.StatePaid, inv.ID)
			Expect(err).To(BeNil())

			_, err = subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).ToNot(BeNil())
			Expect(err).To(MatchError(fmt.Sprintf("Invoice %d has already been paid", inv.ID)))
		})

		It("should return an error if Stripe errors out", func() {
			fakeStripe.Errors = []*stripe.Error{&stripe.Error{}}
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
			}

			_, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(Equal(fakeStripe.Errors[0]))
		})

		It("should return an error if Stripe has marked the invoice as paid", func() {
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
              }
            ],
            "total_count": 1,
            "object": "list",
            "url": "/v1/invoices/inv1/lines"
          }
				}`,
			}

			_, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(MatchError("Stripe invoice inv1 has already been paid"))
		})

		It("should return an error if the billing profile is exempt", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE id = $2;", billingprofile.StateExempt, bp.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "next_payment_attempt": 1434699271,
          "attempt_count": 1,
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
			}

			_, err = subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(MatchError(fmt.Sprintf("Billing profile %d should not be failing in exempt state", bp.ID)))
		})

		It("should update the billing profile that payment is due & update the invoice", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "next_payment_attempt": 1434699271,
          "attempt_count": 1,
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
			}

			i, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).ToNot(BeNil())

			Expect(i.State).To(Equal(invoice.StateUnpaid))
			Expect(*i.FirstAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(*i.LastAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(i.Attempts).To(Equal(1))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaymentDue))

			i2, err := invoice.FindByStripeID(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Tos).To(Equal([]string{`"foo" <foo@example.com>`}))
			Expect(fakeMailer.Ccs).To(Equal([]string{envvars.SupportEmail}))
			Expect(fakeMailer.Bccs).To(BeEmpty())
			Expect(fakeMailer.Subject).To(Equal("Nitrous - payment unsuccessful"))
			Expect(fakeMailer.Body).To(ContainSubstring("Unfortunately, we have been unable to successfully"))
			Expect(fakeMailer.Body).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.Body).To(ContainSubstring("verify your credit card at https://nitrous.example/app/#/billing"))
			Expect(fakeMailer.Body).To(ContainSubstring("email our support team at support@nitrous.io"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Unfortunately, we have been unable to successfully"))
			Expect(fakeMailer.HTML).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/billing">verify your credit card</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring(`email our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>`))
		})

		It("should handle the first retry attempt", func() {
			t := time.Unix(1434681522, 0)
			_, err := db.Exec("UPDATE invoices SET first_attempt_at = $1 WHERE id = $2;", t, inv.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"id": "inv1",
					"object": "invoice",
					"subscription": "abc1",
					"total": 1499,
					"period_start": 1434418419,
					"period_end": 1437010419,
					"next_payment_attempt": 1434699271,
					"attempt_count": 2,
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
			}

			i, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).ToNot(BeNil())

			Expect(i.State).To(Equal(invoice.StateUnpaid))
			Expect(*i.FirstAttemptAt).To(BeTemporally("~", t, time.Second))
			Expect(*i.LastAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(i.Attempts).To(Equal(2))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaymentDue))

			i2, err := invoice.FindByStripeID(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"foo" <foo@example.com>`}))
			Expect(fakeMailer.Ccs).To(Equal([]string{`"Nitrous" <support@nitrous.io>`}))
			Expect(fakeMailer.Bccs).To(BeEmpty())
			Expect(fakeMailer.Subject).To(Equal("Nitrous - payment retry unsuccessful"))
			Expect(fakeMailer.Body).To(ContainSubstring("We are still having issues successfully"))
			Expect(fakeMailer.Body).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.Body).To(ContainSubstring("We will retry billing your card in 3 days"))
			Expect(fakeMailer.Body).To(ContainSubstring("verify your credit card at https://nitrous.example/app/#/billing"))
			Expect(fakeMailer.Body).To(ContainSubstring("email our support team at support@nitrous.io"))
			Expect(fakeMailer.Body).To(ContainSubstring("if we are unable to bill your credit card by Friday Jun 19 2015 (UTC)"))
			Expect(fakeMailer.HTML).To(ContainSubstring("We are still having issues successfully"))
			Expect(fakeMailer.HTML).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.HTML).To(ContainSubstring("We will retry billing your card in 3 days"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/billing">verify your credit card</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring(`email our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring("if we are unable to bill your credit card by Friday Jun 19 2015 (UTC)"))
		})

		It("should handle the second retry attempt", func() {
			t := time.Unix(1434681522, 0)
			_, err := db.Exec("UPDATE invoices SET first_attempt_at = $1 WHERE id = $2;", t, inv.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"id": "inv1",
					"object": "invoice",
					"subscription": "abc1",
					"total": 1499,
					"period_start": 1434418419,
					"period_end": 1437010419,
					"next_payment_attempt": 1434699271,
					"attempt_count": 3,
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
			}

			i, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).ToNot(BeNil())

			Expect(i.State).To(Equal(invoice.StateUnpaid))
			Expect(*i.FirstAttemptAt).To(BeTemporally("~", t, time.Second))
			Expect(*i.LastAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(i.Attempts).To(Equal(3))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaymentDue))

			i2, err := invoice.FindByStripeID(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"foo" <foo@example.com>`}))
			Expect(fakeMailer.Ccs).To(Equal([]string{`"Nitrous" <support@nitrous.io>`}))
			Expect(fakeMailer.Bccs).To(BeEmpty())
			Expect(fakeMailer.Subject).To(Equal("Nitrous - final warning"))
			Expect(fakeMailer.Body).To(ContainSubstring("We are still having issues successfully"))
			Expect(fakeMailer.Body).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.Body).To(ContainSubstring("We will retry billing your card very shortly"))
			Expect(fakeMailer.Body).To(ContainSubstring("verify your credit card at https://nitrous.example/app/#/billing"))
			Expect(fakeMailer.Body).To(ContainSubstring("terminated after our next retry"))
			Expect(fakeMailer.Body).To(ContainSubstring("email our support team at support@nitrous.io"))
			Expect(fakeMailer.Body).To(ContainSubstring("if we are unable to bill your credit card by Friday Jun 19 2015 (UTC)"))
			Expect(fakeMailer.HTML).To(ContainSubstring("We are still having issues successfully"))
			Expect(fakeMailer.HTML).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.HTML).To(ContainSubstring("We will retry billing your card very shortly"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/billing">verify your credit card</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring("terminated after our next retry"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`email our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring("if we are unable to bill your credit card by Friday Jun 19 2015 (UTC)"))
		})

		It("should handle when the payment gives up", func() {
			t := time.Unix(1434681522, 0)
			_, err := db.Exec("UPDATE invoices SET first_attempt_at = $1 WHERE id = $2;", t, inv.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempt_count": 4,
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
			}

			i, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).ToNot(BeNil())

			Expect(i.State).To(Equal(invoice.StateFailed))
			Expect(*i.FirstAttemptAt).To(BeTemporally("~", t, time.Second))
			Expect(*i.LastAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(i.Attempts).To(Equal(4))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateDelinquent))

			i2, err := invoice.FindByStripeID(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"foo" <foo@example.com>`}))
			Expect(fakeMailer.Ccs).To(Equal([]string{`"Nitrous" <support@nitrous.io>`}))
			Expect(fakeMailer.Bccs).To(BeEmpty())
			Expect(fakeMailer.Subject).To(Equal("Nitrous - your workspaces will be terminated"))
			Expect(fakeMailer.Body).To(ContainSubstring("Unfortunately, we have been unable to"))
			Expect(fakeMailer.Body).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.Body).To(ContainSubstring("verify your credit card at https://nitrous.example/app/#/billing"))
			Expect(fakeMailer.Body).To(ContainSubstring("email our support team at support@nitrous.io"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Unfortunately, we have been unable to successfully"))
			Expect(fakeMailer.HTML).To(ContainSubstring("ending in 2334 to the value of $14.99"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/billing">verify your credit card</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring(`email our support team at <a href="mailto:support@nitrous.io">support@nitrous.io</a>`))
		})

		It("should not allow next_payment_attempt to be 0 if the invoice is too new", func() {
			t := time.Unix(1434681522, 0)
			_, err := db.Exec("UPDATE invoices SET created_at = now(), first_attempt_at = $1 WHERE id = $2;", t, inv.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempt_count": 3,
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
			}

			_, err = subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).ToNot(BeNil())
			eMsg := fmt.Sprintf("Received an invoice failed event with next_attempt=0 for invoice ID %d", inv.ID)
			Expect(err.Error()).To(ContainSubstring(eMsg))

			i, err := invoice.FindByStripeID(db, "inv_ABCD1")
			Expect(err).To(BeNil())

			Expect(i.State).To(Equal(invoice.StateUnpaid))
		})

		It("should stop the hosts", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempt_count": 3,
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
			}

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
				Expect(headers.Get(constants.NitrousUUIDHeader)).To(Equal(fmt.Sprintf("subscription_terminated_%d", sub.ID)))
				Expect(initiatorID).To(Equal(prov.ID))
				termCalled = true
				return e
			}

			i, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).ToNot(BeNil())

			Expect(termCalled).To(BeTrue())
		})

		It("should be compatible with invoices that don't have subscriptions", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
				  "id": "inv1",
				  "object": "invoice",
          "subscription": "abc1",
          "total": 1499,
          "period_start": 1434418419,
          "period_end": 1437010419,
          "attempt_count": 3,
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
			}

			_, err := db.Exec("UPDATE invoices SET subscription_id = NULL WHERE id = $1;", inv.ID)
			Expect(err).To(BeNil())

			i, err := subProvider.HandleSubscriptionInvoiceFailed(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).ToNot(BeNil())

			Expect(i.State).To(Equal(invoice.StateFailed))
			Expect(*i.FirstAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(*i.LastAttemptAt).To(BeTemporally("~", time.Now(), time.Second))
			Expect(i.Attempts).To(Equal(3))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateDelinquent))

			i2, err := invoice.FindByStripeID(db, "inv_ABCD1")
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))
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

func findByOrgID(d ndb.QueryReceiver, id int64) (*subscription.Subscription, error) {
	sub := &subscription.Subscription{}
	err := d.Get(sub, `SELECT subscriptions.* FROM subscriptions
	JOIN billing_profiles ON billing_profiles.id = subscriptions.billing_profile_id
	JOIN orgs ON orgs.id = billing_profiles.org_id
	WHERE orgs.id = $1;`, id)

	if err != nil {
		return nil, err
	}

	return sub, nil
}
