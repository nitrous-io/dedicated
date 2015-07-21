package subscription_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/subscription")
}

var _ = Describe("models/subscription", func() {
	var (
		db *ndb.Database
		u  *user.User
		pl *plan.Plan
		hs *test.HostStack
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
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
	})

	Describe("Insert", func() {
		It("should set the values", func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			s := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            subscription.StateCreating,
				StripeID:         sql.NullString{"sub_1", true},
				Slug:             sql.NullString{"IAMASLUG90", true},
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{}
			Expect(db.Get(sub, "SELECT * FROM subscriptions WHERE id = $1;", s.ID)).To(BeNil())
			Expect(sub).To(Equal(s))
		})
	})

	Describe("Update", func() {
		var sub *subscription.Subscription

		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"sub_1", true},
				Slug:             sql.NullString{"SLUG", true},
			}
			err = test.WrapInTransaction(db, sub.Insert)
			Expect(err).To(BeNil())
		})

		It("updates the host in the database", func() {
			p := &plan.Plan{
				Slug:          "p2",
				Name:          "p2",
				MaxContainers: 2,
				Price:         2599,
				StripePlanID:  sql.NullString{"stripe-plan-2", true},
			}
			Expect(p.Insert(db)).To(BeNil())

			sub.State = "updated"
			sub.PlanID = p.ID
			sub.StripeID = sql.NullString{"sub_2", true}
			periodStart := time.Date(2013, time.November, 1, 1, 0, 0, 0, time.UTC)
			periodEnd := time.Date(2013, time.November, 2, 1, 0, 0, 0, time.UTC)
			sub.CurrentPeriodStart = pq.NullTime{periodStart, true}
			sub.CurrentPeriodEnd = pq.NullTime{periodEnd, true}

			err := sub.Update(db)
			Expect(err).To(BeNil())

			sub2, err := subscription.Find(db, sub.ID)
			Expect(err).To(BeNil())

			// Updated fields.
			Expect(sub2.State).To(Equal("updated"))
			Expect(sub2.PlanID).To(Equal(p.ID))
			Expect(sub2.StripeID.String).To(Equal("sub_2"))
			Expect(sub2.CurrentPeriodStart.Time).To(BeTemporally("==", periodStart))
			Expect(sub2.CurrentPeriodEnd.Time).To(BeTemporally("==", periodEnd))

			// Unchanged fields.
			Expect(sub2.BillingProfileID).To(Equal(sub.BillingProfileID))
			Expect(sub2.CancelPending).To(Equal(sub.CancelPending))
			Expect(sub2.CancelledAt.Time).To(Equal(sub.CancelledAt.Time))
		})

		It("updates UpdateAt", func() {
			origUpdatedAt := sub.UpdatedAt

			err := sub.Update(db)
			Expect(err).To(BeNil())

			Expect(*sub.UpdatedAt).To(BeTemporally(">", *origUpdatedAt))
		})

		It("does not update Slug", func() {
			sub.Slug = sql.NullString{"NOSLUG", true}
			err := sub.Update(db)
			Expect(err).To(BeNil())

			sub2, err := subscription.Find(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(sub2.Slug.String).To(Equal("SLUG"))
		})
	})

	Describe("FindByUserID", func() {
		var (
			s1 *subscription.Subscription
			s2 *subscription.Subscription
			s3 *subscription.Subscription
			u2 *user.User
		)

		BeforeEach(func() {
			// Insert subscriptions
			bp1, err := u.BillingProfile(db)
			Expect(err).To(BeNil())
			s1 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
				Slug:             sql.NullString{"SUB1", true},
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())

			s2 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"X", true},
				Slug:             sql.NullString{"SUB2", true},
			}
			Expect(test.WrapInTransaction(db, s2.Insert)).To(BeNil())

			u2 = &user.User{
				Username: "user2",
				Email:    "user2@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			bp2, err := u2.BillingProfile(db)
			Expect(err).To(BeNil())
			s3 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp2.ID,
				State:            "active",
				StripeID:         sql.NullString{"3", true},
				Slug:             sql.NullString{"SUB3", true},
			}
			Expect(test.WrapInTransaction(db, s3.Insert)).To(BeNil())
		})

		It("should find a subscription belonging to a user", func() {
			s, err := subscription.FindByUserID(db, s1.ID, u.ID)
			Expect(err).To(BeNil())
			Expect(s).To(Equal(s1))

			s, err = subscription.FindByUserID(db, s2.ID, u.ID)
			Expect(err).To(BeNil())
			Expect(s).To(Equal(s2))
		})

		It("should not find a subscription if it doesn't belong to the user", func() {
			s, err := subscription.FindByUserID(db, s1.ID, u2.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(s).To(BeNil())
		})
	})

	Describe("FindAllByUserID", func() {
		var (
			s1 *subscription.Subscription
			s2 *subscription.Subscription
		)

		BeforeEach(func() {
			u2 := &user.User{
				Username: "user2",
				Email:    "user2@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			// Insert subscriptions
			bp1, err := u.BillingProfile(db)
			s1 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
				Slug:             sql.NullString{"SUB1", true},
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())

			s2 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"X", true},
				Slug:             sql.NullString{"SUB2", true},
			}
			Expect(test.WrapInTransaction(db, s2.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s2.ID, hs.H.ID)
			Expect(err).To(BeNil())

			s3 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "cancelled",
				StripeID:         sql.NullString{"2", true},
				Slug:             sql.NullString{"SUB3", true},
			}
			Expect(test.WrapInTransaction(db, s3.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE subscriptions SET cancelled_at = now() WHERE id = $1;", s3.ID)
			Expect(err).To(BeNil())

			bp2, err := u2.BillingProfile(db)
			s4 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp2.ID,
				State:            "active",
				StripeID:         sql.NullString{"3", true},
				Slug:             sql.NullString{"SUB4", true},
			}
			Expect(test.WrapInTransaction(db, s4.Insert)).To(BeNil())
		})

		It("returns active subscriptions for the given user id", func() {
			subscriptions, err := subscription.FindAllActiveByUserID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(subscriptions).To(HaveLen(2))
			Expect(subscriptions[0].ID).To(Equal(s1.ID))
			Expect(subscriptions[0].HostSlug.Valid).To(BeFalse())

			Expect(subscriptions[1].ID).To(Equal(s2.ID))
			Expect(subscriptions[1].HostSlug.String).To(Equal("my-host-1"))
		})
	})
})
