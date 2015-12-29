package subscription_test

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
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
				Slug:             "IAMASLUG90",
				MultiHost:        true,
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
				Slug:             "SLUG",
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
			sub.Slug = "NOSLUG"
			err := sub.Update(db)
			Expect(err).To(BeNil())

			sub2, err := subscription.Find(db, sub.ID)
			Expect(err).To(BeNil())
			Expect(sub2.Slug).To(Equal("SLUG"))
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
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())

			s2 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"X", true},
				Slug:             "SUB2",
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
				Slug:             "SUB3",
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

	Describe("FindBySlug", func() {
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
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())

			s2 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"X", true},
				Slug:             "SUB2",
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
				Slug:             "SUB3",
			}
			Expect(test.WrapInTransaction(db, s3.Insert)).To(BeNil())
		})

		It("should find by slug belonging to a user", func() {
			s, err := subscription.FindBySlug(db, "SUB1", u.ID)
			Expect(err).To(BeNil())
			Expect(s).To(Equal(s1))
		})

		It("should not find by slug if the subscription doesn't belong to the user", func() {
			s, err := subscription.FindBySlug(db, "SUB3", u.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(s).To(BeNil())
		})

		Context("with organisation", func() {
			var (
				owner  *user.User
				o      *org.Org
				orgSub *subscription.Subscription
				orgHS  *test.HostStack
			)

			BeforeEach(func() {
				owner = &user.User{
					Username: "boss",
					Email:    "boss@corp.com",
					Password: "123456",
				}
				Expect(test.WrapInTransaction(db, owner.Insert)).To(BeNil())
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(t *sqlx.Tx) error {
					Expect(billingprofile.Create(t, owner.ID, sql.NullInt64{o.ID, true})).To(BeNil())
					return nil
				})
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(owner.ID, 10), orgmembership.RoleOwner)
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				oBP, err := billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())

				orgSub = &subscription.Subscription{
					PlanID:           pl.ID,
					BillingProfileID: oBP.ID,
					State:            "active",
					StripeID:         sql.NullString{"orgSub", true},
					Slug:             "ORG-SUB1",
				}
				Expect(test.WrapInTransaction(db, orgSub.Insert)).To(BeNil())

				orgHS, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:        "my-org-host",
						HourlyPrice: 2000,
					},
					P: pl,
					U: u,
				})
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", orgSub.ID, orgHS.H.ID)
				Expect(err).To(BeNil())
			})

			It("should find if the subscription is tied to a org and host which belongs to a user", func() {
				s, err := subscription.FindBySlug(db, "ORG-SUB1", u.ID)
				Expect(err).To(BeNil())
				Expect(s).To(Equal(orgSub))
			})

			It("should not find if the subscription is tied to a org and host which belongs to a different user", func() {
				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
				Expect(s).To(BeNil())
			})

			It("should find if the user manages the billing profile", func() {
				_, err := db.Exec("UPDATE billing_profiles SET user_id = $1 WHERE org_id = $2", u2.ID, o.ID)
				Expect(err).To(BeNil())

				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(BeNil())
				Expect(s).To(Equal(orgSub))
			})

			It("should find if the subscription is tied to a org and host and the other user is an owner", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleOwner)
				Expect(err).To(BeNil())

				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(BeNil())
				Expect(s).To(Equal(orgSub))
			})

			It("should not find if the subscription is tied to a org and host and the other user is an owner of a different org", func() {
				o2 := &org.Org{
					Name:  "Foo Bar2, Inc.",
					Slug:  "foobar2",
					Email: "eng@foobar2.com",
				}
				err := o2.Insert(db)
				Expect(err).To(BeNil())

				_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleOwner)
				Expect(err).To(BeNil())

				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
				Expect(s).To(BeNil())
			})

			It("should not find if the subscription is tied to a org and host and the other user is an admin", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())

				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
				Expect(s).To(BeNil())
			})

			It("should find if the subscription is tied to a org and host and the other user is a member", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
				Expect(s).To(BeNil())
			})

			It("should find if the subscription is tied to a org and host and the other user is a limited user", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleLimited)
				Expect(err).To(BeNil())

				s, err := subscription.FindBySlug(db, "ORG-SUB1", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
				Expect(s).To(BeNil())
			})
		})
	})

	Describe("FindAllActiveForOrg()", func() {
		var (
			o      *org.Org
			orgSub *subscription.Subscription
		)

		BeforeEach(func() {
			bp1, err := u.BillingProfile(db)
			s := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, s.Insert)).To(BeNil())

			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err = o.Insert(db)
			Expect(err).To(BeNil())
			err = db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())

			oBP, err := billingprofile.FindForOrg(db, o.ID)
			Expect(err).To(BeNil())

			orgSub = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: oBP.ID,
				State:            "active",
				StripeID:         sql.NullString{"orgSub", true},
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, orgSub.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", orgSub.ID, hs.H.ID)
			Expect(err).To(BeNil())
		})

		It("should find subscriptions under the org", func() {
			subs, err := subscription.FindAllActiveForOrg(db, o.ID)
			Expect(err).To(BeNil())
			Expect(subs).To(HaveLen(1))
			Expect(subs[0].ID).To(Equal(orgSub.ID))
			Expect(subs[0].HostSlug.String).To(Equal("my-host-1"))
		})

		It("should not duplicate if subscription is multi host", func() {
			hs2, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 2000,
				},
				P: pl,
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", orgSub.ID, hs2.H.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE subscriptions SET multi_host = TRUE WHERE id = $1", orgSub.ID)
			Expect(err).To(BeNil())

			subs, err := subscription.FindAllActiveForOrg(db, o.ID)
			Expect(err).To(BeNil())
			Expect(subs).To(HaveLen(1))
			Expect(subs[0].ID).To(Equal(orgSub.ID))
			Expect(subs[0].HostSlug.Valid).To(BeFalse())
		})
	})

	Describe("FindAllActiveByUserID", func() {
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
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())

			s2 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"X", true},
				Slug:             "SUB2",
			}
			Expect(test.WrapInTransaction(db, s2.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s2.ID, hs.H.ID)
			Expect(err).To(BeNil())

			s3 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "cancelled",
				StripeID:         sql.NullString{"2", true},
				Slug:             "SUB3",
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
				Slug:             "SUB4",
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

		It("should not duplicate if subscription is multi host", func() {
			hs2, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 2000,
				},
				P: pl,
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s2.ID, hs2.H.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE subscriptions SET multi_host = TRUE WHERE id = $1", s2.ID)
			Expect(err).To(BeNil())

			subscriptions, err := subscription.FindAllActiveByUserID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(subscriptions).To(HaveLen(2))
			Expect(subscriptions[0].ID).To(Equal(s1.ID))
			Expect(subscriptions[0].HostSlug.Valid).To(BeFalse())

			Expect(subscriptions[1].ID).To(Equal(s2.ID))
			Expect(subscriptions[1].HostSlug.Valid).To(BeFalse())
		})

		Context("with organisation", func() {
			var (
				owner  *user.User
				o      *org.Org
				orgSub *subscription.Subscription
				orgHS  *test.HostStack
			)

			BeforeEach(func() {
				owner = &user.User{
					Username: "boss",
					Email:    "boss@corp.com",
					Password: "123456",
				}
				Expect(test.WrapInTransaction(db, owner.Insert)).To(BeNil())
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(t *sqlx.Tx) error {
					Expect(billingprofile.Create(t, owner.ID, sql.NullInt64{o.ID, true})).To(BeNil())
					return nil
				})
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(owner.ID, 10), orgmembership.RoleOwner)
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				oBP, err := billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())

				orgSub = &subscription.Subscription{
					PlanID:           pl.ID,
					BillingProfileID: oBP.ID,
					State:            "active",
					StripeID:         sql.NullString{"orgSub", true},
					Slug:             "SUB1",
				}
				Expect(test.WrapInTransaction(db, orgSub.Insert)).To(BeNil())

				orgHS, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:        "my-org-host",
						HourlyPrice: 2000,
					},
					P: pl,
					U: u,
				})
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", orgSub.ID, orgHS.H.ID)
				Expect(err).To(BeNil())
			})

			It("should return subscriptions for which the user's hosts belongs to in an org", func() {
				subscriptions, err := subscription.FindAllActiveByUserID(db, u.ID)
				Expect(err).To(BeNil())

				Expect(subscriptions).To(HaveLen(3))
				Expect(subscriptions[2].ID).To(Equal(orgSub.ID))
				Expect(subscriptions[2].HostSlug.String).To(Equal("my-org-host"))
				Expect(subscriptions[2].OrgID.Int64).To(Equal(o.ID))
			})

			It("should not double up subscriptions if the host and org BP belong to the same user", func() {
				_, err := db.Exec("UPDATE billing_profiles SET user_id = $1 WHERE org_id = $2;", u.ID, o.ID)
				Expect(err).To(BeNil())

				subscriptions, err := subscription.FindAllActiveByUserID(db, u.ID)
				Expect(err).To(BeNil())

				Expect(subscriptions).To(HaveLen(3))
				Expect(subscriptions[2].ID).To(Equal(orgSub.ID))
				Expect(subscriptions[2].HostSlug.String).To(Equal("my-org-host"))
				Expect(subscriptions[2].OrgID.Int64).To(Equal(o.ID))
			})
		})
	})

	Describe("FindAllTrialEnding", func() {
		BeforeEach(func() {
			u2 := &user.User{
				Username: "u2",
				Email:    "u2@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			u3 := &user.User{
				Username: "u3",
				Email:    "u3@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, u3.Insert)).To(BeNil())

			bp1, err := u.BillingProfile(db)
			Expect(err).To(BeNil())
			s1 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp1.ID,
				State:            "trialing",
				StripeID:         sql.NullString{"a", true},
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())
			_, err = db.Exec(`UPDATE subscriptions SET trial_end=$1 WHERE billing_profile_id=$2;`, time.Now().Add((2*24+12)*time.Hour), bp1.ID)
			Expect(err).To(BeNil())

			bp2, err := u2.BillingProfile(db)
			Expect(err).To(BeNil())
			s2 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp2.ID,
				State:            "trialing",
				StripeID:         sql.NullString{"b", true},
				Slug:             "SUB2",
			}
			Expect(test.WrapInTransaction(db, s2.Insert)).To(BeNil())
			_, err = db.Exec(`UPDATE subscriptions SET trial_end=$1 WHERE billing_profile_id=$2;`, time.Now().Add((1*24+2)*time.Hour), bp2.ID)
			Expect(err).To(BeNil())

			bp3, err := u3.BillingProfile(db)
			Expect(err).To(BeNil())
			s3 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp3.ID,
				State:            "trialing",
				StripeID:         sql.NullString{"c", true},
				Slug:             "SUB3",
			}
			Expect(test.WrapInTransaction(db, s3.Insert)).To(BeNil())
			_, err = db.Exec(`UPDATE subscriptions SET trial_end=$1 WHERE billing_profile_id=$2;`, time.Now().Add(15*time.Hour), bp3.ID)
			Expect(err).To(BeNil())
		})

		It("returns subscriptions ending between 1 to 3 days", func() {
			subscriptions, err := subscription.FindAllTrialEnding(db, 1*24, 3*24)
			Expect(err).To(BeNil())

			Expect(subscriptions).To(HaveLen(2))
		})

		It("returns subscriptions ending in less than 1 day", func() {
			subscriptions, err := subscription.FindAllTrialEnding(db, 0, 1*24)
			Expect(err).To(BeNil())

			Expect(subscriptions).To(HaveLen(1))
			Expect(subscriptions[0].Slug).To(Equal("SUB3"))

			// not if they are deleted (trial cancelled)
			_, err = db.Exec(`UPDATE subscriptions SET state = 'deleted' WHERE slug = $1`, "SUB3")
			Expect(err).To(BeNil())

			subscriptions, err = subscription.FindAllTrialEnding(db, 0, 1*24)
			Expect(err).To(BeNil())
			Expect(subscriptions).To(HaveLen(0))
		})

	})

	Describe("WithinFreeTrial()", func() {
		It("returns true if TrialEnd is valid and current time is before TrialEnd time", func() {
			s := &subscription.Subscription{
				TrialEnd: pq.NullTime{time.Now().Add(24 * time.Hour), true},
			}
			Expect(s.WithinFreeTrial()).To(BeTrue())
		})

		It("returns false if TrialEnd is valid but current time is after TrialEnd time", func() {
			s := &subscription.Subscription{
				TrialEnd: pq.NullTime{time.Unix(1, 0), true},
			}
			Expect(s.WithinFreeTrial()).To(BeFalse())
		})

		It("returns false if TrialEnd is not valid", func() {
			s := &subscription.Subscription{
				TrialEnd: pq.NullTime{time.Now().Add(24 * time.Hour), false},
			}
			Expect(s.WithinFreeTrial()).To(BeFalse())
		})
	})

	Describe("FindMultiHostSubscription", func() {
		var (
			s1  *subscription.Subscription
			s3  *subscription.Subscription
			u2  *user.User
			bp  *billingprofile.BillingProfile
			bp2 *billingprofile.BillingProfile
		)

		BeforeEach(func() {
			// Insert subscriptions
			var err error
			bp, err = u.BillingProfile(db)
			Expect(err).To(BeNil())
			s1 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
				Slug:             "SUB1",
				MultiHost:        true,
			}
			Expect(test.WrapInTransaction(db, s1.Insert)).To(BeNil())

			s2 := &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"X", true},
				Slug:             "SUB2",
			}
			Expect(test.WrapInTransaction(db, s2.Insert)).To(BeNil())

			u2 = &user.User{
				Username: "user2",
				Email:    "user2@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			bp2, err = u2.BillingProfile(db)
			Expect(err).To(BeNil())
			s3 = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp2.ID,
				State:            "active",
				StripeID:         sql.NullString{"3", true},
				Slug:             "SUB3",
			}
			Expect(test.WrapInTransaction(db, s3.Insert)).To(BeNil())
		})

		It("should find the multi host subscription", func() {
			s, err := subscription.FindMultiHostSubscription(db, bp.ID)
			Expect(err).To(BeNil())
			Expect(s).To(Equal(s1))
		})

		It("should not find a multi host subscription if one doesn't exist for the BP", func() {
			s, err := subscription.FindMultiHostSubscription(db, bp2.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(s).To(BeNil())
		})

		It("should not find a cancelled multi host subscription", func() {
			_, err := db.Exec("UPDATE subscriptions SET cancelled_at = now() WHERE billing_profile_id = $1", bp.ID)
			Expect(err).To(BeNil())
			s, err := subscription.FindMultiHostSubscription(db, bp.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(s).To(BeNil())
		})
	})
})
