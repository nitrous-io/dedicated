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
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "usagerecorder")
}

var _ = Describe("usagerecorder", func() {
	var (
		db  *db.Database
		u   *user.User
		h1  *host.Host
		h2  *host.Host
		r   *region.Region
		p   *plan.Plan
		pt  *providertemplate.ProviderTemplate
		lgr *logger.Logger

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

		lgr = &logger.Logger{Level: logger.LogLevelQuiet}

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
			inst := usagerecorder.New(db, lgr, &startTime, &endTime)
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
			inst := usagerecorder.New(db, lgr, &startTime, &endTime)
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

})
