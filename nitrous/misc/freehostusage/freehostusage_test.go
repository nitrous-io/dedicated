package freehostusage_test

import (
	"database/sql"
	"github.com/lib/pq"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/misc/freehostusage"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "misc/freehostusage")
}

var _ = Describe("misc/freehostusage", func() {
	var (
		db                 *db.Database
		u                  *user.User
		h                  *host.Host
		paidPlan, freePlan *plan.Plan

		logr = &logger.Logger{Level: logger.LogLevelQuiet}
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

	scanAll := func(scanTime time.Time) *freehostusage.MockAction {
		action := &freehostusage.MockAction{}
		freehostusage.ScanAllFreeHosts(db, logr, action, scanTime, false)
		return action
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

		doScanHost := func(scanTime time.Time) *hostscan.ScanResult {
			mock := scanAll(scanTime)

			Expect(len(mock.UpdatedScans)).To(Equal(1))
			Expect(len(mock.RefreshedHosts)).To(Equal(0))
			Expect(len(mock.TerminatedHosts)).To(Equal(0))

			return mock.UpdatedScans[0]
		}

		expectScanStateAtTimeEquals := func(scanTimeSinceCreated time.Duration, expectedState string) {
			scan := doScanHost(h.CreatedAt.Add(scanTimeSinceCreated))
			Expect(scan.State).To(Equal(expectedState))
		}

		expectAndUpdateScanState := func(scanTimeSinceCreated time.Duration, expectedState string) {
			scan := doScanHost(h.CreatedAt.Add(scanTimeSinceCreated))
			Expect(scan.State).To(Equal(expectedState))
			Expect(scan.UpdateHost(db)).To(BeNil())
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
				mock := scanAll(*h.CreatedAt)

				Expect(len(mock.UpdatedScans)).To(Equal(1))
				Expect(len(mock.RefreshedHosts)).To(Equal(0))
				Expect(len(mock.TerminatedHosts)).To(Equal(0))

				Expect(mock.UpdatedScans[0].Host.ID).To(Equal(h.ID))
			})

			Describe("when host is scanned on time after next_scan", func() {
				It("should give warning and terminating result at respective time", func() {
					expectScanStateAtTimeEquals(24*time.Hour, hostscan.StateGracePeriod)
					expectScanStateAtTimeEquals(7*24*time.Hour, hostscan.StateGracePeriod)

					expectAndUpdateScanState(2*7*24*time.Hour, hostscan.StateGracePeriod)
					expectAndUpdateScanState(3*7*24*time.Hour, hostscan.StateOneWeekNotice)

					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateTerminating)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateTerminating)
				})
			})

			Describe("when host is never scanned before", func() {
				It("should skip warning and terminating state", func() {
					expectScanStateAtTimeEquals(24*time.Hour, hostscan.StateGracePeriod)
					expectScanStateAtTimeEquals(7*24*time.Hour, hostscan.StateGracePeriod)
					expectScanStateAtTimeEquals(2*7*24*time.Hour, hostscan.StateGracePeriod)

					expectScanStateAtTimeEquals(3*7*24*time.Hour, hostscan.StateSkip)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-4*24*time.Hour, hostscan.StateSkip)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateSkip)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateSkip)
				})
			})

			Describe("when host is scanned too late after next_scan", func() {
				It("should skip warning and terminating state", func() {
					expectAndUpdateScanState(2*7*24*time.Hour, hostscan.StateGracePeriod)

					// on time
					expectScanStateAtTimeEquals(3*7*24*time.Hour, hostscan.StateOneWeekNotice)

					// missed
					expectScanStateAtTimeEquals((3*7+3)*24*time.Hour, hostscan.StateSkip)
					expectScanStateAtTimeEquals((3*7+4)*24*time.Hour, hostscan.StateSkip)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateSkip)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateSkip)
				})

				It("should skip terminating state", func() {
					expectAndUpdateScanState(2*7*24*time.Hour, hostscan.StateGracePeriod)
					expectAndUpdateScanState(3*7*24*time.Hour, hostscan.StateOneWeekNotice)

					// on time
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateTerminating)
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateTerminating)

					// missed
					expectScanStateAtTimeEquals(hostscan.HostScanPeriod-1*24*time.Hour, hostscan.StateSkip)
				})
			})

			It("should terminate after 4 weeks if previous db record is terminating", func() {
				scanTime := h.CreatedAt.Add(hostscan.HostScanPeriod - 3*24*time.Hour)
				h.NextScan = pq.NullTime{scanTime, true}
				hostscan.UpdateHostScanState(db, h)

				terminateTime := scanTime.Add(hostscan.TerminateNoticePeriod)

				hostScan := doScanHost(scanTime)
				Expect(hostScan.State).To(Equal(hostscan.StateTerminating))
				Expect(hostScan.NextScan).To(BeTemporally("==", terminateTime, 24*time.Hour))

				Expect(hostScan.UpdateHost(db)).To(BeNil())

				hosts, err := hostscan.FindTerminatingHosts(db, terminateTime)
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
				expectScanStateAtTimeEquals(5*24*time.Hour, hostscan.StateActive)
				expectScanStateAtTimeEquals(7*24*time.Hour, hostscan.StateActive)
				expectScanStateAtTimeEquals(2*7*24*time.Hour, hostscan.StateActive)
				expectScanStateAtTimeEquals(3*7*24*time.Hour, hostscan.StateActive)
				expectScanStateAtTimeEquals(hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateActive)
				expectScanStateAtTimeEquals(hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateActive)
			})

			It("should reset after 4 weeks period", func() {
				expectScanStateAtTimeEquals(hostscan.HostScanPeriod, hostscan.StateGracePeriod)
				expectScanStateAtTimeEquals(hostscan.HostScanPeriod+24*time.Hour, hostscan.StateGracePeriod)
				expectScanStateAtTimeEquals(hostscan.HostScanPeriod+7*24*time.Hour, hostscan.StateGracePeriod)
				expectScanStateAtTimeEquals(hostscan.HostScanPeriod+2*7*24*time.Hour, hostscan.StateGracePeriod)

				expectAndUpdateScanState(hostscan.HostScanPeriod+2*7*24*time.Hour, hostscan.StateGracePeriod)
				expectAndUpdateScanState(hostscan.HostScanPeriod+3*7*24*time.Hour, hostscan.StateOneWeekNotice)

				expectScanStateAtTimeEquals(2*hostscan.HostScanPeriod-3*24*time.Hour, hostscan.StateTerminating)
				expectScanStateAtTimeEquals(2*hostscan.HostScanPeriod-2*24*time.Hour, hostscan.StateTerminating)
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
				mock := scanAll(*h.CreatedAt)
				Expect(len(mock.UpdatedScans)).To(Equal(0))
				Expect(len(mock.RefreshedHosts)).To(Equal(0))
				Expect(len(mock.TerminatedHosts)).To(Equal(0))
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
			mock := scanAll(scanTime)
			Expect(len(mock.UpdatedScans)).To(Equal(0))
			Expect(len(mock.RefreshedHosts)).To(Equal(0))
			Expect(len(mock.TerminatedHosts)).To(Equal(0))
		})
	})
})
