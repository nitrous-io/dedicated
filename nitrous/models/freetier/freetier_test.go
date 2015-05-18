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

			It("should be allowed to create free-tier host but in a queue", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
				Expect(err).To(Equal(freetier.ErrFreeQueueRequired))
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

					It("Should not be allowed to create new free host", func() {
						err := freetier.CreateHostWithPlanAllowed(db, u, freePlan)
						Expect(err).To(Equal(freetier.ErrMaxFreeHostReached))
					})

					It("Should not be allowed to create host with paid plan", func() {
						err := freetier.CreateHostWithPlanAllowed(db, u, paidPlan)
						Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
					})

					It("Should be allowed to create host in queue if terminated host is orphan", func() {
						_, err := db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2`, host.StateOrphan, h1.ID)
						Expect(err).To(BeNil())
						err = freetier.CreateHostWithPlanAllowed(db, u, freePlan)
						Expect(err).To(Equal(freetier.ErrFreeQueueRequired))
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

	Describe("StartHostAllowed()", func() {
		var (
			h1 *host.Host
		)
		BeforeEach(func() {
			h1 = &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 3000,
			}

			_, err := test.CreateHostStack(db, &test.HostStack{
				H: h1,
				U: u,
			})
			Expect(err).To(BeNil())
		})

		BeforeEach(func() {
			u.MaxRunningTime = sql.NullInt64{60, true}
			u.RefreshPeriod = sql.NullInt64{86400, true}
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2", billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())
		})

		It("should be nil if the user is in the paid tier", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2", billingprofile.StatePaid, u.ID)
			Expect(err).To(BeNil())

			Expect(freetier.StartHostAllowed(db, u, h1)).To(BeNil())
		})

		It("should be nil the user has no MaxRunningTime limit", func() {
			u.MaxRunningTime = sql.NullInt64{0, false}

			Expect(freetier.StartHostAllowed(db, u, h1)).To(BeNil())
		})

		It("should be nil the user has no RefreshPeriod limit", func() {
			u.RefreshPeriod = sql.NullInt64{0, false}

			Expect(freetier.StartHostAllowed(db, u, h1)).To(BeNil())
		})

		It("should be nil if the last running time is too old", func() {
			h1.LastRunningAt = pq.NullTime{time.Now().Add(-48 * time.Hour), true}

			Expect(freetier.StartHostAllowed(db, u, h1)).To(BeNil())
		})

		It("should not be exceeded if it was recently put into running", func() {
			h1.LastRunningAt = pq.NullTime{time.Now().Add(-30 * time.Second), true}

			Expect(freetier.StartHostAllowed(db, u, h1)).To(BeNil())
		})

		It("should be exceeded if it has been running for longer than allowed", func() {
			h1.LastRunningAt = pq.NullTime{time.Now().Add(-61 * time.Second), true}

			Expect(freetier.StartHostAllowed(db, u, h1)).To(Equal(freetier.ErrFreeTimeLimitExceeded))
		})
	})

	Describe("StopHostAllowed()", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2", billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())
		})

		It("should be true if the user is in the paid tier", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2", billingprofile.StatePaid, u.ID)
			Expect(err).To(BeNil())

			Expect(freetier.StopHostAllowed(db, u)).To(BeTrue())
		})

		It("should be false if the user is in the free tier", func() {
			Expect(freetier.StopHostAllowed(db, u)).To(BeFalse())
		})
	})

	Describe("ExpiredHosts()", func() {
		var (
			u2             *user.User
			h1, h2, h3, h4 *host.Host
		)

		BeforeEach(func() {
			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			h1 = &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 3000,
			}

			_, err := test.CreateHostStack(db, &test.HostStack{
				H: h1,
				U: u,
			})
			Expect(err).To(BeNil())

			h2 = &host.Host{
				Slug:               "my-host-2",
				HourlyPrice:        3000,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				State:              host.StateRunning,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h2.Insert(db)).To(BeNil())

			h3 = &host.Host{
				Slug:               "my-host-3",
				HourlyPrice:        3000,
				UserID:             u.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				State:              host.StateRunning,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h3.Insert(db)).To(BeNil())

			h4 = &host.Host{
				Slug:               "my-host-4",
				HourlyPrice:        3000,
				UserID:             u2.ID,
				PlanID:             h1.PlanID,
				RegionID:           h1.RegionID,
				State:              host.StateRunning,
				ProviderTemplateID: h1.ProviderTemplateID,
			}
			Expect(h4.Insert(db)).To(BeNil())
		})

		Context("when user is in free tier", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE users SET max_running_time = 60, refresh_period = 86400 WHERE id = $1", u.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE users SET max_running_time = 10, refresh_period = 86400 WHERE id = $1", u2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE billing_profiles SET state=$1 WHERE user_id IN ($2, $3)",
					billingprofile.StateFree, u.ID, u2.ID)
				Expect(err).To(BeNil())
			})

			It("should be empty if the hosts don't have any running times", func() {
				res, err := freetier.ExpiredHosts(db)
				Expect(err).To(BeNil())
				Expect(res).To(BeEmpty())
			})

			It("should include hosts that are past their max limit", func() {
				_, err := db.Exec("UPDATE hosts SET last_running_at = (now() - interval '61 seconds') WHERE id = $1 OR id = $2", h1.ID, h2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET last_running_at = (now() - interval '55 seconds') WHERE id = $1 OR id = $2", h3.ID, h4.ID)
				Expect(err).To(BeNil())

				res, err := freetier.ExpiredHosts(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(3))
				Expect([]int64{res[0].ID, res[1].ID, res[2].ID}).To(ConsistOf([]int64{h1.ID, h2.ID, h4.ID}))
			})

			It("should not include hosts that are not running", func() {
				_, err := db.Exec("UPDATE hosts SET last_running_at = (now() - interval '61 seconds') WHERE id = $1 OR id = $2", h1.ID, h2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET state = 'orphan' WHERE id = $1", h1.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET last_running_at = (now() - interval '55 seconds') WHERE id = $1 OR id = $2", h3.ID, h4.ID)
				Expect(err).To(BeNil())

				res, err := freetier.ExpiredHosts(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(2))
				Expect([]int64{res[0].ID, res[1].ID}).To(ConsistOf([]int64{h2.ID, h4.ID}))
			})

			It("should not include hosts that are pre-provisioned", func() {
				_, err := db.Exec("UPDATE hosts SET last_running_at = (now() - interval '61 seconds') WHERE id = $1 OR id = $2", h1.ID, h2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET preprovisioned = TRUE WHERE id = $1", h1.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET last_running_at = (now() - interval '55 seconds') WHERE id = $1 OR id = $2", h3.ID, h4.ID)
				Expect(err).To(BeNil())

				res, err := freetier.ExpiredHosts(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(2))
				Expect([]int64{res[0].ID, res[1].ID}).To(ConsistOf([]int64{h2.ID, h4.ID}))
			})
		})

		Context("when user is in paid tier", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE billing_profiles SET state=$1 WHERE user_id IN ($2, $3)",
					billingprofile.StatePaid, u.ID, u2.ID)
				Expect(err).To(BeNil())
			})

			It("should not include hosts whose user is not in free-tier", func() {
				// paid user hosts should not have last_running_at set. But we set here anyway
				// and make it the same condition as "should include hosts that are past their max limit"
				// just to be sure that paid users' hosts are never selected
				_, err := db.Exec("UPDATE hosts SET last_running_at = (now() - interval '61 seconds') WHERE id = $1 OR id = $2", h1.ID, h2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET last_running_at = (now() - interval '55 seconds') WHERE id = $1 OR id = $2", h3.ID, h4.ID)
				Expect(err).To(BeNil())

				res, err := freetier.ExpiredHosts(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(0))
			})
		})
	})
})
