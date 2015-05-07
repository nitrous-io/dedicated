package hostscan

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
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
	RunSpecs(t, "models/hostscan")
}

var _ = Describe("models/hostscan", func() {
	var (
		db                 *db.Database
		u                  *user.User
		h                  *host.Host
		paidPlan, freePlan *plan.Plan
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

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

	Describe("with free host", func() {
		BeforeEach(func() {
			_, err := db.Exec(`UPDATE billing_profiles
				SET state='free' WHERE user_id=$1;
				`, u.ID)
			Expect(err).To(BeNil())

			h = &host.Host{
				Slug:        "my-free-host",
				HourlyPrice: 0,
				State:       host.StateRunning,
			}

			_, err = test.CreateHostStack(db, &test.HostStack{
				H: h,
				U: u,
				P: freePlan,
			})
			Expect(err).To(BeNil())
		})

		doScanHost := func(scanTime time.Time) *ScanResult {
			hostScans, errs := ScanHosts(db, scanTime)
			Expect(len(errs)).To(Equal(0))
			Expect(len(hostScans)).To(Equal(1))

			hostScan := hostScans[0]
			Expect(hostScan.Host.ID).To(Equal(h.ID))
			return hostScan
		}

		expectScanStateAtTimeEquals := func(scanTimeSinceCreated time.Duration, expectedState string) {
			Expect(doScanHost(h.CreatedAt.Add(scanTimeSinceCreated)).State).To(Equal(expectedState))
		}

		insertHostTransitions := func(runningTimes []time.Time) {
			for _, runningTime := range runningTimes {
				_, err := db.Exec(`INSERT INTO host_transitions
					(host_id, previous_state, state, created_at, ended_at)
					VALUES ($1, $2, $3, $4, $5);`,
					h.ID, "stopped", "running", runningTime, runningTime.Add(2*time.Hour))
				Expect(err).To(BeNil())
			}
		}

		Describe("when initially created with no usage", func() {
			It("should be listed in scannable hosts", func() {
				scannedHosts, err := FindScannableHosts(db, *h.CreatedAt)
				Expect(err).To(BeNil())

				Expect(len(scannedHosts)).To(Equal(1))

				scannedHost := scannedHosts[0]
				Expect(scannedHost.ID).To(Equal(h.ID))
			})

			It("should terminate after 4 weeks if previous db record is terminating", func() {
				hostScan := doScanHost(h.CreatedAt.Add(fourWeeks - 3*oneDay))
				Expect(hostScan.State).To(Equal(StateTerminating))
				Expect(hostScan.NextScan).To(BeTemporally("==", h.CreatedAt.Add(fourWeeks), oneDay))

				h.NextScan = pq.NullTime{}
				h.TerminatingAt = pq.NullTime{hostScan.NextScan, true}
				err := UpdateHostScanState(db, h)
				Expect(err).To(BeNil())

				hosts, err := FindTerminatingHosts(db, h.CreatedAt.Add(fourWeeks))
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(1))
				Expect(hosts[0].ID).To(Equal(h.ID))
			})
		})

		Describe("with 4 times monthly usage", func() {
			BeforeEach(func() {
				// Start host the first four days
				runningTimes := []time.Time{
					h.CreatedAt.Add(1 * oneDay),
					h.CreatedAt.Add(2 * oneDay),
					h.CreatedAt.Add(3 * oneDay),
					h.CreatedAt.Add(4 * oneDay),
				}
				insertHostTransitions(runningTimes)
			})

			It("should pass with active state within 4 weeks period", func() {
				expectScanStateAtTimeEquals(5*oneDay, StateActive)
				expectScanStateAtTimeEquals(oneWeek, StateActive)
				expectScanStateAtTimeEquals(2*oneWeek, StateActive)
				expectScanStateAtTimeEquals(3*oneWeek, StateActive)
				expectScanStateAtTimeEquals(fourWeeks-3*oneDay, StateActive)
				expectScanStateAtTimeEquals(fourWeeks-2*oneDay, StateActive)
			})

			It("should reset after 4 weeks period", func() {
				expectScanStateAtTimeEquals(fourWeeks, StateGracePeriod)
				expectScanStateAtTimeEquals(fourWeeks+oneDay, StateGracePeriod)
				expectScanStateAtTimeEquals(fourWeeks+oneWeek, StateGracePeriod)
				expectScanStateAtTimeEquals(fourWeeks+2*oneWeek, StateGracePeriod)
				expectScanStateAtTimeEquals(fourWeeks+3*oneWeek, StateOneWeekNotice)
				expectScanStateAtTimeEquals(fourWeeks+fourWeeks-3*oneDay, StateTerminating)
				expectScanStateAtTimeEquals(fourWeeks+fourWeeks-2*oneDay, StateTerminating)
			})
		})

		Describe("when free host is upgraded to paid plan", func() {
			BeforeEach(func() {
				// host was about to be terminated
				h.NextScan = pq.NullTime{}
				h.TerminatingAt = pq.NullTime{h.CreatedAt.Add(fourWeeks), true}
				err := UpdateHostScanState(db, h)
				Expect(err).To(BeNil())

				// originally can be terminated
				hosts, err := FindTerminatingHosts(db, h.CreatedAt.Add(fourWeeks))
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(1))
			})

			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles
						SET state='paid' WHERE user_id=$1;
						`, u.ID)
				Expect(err).To(BeNil())

				err = freetier.UpgradeFreeUserHosts(db, u)
				Expect(err).To(BeNil())
			})

			It("should not be listed in scannable hosts", func() {
				scannedHosts, err := FindScannableHosts(db, *h.CreatedAt)
				Expect(err).To(BeNil())

				Expect(len(scannedHosts)).To(Equal(0))
			})

			It("should not be listed in terminating hosts", func() {
				hosts, err := FindTerminatingHosts(db, h.CreatedAt.Add(fourWeeks))
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(0))
			})
		})
	})

	Describe("with paid host", func() {
		BeforeEach(func() {
			h = &host.Host{
				Slug:        "my-paid-host",
				HourlyPrice: 100,
				State:       host.StateRunning,
			}

			_, err := test.CreateHostStack(db, &test.HostStack{
				H: h,
				U: u,
				P: freePlan,
			})
			Expect(err).To(BeNil())
		})

		It("should not be listed in scannable hosts", func() {
			scanTime := *h.CreatedAt
			scannedHosts, err := FindScannableHosts(db, scanTime)
			Expect(err).To(BeNil())

			Expect(len(scannedHosts)).To(Equal(0))
		})
	})
})
