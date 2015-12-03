package promocode_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/promocode")
}

var _ = Describe("models/promocode", func() {
	var (
		db    *db.Database
		u     *user.User
		p, p2 *plan.Plan

		pcID1 int64
		pcID2 int64
		pcID3 int64
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		p = &plan.Plan{
			Slug:          "promoPlan",
			Name:          "promoPlan",
			MaxContainers: 1,
			Price:         500,
			BasePrice:     500,
		}
		Expect(p.Insert(db)).To(BeNil())

		p2 = &plan.Plan{
			Slug:          "promo2Plan",
			Name:          "promo2Plan",
			MaxContainers: 1,
			Price:         500,
			BasePrice:     500,
		}
		Expect(p2.Insert(db)).To(BeNil())

		err = db.Get(&pcID1, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('just10', 10, 900, now()+interval '2 days') RETURNING id;")
		Expect(err).To(BeNil())
		err = db.Get(&pcID2, "INSERT INTO promo_codes(slug, quantity) VALUES ('just20', 20) RETURNING id;")
		Expect(err).To(BeNil())
		err = db.Get(&pcID3, "INSERT INTO promo_codes(slug, quantity, trial_seconds) VALUES ('just30', 30, 400) RETURNING id;")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO promo_code_plans(promo_code_id, plan_id) SELECT id, $1 FROM promo_codes WHERE id IN ($2, $3, $4)", p.ID, pcID1, pcID2, pcID3)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Claim", func() {
		It("should return an error if the slug does not exist", func() {
			use, err := promocode.Claim(db, "jest10", u.ID, p.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(use).To(BeNil())
		})

		It("should return an error if the plan is not associated with the promo code", func() {
			use, err := promocode.Claim(db, "jest10", u.ID, p2.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(use).To(BeNil())
		})

		It("should return an error if the promo has expired", func() {
			_, err := db.Exec("UPDATE promo_codes SET expires_at = now() WHERE id = $1", pcID1)
			Expect(err).To(BeNil())
			use, err := promocode.Claim(db, "just10", u.ID, p.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(use).To(BeNil())
		})

		It("should insert successfully if there is enough room", func() {
			use, err := promocode.Claim(db, "just10", u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(use).ToNot(BeNil())
			Expect(use.PromoCodeID).To(Equal(pcID1))
		})

		It("should not insert successfully if there isn't any room", func() {
			for i := 0; i < 10; i++ {
				ou := &user.User{
					Username: fmt.Sprintf("user%d", i),
					Password: "123456",
					Email:    fmt.Sprintf("user%d@example.com", i),
				}
				Expect(test.WrapInTransaction(db, ou.Insert)).To(BeNil())

				use, err := promocode.Claim(db, "just10", ou.ID, p.ID)
				Expect(err).To(BeNil())
				Expect(use).ToNot(BeNil())
				Expect(use.PromoCodeID).To(Equal(pcID1))
				Expect(use.UserID).To(Equal(ou.ID))
			}

			use, err := promocode.Claim(db, "just10", u.ID, p.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(use).To(BeNil())

			var count int
			err = db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses u
				INNER JOIN promo_codes c ON c.id = u.promo_code_id
				WHERE slug = $1`, "just10")
			Expect(err).To(BeNil())
			Expect(count).To(Equal(10))
		})

		It("should insert successfully for a different promocode", func() {
			for i := 0; i < 10; i++ {
				ou := &user.User{
					Username: fmt.Sprintf("user%d", i),
					Password: "123456",
					Email:    fmt.Sprintf("user%d@example.com", i),
				}
				Expect(test.WrapInTransaction(db, ou.Insert)).To(BeNil())

				use, err := promocode.Claim(db, "just10", ou.ID, p.ID)
				Expect(err).To(BeNil())
				Expect(use).ToNot(BeNil())
				Expect(use.PromoCodeID).To(Equal(pcID1))
				Expect(use.UserID).To(Equal(ou.ID))
			}

			use, err := promocode.Claim(db, "just20", u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(use.PromoCodeID).To(Equal(pcID2))

			var count int
			err = db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses u
				INNER JOIN promo_codes c ON c.id = u.promo_code_id
				WHERE slug = $1`, "just10")
			Expect(err).To(BeNil())
			Expect(count).To(Equal(10))

			err = db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses u
				INNER JOIN promo_codes c ON c.id = u.promo_code_id
				WHERE slug = $1`, "just20")
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})
	})

	Describe("FindPendingClaim", func() {
		It("should return an error if there is no pending claim", func() {
			cl, err := promocode.FindPendingClaim(db, u.ID, p.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(cl).To(BeNil())
		})

		It("should find an outstanding claim", func() {
			use, err := promocode.Claim(db, "just10", u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(use).ToNot(BeNil())

			cl, err := promocode.FindPendingClaim(db, u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(cl).ToNot(BeNil())
			Expect(cl.Use.ID).To(Equal(use.ID))
			Expect(cl.Code.ID).To(Equal(pcID1))
		})

		It("should not find an outstanding claim if it relates to a different plan", func() {
			_, err := db.Exec("DELETE FROM promo_code_plans WHERE plan_id = $1", p.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO promo_code_plans(promo_code_id, plan_id) SELECT id, $1 FROM promo_codes WHERE id IN ($2, $3, $4)", p2.ID, pcID1, pcID2, pcID3)
			Expect(err).To(BeNil())

			use, err := promocode.Claim(db, "just10", u.ID, p2.ID)
			Expect(err).To(BeNil())
			Expect(use).ToNot(BeNil())

			cl, err := promocode.FindPendingClaim(db, u.ID, p.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(cl).To(BeNil())
		})
	})

	Describe("UpdateSubscriptionId", func() {
		var (
			use *promocode.PromoCodeUse
			sub *subscription.Subscription
		)

		BeforeEach(func() {
			var err error
			use, err = promocode.Claim(db, "just10", u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(use).ToNot(BeNil())

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			pl := &plan.Plan{
				Slug:          "hs1Plan",
				Name:          "hs1Plan",
				MaxContainers: 1,
				Price:         1499,
				StripePlanID:  sql.NullString{"stripe-plan-id", true},
			}
			Expect(pl.Insert(db)).To(BeNil())

			sub = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"sub_1", true},
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, sub.Insert)
			Expect(err).To(BeNil())
		})

		It("should update just the subscription_id", func() {
			use.SubscriptionID = sql.NullInt64{sub.ID, true}
			use.UserID = 900
			Expect(use.UpdateSubscriptionId(db)).To(BeNil())

			var count int
			err := db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses u
				INNER JOIN promo_codes c ON c.id = u.promo_code_id
				WHERE slug = $1 AND subscription_id = $2 AND user_id != 900`, "just10", sub.ID)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})
	})

	Describe("Delete", func() {
		var (
			use, use2 *promocode.PromoCodeUse
			sub       *subscription.Subscription
		)

		BeforeEach(func() {
			var err error
			use, err = promocode.Claim(db, "just10", u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(use).ToNot(BeNil())

			use2, err = promocode.Claim(db, "just20", u.ID, p.ID)
			Expect(err).To(BeNil())
			Expect(use2).ToNot(BeNil())

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			pl := &plan.Plan{
				Slug:          "hs1Plan",
				Name:          "hs1Plan",
				MaxContainers: 1,
				Price:         1499,
				StripePlanID:  sql.NullString{"stripe-plan-id", true},
			}
			Expect(pl.Insert(db)).To(BeNil())

			sub = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"sub_1", true},
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, sub.Insert)
			Expect(err).To(BeNil())
		})

		It("should delete the target", func() {
			Expect(use.Delete(db)).To(BeNil())

			var count int64
			err := db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses WHERE id = $1`, use.ID)
			Expect(err).To(BeNil())
			Expect(count).To(BeZero())

			err = db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses WHERE id = $1`, use2.ID)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(int64(1)))
		})
	})
})
