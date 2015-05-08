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
				hostScan := doScanHost(h.CreatedAt.Add(HostScanPeriod - 3*24*time.Hour))
				Expect(hostScan.State).To(Equal(StateTerminating))
				Expect(hostScan.NextScan).To(BeTemporally("==", h.CreatedAt.Add(HostScanPeriod), 24*time.Hour))

				h.NextScan = pq.NullTime{}
				h.TerminatingAt = pq.NullTime{hostScan.NextScan, true}
				err := UpdateHostScanState(db, h)
				Expect(err).To(BeNil())

				hosts, err := FindTerminatingHosts(db, h.CreatedAt.Add(HostScanPeriod))
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(1))
				Expect(hosts[0].ID).To(Equal(h.ID))
			})
		})

		Describe("with 4 times monthly usage", func() {
			BeforeEach(func() {
				// Start host the first four days
				runningTimes := []time.Time{
					h.CreatedAt.Add(1 * 24 * time.Hour),
					h.CreatedAt.Add(2 * 24 * time.Hour),
					h.CreatedAt.Add(3 * 24 * time.Hour),
					h.CreatedAt.Add(4 * 24 * time.Hour),
				}
				insertHostTransitions(runningTimes)
			})

			It("should pass with active state within 4 weeks period", func() {
				expectScanStateAtTimeEquals(5*24*time.Hour, StateActive)
				expectScanStateAtTimeEquals(7*24*time.Hour, StateActive)
				expectScanStateAtTimeEquals(2*7*24*time.Hour, StateActive)
				expectScanStateAtTimeEquals(3*7*24*time.Hour, StateActive)
				expectScanStateAtTimeEquals(HostScanPeriod-3*24*time.Hour, StateActive)
				expectScanStateAtTimeEquals(HostScanPeriod-2*24*time.Hour, StateActive)
			})

			It("should reset after 4 weeks period", func() {
				expectScanStateAtTimeEquals(HostScanPeriod, StateGracePeriod)
				expectScanStateAtTimeEquals(HostScanPeriod+24*time.Hour, StateGracePeriod)
				expectScanStateAtTimeEquals(HostScanPeriod+7*24*time.Hour, StateGracePeriod)
				expectScanStateAtTimeEquals(HostScanPeriod+2*7*24*time.Hour, StateGracePeriod)
				expectScanStateAtTimeEquals(HostScanPeriod+3*7*24*time.Hour, StateOneWeekNotice)
				expectScanStateAtTimeEquals(2*HostScanPeriod-3*24*time.Hour, StateTerminating)
				expectScanStateAtTimeEquals(2*HostScanPeriod+HostScanPeriod-2*24*time.Hour, StateTerminating)
			})
		})

		Describe("when free host is upgraded to paid plan", func() {
			BeforeEach(func() {
				// host was about to be terminated
				h.NextScan = pq.NullTime{}
				h.TerminatingAt = pq.NullTime{h.CreatedAt.Add(HostScanPeriod), true}
				err := UpdateHostScanState(db, h)
				Expect(err).To(BeNil())

				// originally can be terminated
				hosts, err := FindTerminatingHosts(db, h.CreatedAt.Add(HostScanPeriod))
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
				hosts, err := FindTerminatingHosts(db, h.CreatedAt.Add(HostScanPeriod))
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
