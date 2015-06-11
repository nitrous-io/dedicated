package freetier_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/freetier")
}

func insertHostTransition(db *db.Database, hostID int64, prevState, state string, start, end pq.NullTime) error {
	_, err := db.Exec(`INSERT INTO host_transitions
		(host_id, previous_state, state, created_at, ended_at)
		VALUES ($1, $2, $3, $4, $5);`,
		hostID, prevState, state, start, end)

	return err
}

var _ = Describe("models/freetier", func() {
	var (
		db *db.Database
		u  *user.User
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
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Context("with free and paid plans", func() {
		var (
			paidPlan, freePlan *plan.Plan
		)

		BeforeEach(func() {
			paidPlan = &plan.Plan{
				Slug:          "paid plan",
				Price:         100,
				MaxContainers: 2,
			}
			Expect(paidPlan.Insert(db)).To(BeNil())

			freePlan = &plan.Plan{
				Slug:          "free plan",
				Price:         0,
				MaxContainers: 1,
				PaidPlanID:    sql.NullInt64{Valid: true, Int64: paidPlan.ID},
			}
			Expect(freePlan.Insert(db)).To(BeNil())
		})

		Describe("UpgradeFreeUserHosts()", func() {
			var h *host.Host

			BeforeEach(func() {
				h = &host.Host{
					Slug:          "my-free-host",
					HourlyPrice:   0,
					Free:          true,
					MaxContainers: sql.NullInt64{1, true},
				}

				_, err := test.CreateHostStack(db, &test.HostStack{
					H: h,
					U: u,
					P: freePlan,
				})
				Expect(err).To(BeNil())

				Expect(h.UserID).To(Equal(u.ID))
				Expect(h.PlanID).To(Equal(freePlan.ID))
				Expect(h.HourlyPrice).To(Equal(int64(0)))
				Expect(h.Free).To(BeTrue())
				Expect(h.MaxContainers).To(Equal(sql.NullInt64{1, true}))
				Expect(h.UpgradedAt.Valid).To(BeFalse())
			})

			It("should not allow for an upgrade if there is no equivalent paid plan", func() {
				_, err := db.Exec(`UPDATE plans SET paid_plan_id = NULL WHERE id=$1;`, h.PlanID)
				Expect(err).To(BeNil())

				err = freetier.UpgradeFreeUserHosts(db, u)
				Expect(err).To(Equal(freetier.ErrUpgradeNotAvailable))

				h2, err := host.FindByIdentifier(db, h.Slug)
				Expect(err).To(BeNil())
				Expect(h2.UserID).To(Equal(u.ID))
				Expect(h2.PlanID).To(Equal(freePlan.ID))
				Expect(h2.HourlyPrice).To(Equal(int64(0)))
				Expect(h2.Free).To(BeTrue())
				Expect(h2.MaxContainers).To(Equal(sql.NullInt64{1, true}))
				Expect(h2.UpgradedAt.Valid).To(BeFalse())
			})

			It("should upgrade the host properties", func() {
				err := freetier.UpgradeFreeUserHosts(db, u)
				Expect(err).To(BeNil())

				h2, err := host.FindByIdentifier(db, h.Slug)
				Expect(err).To(BeNil())

				Expect(h2.UserID).To(Equal(u.ID))
				Expect(h2.PlanID).To(Equal(paidPlan.ID))
				Expect(h2.HourlyPrice).To(Equal(int64(paidPlan.HourlyPrice())))
				Expect(h2.Free).To(BeFalse())
				Expect(h2.MaxContainers).To(Equal(sql.NullInt64{2, true}))
				Expect(h2.UpgradedAt.Valid).To(BeTrue())
			})
		})

		Context("When user is in free tier and phone authenticated", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state=$1, max_hours=10 WHERE user_id=$2;`,
					billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())

				err = u.UpdatePhone(db, "005550006", "15")
				Expect(err).To(BeNil())

				err = u.UpdatePhoneAuthed(db, true)
				Expect(err).To(BeNil())
			})

			It("should be allowed to create free-tier host", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
				Expect(err).To(BeNil())
			})

			It("should not be allowed to create host with paid plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, paidPlan)
				Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
			})

			Context("When there is host created", func() {
				var (
					h1 *host.Host
				)
				BeforeEach(func() {
					h1 = &host.Host{
						Slug:        "my-free-host",
						HourlyPrice: 0,
						Free:        true,
					}

					_, err := test.CreateHostStack(db, &test.HostStack{
						H: h1,
						U: u,
						P: freePlan,
					})
					Expect(err).To(BeNil())
				})

				Context("when host is running", func() {
					It("free-tier user should only be allowed to create one free host", func() {
						hosts, err := host.FindAllByUser(db, u.ID)
						Expect(err).To(BeNil())
						Expect(len(hosts)).To(Equal(1))

						err = freetier.CreateHostWithPlanAllowed(db, u, freePlan)
						Expect(err).To(Equal(freetier.ErrMaxFreeHostReached))
					})
				})

				Context("when host is terminated", func() {
					BeforeEach(func() {
						err := h1.Delete(db)
						Expect(err).To(BeNil())
						Expect(h1.TerminatedAt).ToNot(BeNil())
					})

					It("should be allowed to create new free host", func() {
						err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
						Expect(err).To(BeNil())
					})

					It("should not be allowed to create host with paid plan", func() {
						err := freetier.CreateHostWithPlanAllowed(db, u, paidPlan)
						Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
					})

					It("Should be allowed to create host in queue if terminated host is orphan", func() {
						_, err := db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2`, host.StateOrphan, h1.ID)
						Expect(err).To(BeNil())
						err = freetier.CreateHostWithPlanAllowed(db, u, freePlan)
						Expect(err).To(BeNil())
					})
				})
			})
		})

		Context("When user is in free tier without phone verification", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state=$1, max_hours=10 WHERE user_id=$2;`,
					billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())
			})

			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
				Expect(err).To(Equal(freetier.ErrPhoneAuthRequired))
			})
		})

		Context("When user is in paid tier", func() {
			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})
		})

		Context("When admin user is in paid tier", func() {
			It("Should be allowed to create host with free plan", func() {
				u.Admin = true
				err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
				u.Admin = false
				Expect(err).To(BeNil())
			})
		})

	})

	Describe("EligiblePlansForUser()", func() {
		plans := []*plan.Plan{
			{Slug: "free", Price: 0},
			{Slug: "start", Price: 500},
			{Slug: "shift", Price: 1000},
			{Slug: "turbo", Price: 1500},
			{Slug: "hidden", Price: 2000, Disabled: true},
		}

		BeforeEach(func() {
			r := region.Region{
				Slug: "sfo",
				Name: "US West",
			}
			Expect(r.Insert(db)).To(BeNil())

			for _, pl := range plans {
				Expect(pl.Insert(db)).To(BeNil())
				pt := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: r.ID,
					Provider: envvars.DefaultProvider,
				}
				Expect(pt.Insert(db)).To(BeNil())
			}

			pl := plan.Plan{Slug: "start-windows-azure", Price: 3000}
			Expect(pl.Insert(db)).To(BeNil())
			pt := providertemplate.ProviderTemplate{
				PlanID:   pl.ID,
				RegionID: r.ID,
				Provider: "windows-azure",
			}
			Expect(pt.Insert(db)).To(BeNil())
		})

		It("should return all plans if the user has no card and hasn't been authed", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateNoCard, u.ID)
			Expect(err).To(BeNil())

			u.PhoneAuthed = false

			pls, err := freetier.EligiblePlansForUser(db, u, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[0], plans[1], plans[2], plans[3]}))
		})

		It("should hide plans if disabled", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateNoCard, u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE plans SET disabled=TRUE WHERE id=$1;`, plans[0].ID)
			Expect(err).To(BeNil())

			u.PhoneAuthed = false

			pls, err := freetier.EligiblePlansForUser(db, u, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})

		It("should return only paid plans if the user has no card and has been authed", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateNoCard, u.ID)
			Expect(err).To(BeNil())

			u.PhoneAuthed = true

			pls, err := freetier.EligiblePlansForUser(db, u, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})

		It("should return free and paid plans if the user is in free card state", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())

			pls, err := freetier.EligiblePlansForUser(db, u, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[0], plans[1], plans[2], plans[3]}))
		})

		It("should return only paid plans if the user is paid", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StatePaid, u.ID)
			Expect(err).To(BeNil())

			pls, err := freetier.EligiblePlansForUser(db, u, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})

		It("should only return paid plans if the user has free host", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())

			u.PhoneAuthed = true

			h1 := &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 3000,
				State:       host.StateRunning,
			}

			_, err = test.CreateHostStack(db, &test.HostStack{
				H: h1,
				U: u,
				P: plans[1],
			})
			Expect(err).To(BeNil())

			pls, err := freetier.EligiblePlansForUser(db, u, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})
	})

	Describe("IdleFreeHosts()", func() {
		var (
			h1, h2, h3, h4, h5, h6, h7, h8 *host.Host
			t1, t2, t3, t4                 time.Time
		)

		BeforeEach(func() {
			t1 = time.Now()
			t2 = t1.Add(-freetier.IdleHostTimeout)
			t3 = t2.Add(1 * time.Minute)
			t4 = t2.Add(-1 * time.Minute)

			h1 = &host.Host{
				Slug:         "my-host-1",
				State:        host.StateRunning,
				LastPingedAt: pq.NullTime{Valid: false},
				AutoShutdown: false,
			}

			_, err := test.CreateHostStack(db, &test.HostStack{
				H: h1,
				U: u,
			})
			Expect(err).To(BeNil())

			h2 = &host.Host{
				Slug:               "my-host-2",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t1, true},
				AutoShutdown:       false,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h2.Insert(db)).To(BeNil())

			h3 = &host.Host{
				Slug:               "my-host-3",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t1, true},
				AutoShutdown:       true,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h3.Insert(db)).To(BeNil())

			h4 = &host.Host{
				Slug:               "my-host-4",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       true,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h4.Insert(db)).To(BeNil())

			h5 = &host.Host{
				Slug:               "my-host-5",
				State:              host.StateStopped,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       true,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h5.Insert(db)).To(BeNil())

			h6 = &host.Host{
				Slug:               "my-host-6",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       false,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h6.Insert(db)).To(BeNil())

			h7 = &host.Host{
				Slug:               "my-host-7",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t3, true},
				AutoShutdown:       true,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h7.Insert(db)).To(BeNil())

			h8 = &host.Host{
				Slug:               "my-host-8",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t4, true},
				AutoShutdown:       true,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h8.Insert(db)).To(BeNil())
		})

		It("returns a list of idle hosts that are eligible for auto shutdown", func() {
			res, err := freetier.IdleFreeHosts(db)
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(2))
			Expect([]int64{res[0].ID, res[1].ID}).To(Equal([]int64{h8.ID, h4.ID}))
		})

		Context("when db query fails with error", func() {
			BeforeEach(func() {
				db.Close()
			})

			It("returns error", func() {
				_, err := freetier.IdleFreeHosts(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
