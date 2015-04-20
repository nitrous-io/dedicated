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

	Describe("CreateHostWithPlanAllowed()", func() {
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

				It("should not be able to upgrade host", func() {
					_, err := freetier.UpgradeHostToPaid(db, u, h1)
					Expect(err).To(Equal(freetier.ErrPaymentDetailsRequired))
				})

				Context("when host is running", func() {
					It("free-tier user should only be allowed to create one free host", func() {
						hosts, err := host.FindAllByUser(db, u)
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

			It("Should be able to upgrade existing free host", func() {
				h1 := &host.Host{
					Slug:        "my-free-host",
					HourlyPrice: 0,
				}

				_, err := test.CreateHostStack(db, &test.HostStack{
					H: h1,
					U: u,
					P: freePlan,
				})
				Expect(err).To(BeNil())

				h2, err := freetier.UpgradeHostToPaid(db, u, h1)
				Expect(err).To(BeNil())

				Expect(h2.PlanID).To(Equal(paidPlan.ID))
				Expect(h2.HourlyPrice).To(Equal(paidPlan.HourlyPrice()))
			})

			It("Should not be able to upgrade paid host", func() {
				h1 := &host.Host{
					Slug:        "my-paid-host",
					HourlyPrice: 100,
				}

				_, err := test.CreateHostStack(db, &test.HostStack{
					H: h1,
					U: u,
					P: paidPlan,
				})
				Expect(err).To(BeNil())

				_, err = freetier.UpgradeHostToPaid(db, u, h1)
				Expect(err).To(Equal(freetier.ErrUpgradePaidNotAllowed))
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

			_, err = db.Exec("UPDATE users SET max_running_time = 60, refresh_period = 86400 WHERE id = $1", u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE users SET max_running_time = 10, refresh_period = 86400 WHERE id = $1", u2.ID)
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
})
