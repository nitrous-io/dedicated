package hostscan_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
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

	doScanHost := func(scanTime time.Time) *hostscan.ScanResult {
		hostScans, errs := hostscan.ScanHosts(db, scanTime)
		Expect(len(errs)).To(Equal(0))
		Expect(len(hostScans)).To(Equal(1))

		hostScan := hostScans[0]
		Expect(hostScan.Host.ID).To(Equal(h.ID))
		return hostScan
	}

	expectNoHostFoundAt := func(scanTimeSinceCreated time.Duration) {
		hostScans, errs := hostscan.ScanHosts(db, h.CreatedAt.Add(scanTimeSinceCreated))
		Expect(len(errs)).To(Equal(0))
		Expect(len(hostScans)).To(Equal(0))
	}

	expectHostPassedAt := func(scanTimeSinceCreated time.Duration, expectedState string, update bool) {
		scan := doScanHost(h.CreatedAt.Add(scanTimeSinceCreated))
		Expect(scan.State).To(Equal(expectedState))

		if update {
			Expect(scan.UpdateHost(db)).To(BeNil())
		}
	}

	expectHostWarningAt := func(scanTimeSinceCreated time.Duration, update bool) {
		scan := doScanHost(h.CreatedAt.Add(scanTimeSinceCreated))
		Expect(scan.State).To(Equal(hostscan.StateOneWeekNotice))

		if update {
			Expect(scan.UpdateHost(db)).To(BeNil())
		}
	}

	expectHostTerminatingAt := func(scanTimeSinceCreated time.Duration, update bool) {
		scan := doScanHost(h.CreatedAt.Add(scanTimeSinceCreated))
		Expect(scan.State).To(Equal(hostscan.StateTerminating))

		if update {
			Expect(scan.UpdateHost(db)).To(BeNil())
		}
	}

	expectHostTerminatedAt := func(scanTimeSinceCreated time.Duration) {
		hosts, err := hostscan.FindTerminatingHosts(db, h.CreatedAt.Add(scanTimeSinceCreated))
		Expect(err).To(BeNil())
		Expect(len(hosts)).To(Equal(1))
		Expect(hosts[0].ID).To(Equal(h.ID))
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

	Describe("with free host", func() {
		BeforeEach(func() {
			_, err := db.Exec(`UPDATE billing_profiles
							SET state='free' WHERE user_id=$1;
							`, u.ID)
			Expect(err).To(BeNil())

			h = &host.Host{
				Slug:        "my-free-host",
				HourlyPrice: 0,
				Free:        true,
				State:       host.StateRunning,
			}

			_, err = test.CreateHostStack(db, &test.HostStack{
				H: h,
				U: u,
				P: freePlan,
			})
			Expect(err).To(BeNil())
		})

		Describe("when initially created with no usage", func() {
			It("should be listed in scannable hosts", func() {
				hostScans, errs := hostscan.ScanHosts(db, *h.CreatedAt)
				Expect(len(errs)).To(Equal(0))
				Expect(len(hostScans)).To(Equal(1))

				hostScan := hostScans[0]
				Expect(hostScan.Host.ID).To(Equal(h.ID))
			})

			It("should give warning and terminating result at respective time", func() {
				expectHostPassedAt(24*time.Hour, hostscan.StateGracePeriod, false)
				expectHostPassedAt(7*24*time.Hour, hostscan.StateGracePeriod, false)

				expectHostPassedAt(2*7*24*time.Hour, hostscan.StateGracePeriod, true)
				expectHostWarningAt(3*7*24*time.Hour, true)

				expectHostTerminatingAt(hostscan.HostScanPeriod-3*24*time.Hour, false)
				expectHostTerminatingAt(hostscan.HostScanPeriod-2*24*time.Hour, true)

				expectHostTerminatedAt(hostscan.HostScanPeriod + 1*24*time.Hour)
			})

			It("should skip when host is never scanned before", func() {
				expectHostPassedAt(24*time.Hour, hostscan.StateGracePeriod, false)
				expectHostPassedAt(7*24*time.Hour, hostscan.StateGracePeriod, false)
				expectHostPassedAt(2*7*24*time.Hour, hostscan.StateGracePeriod, false)

				expectHostPassedAt(3*7*24*time.Hour, hostscan.StateSkip, false)
				expectHostPassedAt(hostscan.HostScanPeriod-4*24*time.Hour, hostscan.StateSkip, false)
				expectHostPassedAt(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateSkip, false)
				expectHostPassedAt(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateSkip, false)
			})

			It("should skip warning and terminating state", func() {
				expectHostPassedAt(2*7*24*time.Hour, hostscan.StateGracePeriod, true)

				// on time
				expectHostWarningAt(3*7*24*time.Hour, false)

				// missed
				expectHostPassedAt((3*7+3)*24*time.Hour, hostscan.StateSkip, false)
				expectHostPassedAt((3*7+4)*24*time.Hour, hostscan.StateSkip, false)
				expectHostPassedAt(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateSkip, false)
				expectHostPassedAt(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateSkip, false)
			})

			It("should terminate or skip depending on scan time", func() {
				expectHostPassedAt(2*7*24*time.Hour, hostscan.StateGracePeriod, true)
				expectNoHostFoundAt((2*7 + 1) * 24 * time.Hour)
				expectNoHostFoundAt((2*7 + 2) * 24 * time.Hour)

				expectHostWarningAt(3*7*24*time.Hour, true)
				expectNoHostFoundAt((3*7 + 1) * 24 * time.Hour)

				// on time
				expectHostTerminatingAt(hostscan.HostScanPeriod-3*24*time.Hour, false)
				expectHostTerminatingAt(hostscan.HostScanPeriod-2*24*time.Hour, false)

				// missed
				expectHostPassedAt(hostscan.HostScanPeriod-1*24*time.Hour, hostscan.StateSkip, false)

				// on time and updated
				expectHostTerminatingAt(hostscan.HostScanPeriod-3*24*time.Hour, true)
				expectHostTerminatedAt(hostscan.HostScanPeriod)
			})

			It("should terminate after 4 weeks if previous db record is terminating", func() {
				scanTime := h.CreatedAt.Add(hostscan.HostScanPeriod - 3*24*time.Hour)
				h.NextScan = pq.NullTime{scanTime, true}
				hostscan.UpdateHostScanState(db, h)

				terminateTime := scanTime.Add(hostscan.TerminatingNoticePeriod)

				hostScan := doScanHost(scanTime)
				Expect(hostScan.State).To(Equal(hostscan.StateTerminating))
				Expect(hostScan.NextScan).To(BeTemporally("==", terminateTime, 24*time.Hour))

				Expect(hostScan.UpdateHost(db)).To(BeNil())

				expectHostTerminatedAt(hostscan.HostScanPeriod)
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
				expectHostPassedAt(5*24*time.Hour, hostscan.StateActive, false)
				expectHostPassedAt(7*24*time.Hour, hostscan.StateActive, false)
				expectHostPassedAt(2*7*24*time.Hour, hostscan.StateActive, false)
				expectHostPassedAt(3*7*24*time.Hour, hostscan.StateActive, false)
				expectHostPassedAt(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateActive, false)
				expectHostPassedAt(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateActive, false)
			})

			It("should reset after 4 weeks period", func() {
				expectHostPassedAt(hostscan.HostScanPeriod, hostscan.StateGracePeriod, false)
				expectHostPassedAt(hostscan.HostScanPeriod+24*time.Hour, hostscan.StateGracePeriod, false)
				expectHostPassedAt(hostscan.HostScanPeriod+7*24*time.Hour, hostscan.StateGracePeriod, false)
				expectHostPassedAt(hostscan.HostScanPeriod+2*7*24*time.Hour, hostscan.StateGracePeriod, false)

				expectHostPassedAt(hostscan.HostScanPeriod+2*7*24*time.Hour, hostscan.StateGracePeriod, true)
				expectHostWarningAt(hostscan.HostScanPeriod+3*7*24*time.Hour, true)

				expectHostTerminatingAt(2*hostscan.HostScanPeriod-3*24*time.Hour, false)
				expectHostTerminatingAt(2*hostscan.HostScanPeriod-2*24*time.Hour, true)
				expectHostTerminatedAt(2*hostscan.HostScanPeriod + 1*24*time.Hour)
			})
		})

		Describe("when free host is upgraded to paid plan", func() {
			BeforeEach(func() {
				// host was about to be terminated
				h.NextScan = pq.NullTime{}
				h.TerminatingAt = pq.NullTime{h.CreatedAt.Add(hostscan.HostScanPeriod), true}
				err := hostscan.UpdateHostScanState(db, h)
				Expect(err).To(BeNil())
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
				expectNoHostFoundAt(0 * time.Hour)
				expectNoHostFoundAt(24 * time.Hour)
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
			expectNoHostFoundAt(0 * time.Hour)
			expectNoHostFoundAt(24 * time.Hour)
		})
	})
})
