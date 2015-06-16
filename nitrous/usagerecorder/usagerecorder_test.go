package usagerecorder_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostusage"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/nitrous/usagerecorder"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "usagerecorder")
}

var _ = Describe("usagerecorder", func() {
	var (
		db *db.Database
		u  *user.User
		h1 *host.Host
		h2 *host.Host
		r  *region.Region
		p  *plan.Plan
		pt *providertemplate.ProviderTemplate

		startTime time.Time
		endTime   time.Time
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
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		r = &region.Region{Slug: "apse1", Name: "Southeast Asia"}
		err = r.Insert(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{Slug: "512mb", Name: "micro", MaxContainers: 1, Price: 500}
		err = p.Insert(db)
		Expect(err).To(BeNil())

		pt = &providertemplate.ProviderTemplate{
			RegionID: r.ID,
			PlanID:   p.ID,
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
		err = pt.Insert(db)
		Expect(err).To(BeNil())

		h1 = &host.Host{
			UserID:             u.ID,
			Slug:               "my-host-1",
			RegionID:           r.ID,
			PlanID:             p.ID,
			ProviderTemplateID: pt.ID,
			HourlyPrice:        3000,
		}
		err = h1.Insert(db)
		Expect(err).To(BeNil())
		h2 = &host.Host{
			UserID:             u.ID,
			Slug:               "my-host-2",
			RegionID:           r.ID,
			PlanID:             p.ID,
			ProviderTemplateID: pt.ID,
		}
		err = h2.Insert(db)
		Expect(err).To(BeNil())

		base := time.Now()
		startTime = base.Add(-48 * time.Hour)
		endTime = base
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("ProcessUsages()", func() {
		var u2 *user.User
		var h3 *host.Host

		BeforeEach(func() {
			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err := WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			h3 = &host.Host{
				UserID:             u2.ID,
				Slug:               "my-host-3",
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
			}
			err = h3.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);",
				h1.ID, "born", "running", endTime.Add(-10*time.Hour))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);",
				h3.ID, "stopped", "running", endTime.Add(-2*time.Hour))
			Expect(err).To(BeNil())
		})

		It("should create usages for all users", func() {
			inst := usagerecorder.New(db, &startTime, &endTime)
			inst.ProcessUsages()

			usages1 := []hostusage.HostUsage{}
			err := db.Select(&usages1, `SELECT * FROM host_usages WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())
			Expect(usages1).To(HaveLen(1))

			usages2 := []hostusage.HostUsage{}
			err = db.Select(&usages2, `SELECT * FROM host_usages WHERE user_id = $1`, u2.ID)
			Expect(err).To(BeNil())
			Expect(usages2).To(HaveLen(1))

			Expect(usages1[0].Hours).To(Equal(10))
			Expect(usages2[0].Hours).To(Equal(2))
		})
	})

	Describe("RecordForUser()", func() {
		checkUsages := func() {
			inst := usagerecorder.New(db, &startTime, &endTime)
			err := inst.RecordForUser(u.ID)
			Expect(err).To(BeNil())

			usages := []hostusage.HostUsage{}
			err = db.Select(&usages, `SELECT * FROM host_usages WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())
			Expect(usages).To(HaveLen(2))

			var usage1 hostusage.HostUsage
			var usage2 hostusage.HostUsage
			for _, us := range usages {
				if us.HostID == h1.ID {
					usage1 = us
				} else {
					usage2 = us
				}
			}

			Expect(usage1.Hours).To(Equal(5))
			Expect(usage2.Hours).To(Equal(3))
		}

		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);",
				h1.ID, "born", "running", endTime.Add(-5*time.Hour))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);",
				h2.ID, "stopped", "running", endTime.Add(-3*time.Hour))
			Expect(err).To(BeNil())
		})

		It("should persist the usage for a given user", func() {
			checkUsages()
		})

		It("should be able to update existing records", func() {
			_, err := db.Exec("INSERT INTO host_usages (user_id, host_id, hours, day) values ($1, $2, $3, $4);",
				u.ID, h1.ID, 900, startTime)
			Expect(err).To(BeNil())
			checkUsages()
		})
	})

	Describe(".BillableHostHours", func() {

		It("should return nothing when there are no rows", func() {
			t := time.Now()
			e := t.Add(5 * time.Hour)
			inst := usagerecorder.New(db, &t, &e)
			res, err := inst.BillableHostHours(int64(1))
			Expect(err).To(BeNil())
			Expect(res).To(BeEmpty())
		})

		Context("with some transitions", func() {
			var run1 time.Time
			var stop1 time.Time
			var run2 time.Time
			var stop2 time.Time
			BeforeEach(func() {
				run1 = time.Now().Add(-48 * time.Hour)
				stop1 = run1.Add(1 * time.Hour)
				run2 = stop1.Add(30 * time.Minute)
				stop2 = run2.Add(2 * time.Hour)
				_, err := db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h1.ID, "born", "running", run1, stop1)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h1.ID, "running", "stopped", stop1, run2)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h1.ID, "stopped", "running", run2, stop2)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);",
					h1.ID, "running", "stopped", stop2)
				Expect(err).To(BeNil())
			})

			It("should tally up the total running hours to 3", func() {
				e := run1.Add(48 * time.Hour)
				inst := usagerecorder.New(db, &run1, &e)
				res, err := inst.BillableHostHours(u.ID)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Terminated).To(BeFalse())
				Expect(f.Hours()).To(Equal(3))
				Expect(f.HourlyPrice).To(Equal(int64(3000)))
			})

			It("should flow over to the next hour", func() {
				run3 := stop2.Add(5 * time.Hour)
				_, err := db.Exec("UPDATE host_transitions SET ended_at = $1 WHERE created_at = $2 AND host_id = $3 AND state = 'stopped';",
					run3, stop2, h1.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h1.ID, "stopped", "running", run3, run3.Add(7*time.Minute))
				Expect(err).To(BeNil())

				e := run1.Add(48 * time.Hour)
				inst := usagerecorder.New(db, &run1, &e)
				res, err := inst.BillableHostHours(u.ID)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Hours()).To(Equal(4))
			})

			It("should not include transitions outside a time range", func() {
				s := run2.Add(1 * time.Minute)
				e := run1.Add(48 * time.Hour)
				inst := usagerecorder.New(db, &s, &e)
				res, err := inst.BillableHostHours(u.ID)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Hours()).To(Equal(2))
			})

			It("should split out other hosts", func() {
				oH := &host.Host{
					UserID:             u.ID,
					Slug:               "my-host-6",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
					HourlyPrice:        9000,
				}
				err := oH.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET terminated_at = now() - interval '1 hour' WHERE id = $1;", oH.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE hosts SET terminated_at = now() + interval '1 hour' WHERE id = $1;", h1.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					oH.ID, "born", "running", run1, run1.Add(12*time.Hour))
				Expect(err).To(BeNil())

				e := run1.Add(48 * time.Hour)
				inst := usagerecorder.New(db, &run1, &e)
				res, err := inst.BillableHostHours(u.ID)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(2))
				var usage1 usagerecorder.HostHourUsage
				var usage2 usagerecorder.HostHourUsage
				for _, us := range res {
					if us.HostID == h1.ID {
						usage1 = us
					} else {
						usage2 = us
					}
				}
				Expect(usage1.Terminated).To(BeFalse())
				Expect(usage1.Hours()).To(Equal(3))
				Expect(usage1.HourlyPrice).To(Equal(int64(3000)))
				Expect(usage2.Terminated).To(BeTrue())
				Expect(usage2.Hours()).To(Equal(12))
				Expect(usage2.HourlyPrice).To(Equal(int64(9000)))
			})

			It("should exclude hosts not managed by user", func() {
				u2 := &user.User{
					Username: "mike",
					Email:    "mike@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())
				oH := &host.Host{
					UserID:             u2.ID,
					Slug:               "my-host-9",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
				}
				err = oH.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					oH.ID, "born", "running", run1, run1.Add(12*time.Hour))
				Expect(err).To(BeNil())

				e := run1.Add(48 * time.Hour)
				inst := usagerecorder.New(db, &run1, &e)
				res, err := inst.BillableHostHours(u.ID)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Hours()).To(Equal(3))
			})
		})

	})

})
