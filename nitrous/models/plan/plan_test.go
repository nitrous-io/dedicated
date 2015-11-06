package plan_test

import (
	"database/sql"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/plan_segment"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func mapPlansToSlugs(plans []*plan.Plan) []string {
	slugs := make([]string, len(plans))
	for i, plan := range plans {
		slugs[i] = plan.Slug
	}
	return slugs
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/plan")
}

var _ = Describe("models/plan", func() {
	var (
		db *ndb.Database
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec(`INSERT INTO plans
			(slug, name, max_containers, price, base_price, cpu, memory, storage, disabled, most_popular)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`,
			"free", "FreeToTry", 4, 0, 0, "2", "1 GB", "20 GB", false, true)
		Expect(err).To(BeNil())

		_, err = db.Exec(`INSERT INTO plans
			(slug, name, max_containers, price, base_price, cpu, memory, storage, disabled, most_popular, stripe_plan_id, org)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);`,
			"1gb", "Micro", 4, 1995, 500, "2", "1 GB", "20 GB", false, true, "Sxcvv67SDx", true)
		Expect(err).To(BeNil())

		_, err = db.Exec(`INSERT INTO plans
			(slug, name, max_containers, price, base_price, cpu, memory, storage, disabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
			"2gb", "Small", 8, 3995, 900, "4", "2 GB", "30 GB", false)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByID()", func() {
		var id int64

		BeforeEach(func() {
			err := db.QueryRow(`SELECT id FROM plans WHERE slug = $1;`, "1gb").Scan(&id)
			Expect(err).To(BeNil())
			Expect(id).NotTo(BeZero())
		})

		Context("when a plan record with a given id exists", func() {
			It("returns the plan record", func() {
				p, err := plan.FindByID(db, id)
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).To(Equal(id))
			})
		})

		Context("when a plan record with a given id does not exist", func() {
			It("returns error", func() {
				p, err := plan.FindByID(db, 9001)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(p).To(BeNil())
			})
		})
	})

	Describe("FindBySlug()", func() {
		Context("when a plan record with a given slug exists", func() {
			It("returns the plan record", func() {
				p, err := plan.FindBySlug(db, "1gb")
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).NotTo(BeZero())
				Expect(p.Slug).To(Equal("1gb"))
				Expect(p.Name).To(Equal("Micro"))
				Expect(p.MaxContainers).To(Equal(int64(4)))
				Expect(p.Price).To(Equal(int64(1995)))
				Expect(p.BasePrice).To(Equal(int64(500)))
				Expect(p.CPU).To(Equal("2"))
				Expect(p.Memory).To(Equal("1 GB"))
				Expect(p.Storage).To(Equal("20 GB"))
				Expect(p.MostPopular).To(BeTrue())
				Expect(p.StripePlanID.String).To(Equal("Sxcvv67SDx"))
				Expect(p.Org).To(BeTrue())
			})
		})

		Context("when a plan record with a given slug exists but is disabled", func() {
			It("returns an error", func() {
				_, err := db.Exec(`INSERT INTO plans
					(slug, name, max_containers, price, cpu, memory, storage, disabled)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`,
					"promo", "Promotion", 4, 1995, "2", "1 GB", "20 GB", true)
				Expect(err).To(BeNil())

				p, err := plan.FindBySlug(db, "promo")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(p).To(BeNil())
			})
		})

		Context("when a plan record with a given slug does not exist", func() {
			It("returns error", func() {
				p, err := plan.FindBySlug(db, "non-existent-slug")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(p).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				p, err := plan.FindBySlug(db, "1gb")
				Expect(p).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByStripePlanID()", func() {
		var id int64

		BeforeEach(func() {
			err := db.QueryRow(`SELECT id FROM plans WHERE slug = $1;`, "1gb").Scan(&id)
			Expect(err).To(BeNil())
			Expect(id).NotTo(BeZero())
		})

		Context("when a plan record with a given id exists", func() {
			It("returns the plan record", func() {
				p, err := plan.FindByStripePlanID(db, "Sxcvv67SDx")
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).To(Equal(id))
			})
		})

		Context("when a plan record with a given id does not exist", func() {
			It("returns error", func() {
				p, err := plan.FindByStripePlanID(db, "nothingToSeeHere")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(p).To(BeNil())
			})
		})
	})

	Describe("PlanPricingHostMap()", func() {
		var (
			u        *user.User
			hs1, hs2 *test.HostStack
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			hs1, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				P: &plan.Plan{
					Slug:          "hs1Plan",
					Name:          "hs1Plan",
					MaxContainers: 1,
					Price:         1901,
					BasePrice:     500,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", hs1.H.ID)
			Expect(err).To(BeNil())

			hs2, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 500,
				},
				P: &plan.Plan{
					Slug:          "hs2Plan",
					Name:          "hs2Plan",
					MaxContainers: 1,
					Price:         2016,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		It("returns a mapping of host IDs to plan prices for a user", func() {
			m, err := plan.PlanPricingHostMap(db, u.ID)
			Expect(err).To(BeNil())
			exMap := map[int64]plan.PlanPricing{
				hs1.H.ID: plan.PlanPricing{Price: 1901, BasePrice: 500},
				hs2.H.ID: plan.PlanPricing{Price: 2016, BasePrice: 0},
			}
			Expect(m).To(Equal(exMap))
		})

		It("excludes subscription hosts from the pricing map", func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           hs1.P.ID,
				BillingProfileID: bp.ID,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", sub.ID, hs1.H.ID)
			Expect(err).To(BeNil())

			m, err := plan.PlanPricingHostMap(db, u.ID)
			Expect(err).To(BeNil())
			exMap := map[int64]plan.PlanPricing{
				hs2.H.ID: plan.PlanPricing{Price: 2016, BasePrice: 0},
			}
			Expect(m).To(Equal(exMap))
		})
	})

	Describe("PlanPricingHostMapWithSubscription()", func() {
		var (
			u        *user.User
			hs1, hs2 *test.HostStack
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			hs1, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				P: &plan.Plan{
					Slug:          "hs1Plan",
					Name:          "hs1Plan",
					MaxContainers: 1,
					Price:         1901,
					BasePrice:     500,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", hs1.H.ID)
			Expect(err).To(BeNil())

			hs2, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 500,
				},
				P: &plan.Plan{
					Slug:          "hs2Plan",
					Name:          "hs2Plan",
					MaxContainers: 1,
					Price:         2016,
				},
				U: u,
			})

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           hs1.P.ID,
				BillingProfileID: bp.ID,
				Slug:             "SLUG",
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", sub.ID, hs1.H.ID)
			Expect(err).To(BeNil())
		})

		It("returns a mapping of host IDs to plan prices for a user", func() {
			m, err := plan.PlanPricingHostMapWithSubscription(db, u.ID)
			Expect(err).To(BeNil())
			exMap := map[int64]plan.PlanPricing{
				hs1.H.ID: plan.PlanPricing{Price: 1901, BasePrice: 500},
				hs2.H.ID: plan.PlanPricing{Price: 2016, BasePrice: 0},
			}
			Expect(m).To(Equal(exMap))
		})
	})

	Describe("Insert()", func() {
		var p *plan.Plan

		BeforeEach(func() {
			p = &plan.Plan{
				Slug:          "512mb",
				Name:          "Nano",
				MaxContainers: 2,
				Price:         995,
				CPU:           "1",
				Memory:        "512 MB",
				Storage:       "20 GB",
				MostPopular:   true,
				Disabled:      false,
				Hidden:        true,
				AutoShutdown:  true,
			}
			Expect(p.Insert(db)).To(BeNil())
		})

		It("persists the plan in the database", func() {
			p2, err := plan.FindBySlug(db, "512mb")
			Expect(err).To(BeNil())
			Expect(p2).NotTo(BeNil())
			Expect(p2.ID).NotTo(BeZero())
			Expect(p2.BasePlanID.Valid).To(BeFalse())
			Expect(p2.PaidPlanID.Valid).To(BeFalse())
			Expect(p2.Slug).To(Equal("512mb"))
			Expect(p2.Name).To(Equal("Nano"))
			Expect(p2.MaxContainers).To(Equal(int64(2)))
			Expect(p2.Price).To(Equal(int64(995)))
			Expect(p2.CPU).To(Equal("1"))
			Expect(p2.Memory).To(Equal("512 MB"))
			Expect(p2.Storage).To(Equal("20 GB"))
			Expect(p2.MostPopular).To(BeTrue())
			Expect(p2.Disabled).To(BeFalse())
			Expect(p2.Hidden).To(BeTrue())
			Expect(p2.AutoShutdown).To(BeTrue())
			Expect(p2.CreatedAt).NotTo(BeNil())
			Expect(p2.UpdatedAt).NotTo(BeNil())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(p.ID).NotTo(BeZero())
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("HourlyPrice()", func() {
		It("rounds up to nearest milicent", func() {
			p := &plan.Plan{Price: 2500}
			Expect(p.HourlyPrice()).To(Equal(int64(3721)))

			p = &plan.Plan{Price: 4500}
			Expect(p.HourlyPrice()).To(Equal(int64(6697)))

			p = &plan.Plan{Price: 8000}
			Expect(p.HourlyPrice()).To(Equal(int64(11905)))
		})
	})

	Describe("CanUpgradeToPlan()", func() {
		It("should be false if the target plan has a lesser price", func() {
			p := &plan.Plan{Price: 2500}
			p2 := &plan.Plan{Price: 200}
			Expect(p.CanUpgradeToPlan(p2)).To(BeFalse())
		})

		It("should be true if the target plan has a higher price", func() {
			p := &plan.Plan{Price: 2500}
			p2 := &plan.Plan{Price: 2900}
			Expect(p.CanUpgradeToPlan(p2)).To(BeTrue())
		})

		It("should be false if the target plan has the same price", func() {
			p := &plan.Plan{Price: 2500}
			p2 := &plan.Plan{Price: 2500}
			Expect(p.CanUpgradeToPlan(p2)).To(BeFalse())
		})

		It("should be false if current plan is org plan and target plan is personal plan", func() {
			p := &plan.Plan{Price: 2000, Org: true}
			p2 := &plan.Plan{Price: 2500}
			Expect(p.CanUpgradeToPlan(p2)).To(BeFalse())
		})

		It("should be false if current plan is personal plan and target plan is org plan", func() {
			p := &plan.Plan{Price: 2000}
			p2 := &plan.Plan{Price: 2500, Org: true}
			Expect(p.CanUpgradeToPlan(p2)).To(BeFalse())
		})

		It("should be true both current plan and target plan are org plans", func() {
			p := &plan.Plan{Price: 2000, Org: true}
			p2 := &plan.Plan{Price: 2500, Org: true}
			Expect(p.CanUpgradeToPlan(p2)).To(BeTrue())
		})
	})

	Describe("PlansFor", func() {
		var (
			u                        *user.User
			userDefaultSegmentID     int64
			freeUserDefaultSegmentID int64
		)

		plans := []*plan.Plan{
			{Slug: "free"},
			{Slug: "1gb"},
			{Slug: "2gb"},
		}

		BeforeEach(func() {
			// Get default segment ID
			err := db.QueryRow(`SELECT id from segments where user_default='t'`).Scan(&userDefaultSegmentID)
			Expect(err).NotTo(HaveOccurred())

			err = db.QueryRow(`SELECT id from segments where free_user_default='t'`).Scan(&freeUserDefaultSegmentID)
			Expect(err).NotTo(HaveOccurred())

			for _, p := range plans {
				pl, err := plan.FindBySlug(db, p.Slug)
				Expect(err).NotTo(HaveOccurred())

				// Add plan segments
				if pl.Slug != "free" {
					ps := plan_segment.PlanSegment{SegmentID: userDefaultSegmentID, PlanID: pl.ID}
					Expect(ps.Insert(db)).NotTo(HaveOccurred())
				}
				ps := plan_segment.PlanSegment{SegmentID: freeUserDefaultSegmentID, PlanID: pl.ID}
				Expect(ps.Insert(db)).NotTo(HaveOccurred())
			}

			u = &user.User{
				Username:  "foo",
				Email:     "foo@example.com",
				Password:  "123456",
				SegmentID: freeUserDefaultSegmentID,
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			u.SegmentID = freeUserDefaultSegmentID
			Expect(u.Update(db)).NotTo(HaveOccurred())
		})

		It("returns default plans if user is nil", func() {
			plans, err := plan.PlansFor(db, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(plans).To(HaveLen(2))
			Expect(plans[0].Slug).To(Equal("1gb"))
			Expect(plans[1].Slug).To(Equal("2gb"))
		})

		It("returns user plans if user is not nil", func() {
			plans, err := plan.PlansFor(db, u)
			Expect(err).NotTo(HaveOccurred())

			Expect(plans).To(HaveLen(3))
			Expect(plans[0].Slug).To(Equal("free"))
			Expect(plans[1].Slug).To(Equal("1gb"))
			Expect(plans[2].Slug).To(Equal("2gb"))
		})
	})
})
