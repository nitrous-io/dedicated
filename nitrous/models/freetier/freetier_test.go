package freetier_test

import (
	"database/sql"
	"testing"

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
})
