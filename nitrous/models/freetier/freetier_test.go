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
	. "github.com/nitrous-io/dedicated/nitrous/test"
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

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		Expect(WrapInTransaction(db, u.Insert)).To(BeNil())
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
			freePlan = &plan.Plan{
				Slug:          "free plan",
				Price:         0,
				MaxContainers: 1,
			}
			Expect(freePlan.Insert(db)).To(BeNil())

			paidPlan = &plan.Plan{
				Slug:          "paid plan",
				Price:         100,
				MaxContainers: 2,
			}

			Expect(paidPlan.Insert(db)).To(BeNil())
		})

		Context("When user is in free tier", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state=$1, max_hours=10 WHERE user_id=$2;`,
					billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())

				bp, err = billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
			})

			It("should be allowed to create free-tier host", func() {
				err := freetier.CreateHostWithPlanAllowed(db, bp, freePlan)
				Expect(err).To(BeNil())
			})

			It("should not be allowed to create host with paid plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, bp, paidPlan)
				Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
			})

			Context("When there is host created", func() {
				var (
					h1 *host.Host
				)
				BeforeEach(func() {
					h1 = &host.Host{
						Slug:        "my-host-1",
						HourlyPrice: 3000,
					}

					_, err := CreateHostStack(db, &HostStack{
						H: h1,
						U: u,
						P: freePlan,
					})
					Expect(err).To(BeNil())
				})

				Context("when host is running", func() {
					It("free-tier user should only be allowed to create one free host", func() {
						hosts, err := host.FindAllByUser(db, u)
						Expect(err).To(BeNil())
						Expect(len(hosts)).To(Equal(1))

						err = freetier.CreateHostWithPlanAllowed(db, bp, freePlan)
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
						err := freetier.CreateHostWithPlanAllowed(db, bp, freePlan)
						Expect(err).To(Equal(freetier.ErrMaxFreeHostReached))
					})

					It("Should not be allowed to create host with paid plan", func() {
						err := freetier.CreateHostWithPlanAllowed(db, bp, paidPlan)
						Expect(err).To(Equal(freetier.ErrPaidPlanNotAllowed))
					})
				})
			})
		})

		Context("When user is in paid tier", func() {
			BeforeEach(func() {
				var err error
				bp, err = billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
			})

			It("Should not be allowed to create host with free plan", func() {
				err := freetier.CreateHostWithPlanAllowed(db, bp, freePlan)
				Expect(err).To(Equal(freetier.ErrFreePlanNotAllowed))
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

			_, err := CreateHostStack(db, &HostStack{
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
})
