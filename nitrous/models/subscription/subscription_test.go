package subscription_test

import (
	"database/sql"
	"testing"

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
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{}
			Expect(db.Get(sub, "SELECT * FROM subscriptions WHERE id = $1;", s.ID)).To(BeNil())
			Expect(sub).To(Equal(s))
		})
	})

	Describe("FindAllByUserID", func() {
		var (
			u1 *user.User
			u2 *user.User
			p  *plan.Plan
			s1 *subscription.Subscription
			s2 *subscription.Subscription
			s3 *subscription.Subscription
		)

		BeforeEach(func() {
			var err error

			u1 = &user.User{
				Username: "user1",
				Email:    "user1@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "user2",
				Email:    "user2@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			// Insert plan
			p = &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Price:         1024,
				Storage:       "1024 GB",
				Memory:        "68 GB",
			}
			p.Insert(db)

			// Insert subscriptions
			bp1, err := u1.BillingProfile(db)
			s1 = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
			}
			err = test.WrapInTransaction(db, s1.Insert)
			Expect(err).To(BeNil())

			s2 = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp1.ID,
				State:            "deleted",
				StripeID:         sql.NullString{"2", true},
			}
			err = test.WrapInTransaction(db, s2.Insert)
			Expect(err).To(BeNil())

			bp2, err := u2.BillingProfile(db)
			s3 = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp2.ID,
				State:            "active",
				StripeID:         sql.NullString{"3", true},
			}
			err = test.WrapInTransaction(db, s3.Insert)
			Expect(err).To(BeNil())
		})

		It("returns active subscriptions for the given user id", func() {
			subscriptions, err := subscription.FindAllActiveByUserID(db, u1.ID)
			Expect(err).To(BeNil())

			Expect(subscriptions).To(HaveLen(1))
			Expect(subscriptions[0].ID).To(Equal(s1.ID))
		})
	})
})
