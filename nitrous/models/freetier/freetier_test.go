package freetier_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
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
		bp *billingprofile.BillingProfile
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

		bp, err = u.BillingProfile(db)
		Expect(err).To(BeNil())
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

		Context("When user is in free tier and phone authenticated", func() {
			BeforeEach(func() {
				bp.State = billingprofile.StateFree
				bp.MaxHours = sql.NullInt64{10, true}
				Expect(bp.Update(db)).To(BeNil())

				err := u.UpdatePhone(db, "005550006", "15")
				Expect(err).To(BeNil())

				err = u.UpdatePhoneAuthed(db, true)
				Expect(err).To(BeNil())
			})

			It("should be allowed to create free-tier host", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
				Expect(err).To(BeNil())
			})

			It("should not be allowed to create host with paid plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, paidPlan)
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

						err = freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
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
						err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
						Expect(err).To(BeNil())
					})

					It("should not be allowed to create host with paid plan", func() {
						err := freetier.CreateHostWithPlanAllowed(db, u, bp, paidPlan)
						Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
					})

					It("Should be allowed to create host in queue if terminated host is orphan", func() {
						_, err := db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2`, host.StateOrphan, h1.ID)
						Expect(err).To(BeNil())
						err = freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
						Expect(err).To(BeNil())
					})
				})
			})
		})

		Context("for an Org", func() {
			BeforeEach(func() {
				bp.OrgID = sql.NullInt64{20, true}
			})

			It("should not allow a free plan even if the BP is in a free tier", func() {
				bp.State = billingprofile.StateFree
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})

			It("should not allow a free plan even if the BP is not in a free state", func() {
				bp.State = billingprofile.StatePaid
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})
		})

		Context("When user is in free tier without phone verification", func() {
			BeforeEach(func() {
				bp.State = billingprofile.StateFree
				bp.MaxHours = sql.NullInt64{10, true}
				Expect(bp.Update(db)).To(BeNil())
			})

			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
				Expect(err).To(Equal(freetier.ErrPhoneAuthRequired))
			})
		})

		Context("When user is in paid tier", func() {
			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
			})
		})

		Context("When admin user is in paid tier", func() {
			It("Should be allowed to create host with free plan", func() {
				u.Admin = true
				err := freetier.CreateHostWithPlanAllowed(db, u, bp, freePlan)
				u.Admin = false
				Expect(err).To(BeNil())
			})
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

			// host which was started but has never been pinged (no websockets
			// support, user didn't open IDE, etc)
			h9 = &host.Host{
				Slug:               "my-host-9",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{Valid: false},
				AutoShutdown:       true,
				UserID:             u.ID,
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
			hs, err = test.CreateHostStack(db, &test.HostStack{U: u})
			Expect(err).To(BeNil())
		})

		It("returns true when host has been idle", func() {
			h := &host.Host{
				Slug:               "my-host-1",
				State:              host.StateRunning,
				LastPingedAt:       pq.NullTime{t2, true},
				AutoShutdown:       true,
				UserID:             u.ID,
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
				UserID:             u.ID,
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
				UserID:             u.ID,
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
				UserID:             u.ID,
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
