package freetier_test

import (
	"testing"
	"time"

	"github.com/jinzhu/now"
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

func insertHostTransition(db *db.Database, hostID int64, prevState, state string, start, end time.Time) error {
	_, err := db.Exec(`INSERT INTO host_transitions 
		(host_id, previous_state, state, created_at, ended_at) 
		VALUES ($1, $2, $3, $4, $5);`,
		hostID, prevState, state, start, end)

	return err
}

var _ = Describe("models/freetier", func() {
	var (
		db                 *db.Database
		u                  *user.User
		h1                 *host.Host
		paidPlan, freePlan *plan.Plan
		bp                 *billingprofile.BillingProfile

		// Stub the current time to be somewhere around middle of current month
		rightNow = now.BeginningOfMonth().Add(15 * 24 * time.Hour)
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

		freePlan = &plan.Plan{
			Slug:          "free plan",
			Price:         0,
			MaxContainers: 1,
		}

		h1 = &host.Host{
			Slug:        "my-host-1",
			HourlyPrice: 3000,
		}

		_, err = CreateHostStack(db, &HostStack{
			H: h1,
			U: u,
			P: freePlan,
		})
		Expect(err).To(BeNil())

		paidPlan = &plan.Plan{
			Slug:          "paid plan",
			Price:         100,
			MaxContainers: 2,
		}

		Expect(paidPlan.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Context("When user is in free tier", func() {
		BeforeEach(func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state=$1, max_hours=10 WHERE user_id=$2;`,
				billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
		})

		Context("when host is running", func() {
			It("free-tier user should only be allowed to create one free host", func() {
				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(1))

				err = freetier.CreateHostAllowed(db, bp, freePlan, rightNow)
				Expect(err).ToNot(BeNil())
			})

			It("Should not be allowed to create host with paid plan", func() {
				err := freetier.CreateHostAllowed(db, bp, paidPlan, rightNow)
				Expect(err).ToNot(BeNil())
			})
		})

		Context("when host is terminated", func() {
			BeforeEach(func() {
				err := h1.Delete(db)
				Expect(err).To(BeNil())
				Expect(h1.TerminatedAt).ToNot(BeNil())
			})

			It("Should be allowed to create new free host", func() {
				err := freetier.CreateHostAllowed(db, bp, freePlan, rightNow)
				Expect(err).To(BeNil())
			})

			It("Should not be allowed to create host with paid plan", func() {
				err := freetier.CreateHostAllowed(db, bp, paidPlan, rightNow)
				Expect(err).ToNot(BeNil())
			})

			It("Should be allowed if previous host did not exceed free time quota", func() {
				err := insertHostTransition(db, h1.ID, "born", "running",
					rightNow.Add(-5*time.Hour),
					rightNow)
				Expect(err).To(BeNil())

				err = freetier.CreateHostAllowed(db, bp, freePlan, rightNow)
				Expect(err).To(BeNil())
			})

			It("Should not be allowed if max free time exceeded", func() {
				err := insertHostTransition(db, h1.ID, "born", "running",
					rightNow.Add(-11*time.Hour),
					rightNow)
				Expect(err).To(BeNil())

				err = freetier.CreateHostAllowed(db, bp, freePlan, rightNow)
				Expect(err).ToNot(BeNil())
			})
		})

		Context("when host is stopped", func() {
			BeforeEach(func() {
				h1.State = host.StateStopped
				Expect(h1.Update(db)).To(BeNil())
			})

			It("Should allow start if there is free time remaining", func() {
				err := insertHostTransition(db, h1.ID, "born", "running",
					rightNow.Add(-10*time.Hour),
					rightNow.Add(-5*time.Hour))
				Expect(err).To(BeNil())

				err = freetier.StartHostAllowed(db, bp, rightNow)
				Expect(err).To(BeNil())
			})

			It("Should disallow start if current month free time exceeded", func() {
				err := insertHostTransition(db, h1.ID, "born", "running",
					rightNow.Add(-26*time.Hour),
					rightNow.Add(-5*time.Hour))
				Expect(err).To(BeNil())

				err = freetier.StartHostAllowed(db, bp, rightNow)
				Expect(err).ToNot(BeNil())
			})

			It("Should allow start when the the exceeded free time is in a previous month", func() {
				err := insertHostTransition(db, h1.ID, "born", "running",
					now.BeginningOfMonth().Add(-15*time.Hour),
					now.BeginningOfMonth().Add(5*time.Hour))
				Expect(err).To(BeNil())

				err = freetier.StartHostAllowed(db, bp, rightNow)
				Expect(err).To(BeNil())
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
			err := freetier.CreateHostAllowed(db, bp, freePlan, rightNow)
			Expect(err).ToNot(BeNil())
		})
	})
})
