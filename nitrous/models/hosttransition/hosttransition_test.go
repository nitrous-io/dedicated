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
		}
		err = h.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
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
				t := time.Now()
				h.State = "terminated"
				err := hosttransition.Record(db, h, "running", t)
				Expect(err).To(BeNil())

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
				t := time.Now()
				err := hosttransition.Record(db, h, "running", t)
				Expect(err).To(BeNil())

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
