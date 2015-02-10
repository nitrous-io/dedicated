package hosttransition_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/hosttransition")
}

var _ = Describe("models/hosttransition", func() {
	var (
		db *db.Database
		u  *user.User
		h  *host.Host
		r  *region.Region
		p  *plan.Plan
		pt *providertemplate.ProviderTemplate
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

		h = &host.Host{
			UserID:             u.ID,
			Slug:               "my-host-1",
			RegionID:           r.ID,
			PlanID:             p.ID,
			ProviderTemplateID: pt.ID,
			HourlyPrice:        3000,
		}
		err = h.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe(".BillableHostHours", func() {

		It("should return nothing when there are no rows", func() {
			t := time.Now()
			res, err := hosttransition.BillableHostHours(db, int64(1), t, t.Add(5*time.Hour))
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
					h.ID, "born", "running", run1, stop1)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h.ID, "running", "stopped", stop1, run2)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h.ID, "stopped", "running", run2, stop2)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);",
					h.ID, "running", "stopped", stop2)
				Expect(err).To(BeNil())
			})

			It("should tally up the total running hours to 3", func() {
				res, err := hosttransition.BillableHostHours(db, u.ID, run1, run1.Add(48*time.Hour))
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
					run3, stop2, h.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					h.ID, "stopped", "running", run3, run3.Add(7*time.Minute))
				Expect(err).To(BeNil())

				res, err := hosttransition.BillableHostHours(db, u.ID, run1, run1.Add(48*time.Hour))
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Hours()).To(Equal(4))
			})

			It("should not include transitions outside a time range", func() {
				res, err := hosttransition.BillableHostHours(db, u.ID, run2.Add(1*time.Minute), run1.Add(48*time.Hour))
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Hours()).To(Equal(2))
			})

			It("should split out other hosts", func() {
				oH := &host.Host{
					UserID:             u.ID,
					Slug:               "my-host-2",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
					HourlyPrice:        9000,
				}
				err := oH.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET terminated_at = now() - interval '1 hour' WHERE id = $1;", oH.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE hosts SET terminated_at = now() + interval '1 hour' WHERE id = $1;", h.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					oH.ID, "born", "running", run1, run1.Add(12*time.Hour))
				Expect(err).To(BeNil())

				res, err := hosttransition.BillableHostHours(db, u.ID, run1, run1.Add(48*time.Hour))
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(2))
				var usage1 hosttransition.HostHourUsage
				var usage2 hosttransition.HostHourUsage
				for _, us := range res {
					if us.HostID == h.ID {
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
					Slug:               "my-host-2",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
				}
				err = oH.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);",
					oH.ID, "born", "running", run1, run1.Add(12*time.Hour))
				Expect(err).To(BeNil())

				res, err := hosttransition.BillableHostHours(db, u.ID, run1, run1.Add(48*time.Hour))
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				f := res[0]
				Expect(f.Hours()).To(Equal(3))
			})
		})

	})

	Describe(".Record", func() {

		Context("with no existing transition for host", func() {
			var (
				exId int64
				oH   *host.Host
			)
			BeforeEach(func() {
				oH = &host.Host{
					UserID:             u.ID,
					Slug:               "my-host-2",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
				}
				err := oH.Insert(db)
				Expect(err).To(BeNil())

				stm, err := db.Prepare(
					`INSERT INTO host_transitions (host_id, previous_state, state) VALUES ($1, 'restarting', 'suspended')  RETURNING ID`)
				Expect(err).To(BeNil())

				err = stm.QueryRow(oH.ID).Scan(&exId)
				Expect(err).To(BeNil())
			})

			It("should insert without touching other transitions", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				t := time.Now()
				h.State = "terminated"
				err = hosttransition.Record(tx, h, "running", t)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				preEx, err := hosttransition.FindByID(db, exId)
				Expect(err).To(BeNil())

				Expect(preEx.HostID).To(Equal(oH.ID))
				Expect(preEx.PreviousState.String).To(Equal("restarting"))
				Expect(preEx.State).To(Equal("suspended"))

				created, err := hosttransition.FindMostRecentForHost(db, h.ID)
				Expect(err).To(BeNil())
				Expect(created).ToNot(BeNil())

				Expect(created.State).To(Equal("terminated"))
				Expect(created.PreviousState.String).To(Equal("running"))
				Expect(created.CreatedAt.Unix()).To(Equal(t.Unix()))
			})
		})

		Context("with existing transitions for host", func() {
			var (
				exId int64
			)
			BeforeEach(func() {
				stm, err := db.Prepare(
					`INSERT INTO host_transitions (host_id, previous_state, state) VALUES ($1, 'restarting', 'suspended')  RETURNING ID`)
				Expect(err).To(BeNil())

				err = stm.QueryRow(h.ID).Scan(&exId)
				Expect(err).To(BeNil())
			})

			It("should update existing transition's ended_at for host", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				t := time.Now()
				err = hosttransition.Record(tx, h, "running", t)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				preEx, err := hosttransition.FindByID(db, exId)
				Expect(err).To(BeNil())

				Expect(preEx.PreviousState.String).To(Equal("restarting"))
				Expect(preEx.State).To(Equal("suspended"))
				Expect(preEx.EndedAt.Unix()).To(Equal(t.Unix()))

				created, err := hosttransition.FindMostRecentForHost(db, h.ID)
				Expect(err).To(BeNil())
				Expect(created).ToNot(BeNil())

				Expect(created.CreatedAt).To(Equal(preEx.EndedAt))
			})
		})

	})

})
