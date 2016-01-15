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
		db         *db.Database
		u1, u2     *user.User
		bp1, bp2   *billingprofile.BillingProfile
		asiaRegion *region.Region
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u1 = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		Expect(test.WrapInTransaction(db, u1.Insert)).To(BeNil())

		u2 = &user.User{
			Username: "bar",
			Email:    "bar@example.com",
			Password: "123456",
		}
		Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

		bp1, err = u1.BillingProfile(db)
		Expect(err).To(BeNil())

		bp2, err = u2.BillingProfile(db)
		Expect(err).To(BeNil())

		asiaRegion = &region.Region{
			Slug: "asia-slug",
			Name: "Southeast Asia",
		}
		Expect(asiaRegion.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("CreateHostWithPlanAllowed()", func() {
		var (
			paidPlan, freePlan *plan.Plan
		)

		BeforeEach(func() {
			paidPlan = &plan.Plan{
				Slug:          "paid plan",
				Price:         100,
				MaxContainers: 2,
				AutoShutdown:  false,
			}
			Expect(paidPlan.Insert(db)).To(BeNil())

			freePlan = &plan.Plan{
				Slug:          "free plan",
				Price:         0,
				MaxContainers: 1,
				AutoShutdown:  true,
			}
			Expect(freePlan.Insert(db)).To(BeNil())
		})

		Context("When free tier and phone authenticated user create a host", func() {
			var (
				h1 *host.Host
			)

			BeforeEach(func() {
				bp1.State = billingprofile.StateFree
				bp1.MaxHours = sql.NullInt64{10, true}
				Expect(bp1.Update(db)).To(BeNil())

				err := u1.UpdatePhone(db, "005550006", "15")
				Expect(err).To(BeNil())

				err = u1.UpdatePhoneAuthed(db, true)
				Expect(err).To(BeNil())

				bp2.State = billingprofile.StateFree
				bp2.MaxHours = sql.NullInt64{10, true}
				Expect(bp2.Update(db)).To(BeNil())

				err = u2.UpdatePhone(db, "115550006", "25")
				Expect(err).To(BeNil())

				err = u2.UpdatePhoneAuthed(db, true)
				Expect(err).To(BeNil())

				h1 := &host.Host{
					Slug:        "my-host-1",
					State:       host.StateRunning,
					HourlyPrice: 0,
					Free:        true,
				}
				_, err := test.CreateHostStack(db, &test.HostStack{
					H: h1,
					U: u1,
					R: asiaRegion,
					P: freePlan,
				})
				Expect(err).To(BeNil())
			})

			It("returns no error", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(BeNil())
			})

			It("returns an error when user choose paid plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, paidPlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
			})

			It("returns an error when selected region has exceeded daily quota", func() {
				h2 := &host.Host{
					Slug:               "my-host-2",
					State:              host.StateRunning,
					UserID:             u1.ID,
					PlanID:             freePlan.ID,
					RegionID:           asiaRegion.ID,
					ProviderTemplateID: h1.ProviderTemplateID,
				}
				Expect(h2.Insert(db)).To(BeNil())

				h3 := &host.Host{
					Slug:               "my-host-3",
					State:              host.StateRunning,
					UserID:             u1.ID,
					PlanID:             freePlan.ID,
					RegionID:           asiaRegion.ID,
					ProviderTemplateID: h1.ProviderTemplateID,
				}
				Expect(h3.Insert(db)).To(BeNil())

				h4 := &host.Host{
					Slug:               "my-host-4",
					State:              host.StateRunning,
					UserID:             u1.ID,
					PlanID:             freePlan.ID,
					RegionID:           asiaRegion.ID,
					ProviderTemplateID: h1.ProviderTemplateID,
				}
				Expect(h4.Insert(db)).To(BeNil())

				err = freetier.CreateHostWithPlanAllowed(db, u2, bp2, freePlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrFreeDailyHostLimitExceeded))
			})

			It("returns an when user already has a free host", func() {
				hosts, err := host.FindAllByUser(db, u1.ID)
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(1))

				err = freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrMaxFreeHostReached))
			})

			It("returns no error when user has an orphaned terminated host", func() {
				err = h1.Delete(db)
				Expect(err).To(BeNil())
				Expect(h1.TerminatedAt).ToNot(BeNil())

				_, err := db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2`, host.StateOrphan, h1.ID)
				Expect(err).To(BeNil())
				err = freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(BeNil())
			})
		})

		Context("for an Org", func() {
			BeforeEach(func() {
				bp1.OrgID = sql.NullInt64{20, true}
			})

			It("should not allow a free plan even if the BP is in a free tier", func() {
				bp1.State = billingprofile.StateFree
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})

			It("should not allow a free plan even if the BP is not in a free state", func() {
				bp1.State = billingprofile.StatePaid
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})
		})

		Context("When user is in free tier without phone verification", func() {
			BeforeEach(func() {
				bp1.State = billingprofile.StateFree
				bp1.MaxHours = sql.NullInt64{10, true}
				Expect(bp1.Update(db)).To(BeNil())
			})

			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrPhoneAuthRequired))
			})
		})

		Context("When user is in paid tier", func() {
			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})
		})

		Context("When admin user is in paid tier", func() {
			It("Should be allowed to create host with free plan", func() {
				u1.Admin = true
				err := freetier.CreateHostWithPlanAllowed(db, u1, bp1, freePlan, asiaRegion)
				u1.Admin = false
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
			bp1.State = billingprofile.StateNoCard
			Expect(bp1.Update(db)).To(BeNil())

			u1.PhoneAuthed = false

			pls, err := freetier.EligiblePlansForUser(db, u1, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[0], plans[1], plans[2], plans[3]}))
		})

		It("should hide plans if disabled", func() {
			bp1.State = billingprofile.StateNoCard
			Expect(bp1.Update(db)).To(BeNil())

			_, err := db.Exec(`UPDATE plans SET disabled=TRUE WHERE id=$1;`, plans[0].ID)
			Expect(err).To(BeNil())

			u1.PhoneAuthed = false

			pls, err := freetier.EligiblePlansForUser(db, u1, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})

		It("should return only paid plans if the user has no card and has been authed", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateNoCard, u1.ID)
			Expect(err).To(BeNil())

			u1.PhoneAuthed = true

			pls, err := freetier.EligiblePlansForUser(db, u1, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})

		It("should return free and paid plans if the user is in free card state", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateFree, u1.ID)
			Expect(err).To(BeNil())

			pls, err := freetier.EligiblePlansForUser(db, u1, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[0], plans[1], plans[2], plans[3]}))
		})

		It("should return only paid plans if the user is paid", func() {
			bp1.State = billingprofile.StatePaid
			Expect(bp1.Update(db)).To(BeNil())

			pls, err := freetier.EligiblePlansForUser(db, u1, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})

		It("should only return paid plans if the user has free host", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1 WHERE user_id=$2;`, billingprofile.StateFree, u1.ID)
			Expect(err).To(BeNil())

			u1.PhoneAuthed = true

			h1 := &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 3000,
				State:       host.StateRunning,
			}

			_, err = test.CreateHostStack(db, &test.HostStack{
				H: h1,
				U: u1,
				P: plans[1],
			})
			Expect(err).To(BeNil())

			pls, err := freetier.EligiblePlansForUser(db, u1, envvars.DefaultProvider)
			Expect(err).To(BeNil())
			Expect(pls).To(ConsistOf([]*plan.Plan{plans[1], plans[2], plans[3]}))
		})
	})

	Describe("IdleFreeHosts()", func() {
		var (
			h1, h2, h3, h4, h5, h6, h7, h8, h9 *host.Host
			t1, t2, t3, t4                     time.Time
		)

		BeforeEach(func() {
			t1 = time.Now()
			t2 = t1.Add(-900 * time.Second) // 900s = idle_timeout DEFAULT
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
				U: u1,
			})
			Expect(err).To(BeNil())

			h2 = &host.Host{
				Slug:               "my-host-2",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t1, true},
				AutoShutdown:       false,
				UserID:             u1.ID,
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
				UserID:             u1.ID,
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
				UserID:             u1.ID,
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
				UserID:             u1.ID,
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
				UserID:             u1.ID,
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
				UserID:             u1.ID,
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
				UserID:             u1.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h8.Insert(db)).To(BeNil())

			// host which was started but has never been pinged (no websockets
			// support, user didn't open IDE, etc)
			h9 = &host.Host{
				Slug:               "my-host-9",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{Valid: false},
				AutoShutdown:       true,
				UserID:             u1.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h9.Insert(db)).To(BeNil())
		})

		It("returns a list of idle hosts that are eligible for auto shutdown", func() {
			res, err := freetier.IdleFreeHosts(db)
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(3))
			Expect([]int64{res[0].ID, res[1].ID, res[2].ID}).To(Equal([]int64{h8.ID, h4.ID, h9.ID}))

			// sanity-check that SQL query populates Provider field (required by hostcontrol)
			Expect(res[0].Provider).To(Equal("docker"))
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

	Describe("IsIdle()", func() {
		var (
			hs         *test.HostStack
			t1, t2, t3 time.Time
		)

		BeforeEach(func() {
			t1 = time.Now()
			t2 = t1.Add(-900 * time.Second) // 900s = idle_timeout DEFAULT
			t3 = t2.Add(1 * time.Minute)

			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{U: u1})
			Expect(err).To(BeNil())
		})

		It("returns true when host has been idle", func() {
			h := &host.Host{
				Slug:               "my-host-1",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       true,
				UserID:             u1.ID,
				PlanID:             hs.H.PlanID,
				RegionID:           hs.H.RegionID,
				ProviderTemplateID: hs.H.ProviderTemplateID,
			}
			Expect(h.Insert(db)).To(BeNil())

			idle, err := freetier.IsIdle(db, h)
			Expect(err).To(BeNil())
			Expect(idle).To(BeTrue())

			// even if last_pinged_at is not set
			_, err = db.Exec(`UPDATE hosts SET last_pinged_at = NULL WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())
			idle, err = freetier.IsIdle(db, h)
			Expect(err).To(BeNil())
			Expect(idle).To(BeTrue())

			// unless the host has been running for less than 45 minutes
			_, err = db.Exec(`UPDATE hosts SET last_running_at = now() - interval '43 minutes' WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())

			idle, err = freetier.IsIdle(db, h)
			Expect(err).To(BeNil())
			Expect(idle).To(BeFalse())
		})

		It("returns false if host does have the auto_shutdown=true", func() {
			h := &host.Host{
				Slug:               "my-host-1",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       false,
				UserID:             u1.ID,
				PlanID:             hs.H.PlanID,
				RegionID:           hs.H.RegionID,
				ProviderTemplateID: hs.H.ProviderTemplateID,
			}
			Expect(h.Insert(db)).To(BeNil())

			idle, err := freetier.IsIdle(db, h)
			Expect(err).To(BeNil())
			Expect(idle).To(BeFalse())
		})

		It("returns false if host is not running", func() {
			h := &host.Host{
				Slug:               "my-host-1",
				State:              host.StateStopped,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       true,
				UserID:             u1.ID,
				PlanID:             hs.H.PlanID,
				RegionID:           hs.H.RegionID,
				ProviderTemplateID: hs.H.ProviderTemplateID,
			}
			Expect(h.Insert(db)).To(BeNil())

			idle, err := freetier.IsIdle(db, h)
			Expect(err).To(BeNil())
			Expect(idle).To(BeFalse())
		})

		It("returns false if it has been pinged within its idle_timeout", func() {
			h := &host.Host{
				Slug:               "my-host-1",
				State:              host.StateStopped,
				LastPingedAt:       pq.NullTime{t3, true},
				AutoShutdown:       true,
				UserID:             u1.ID,
				PlanID:             hs.H.PlanID,
				RegionID:           hs.H.RegionID,
				ProviderTemplateID: hs.H.ProviderTemplateID,
			}
			Expect(h.Insert(db)).To(BeNil())

			idle, err := freetier.IsIdle(db, h)
			Expect(err).To(BeNil())
			Expect(idle).To(BeFalse())
		})
	})
})
