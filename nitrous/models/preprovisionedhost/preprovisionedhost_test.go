package preprovisionedhost_test

import (
	"testing"
	"time"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/preprovisionedhost"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/preprovisionedhost")
}

var _ = Describe("models/preprovisionedhost", func() {
	var (
		db       *ndb.Database
		r1, r2   *region.Region
		p1, p2   *plan.Plan
		pt1, pt2 *providertemplate.ProviderTemplate
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		r1 = &region.Region{Slug: "sgp"}
		Expect(r1.Insert(db)).To(BeNil())
		r2 = &region.Region{Slug: "sfo"}
		Expect(r2.Insert(db)).To(BeNil())
		p1 = &plan.Plan{Slug: "512mb", Name: "Nano", MaxContainers: 1, Price: 500}
		Expect(p1.Insert(db)).To(BeNil())
		p2 = &plan.Plan{Slug: "1gb", Name: "Micro", MaxContainers: 3, Price: 1000}
		Expect(p2.Insert(db)).To(BeNil())
		pt1 = &providertemplate.ProviderTemplate{
			RegionID: r1.ID,
			PlanID:   p1.ID,
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
		Expect(pt1.Insert(db)).To(BeNil())
		pt2 = &providertemplate.ProviderTemplate{
			RegionID: r2.ID,
			PlanID:   p1.ID,
			Name:     "digital-ocean-2",
			Provider: "digitalocean",
		}
		Expect(pt2.Insert(db)).To(BeNil())

		_, err = db.Exec(`INSERT INTO preprovisioned_hosts
				(region_id, plan_id, provider_template_id, slug)
				VALUES ($1, $2, $3, $4);`,
			r1.ID, p1.ID, pt1.ID, "preprovisioned-host-1")
		Expect(err).To(BeNil())
		_, err = db.Exec(`INSERT INTO preprovisioned_hosts
				(region_id, plan_id, provider_template_id, slug)
				VALUES ($1, $2, $3, $4);`,
			r1.ID, p1.ID, pt1.ID, "preprovisioned-host-2")
		Expect(err).To(BeNil())
		_, err = db.Exec(`INSERT INTO preprovisioned_hosts
				(region_id, plan_id, provider_template_id, slug, terminated_at)
				VALUES ($1, $2, $3, $4, $5);`,
			r1.ID, p1.ID, pt1.ID, "preprovisioned-host-3", time.Now())
		_, err = db.Exec(`INSERT INTO preprovisioned_hosts
				(region_id, plan_id, provider_template_id, slug)
				VALUES ($1, $2, $3, $4);`,
			r2.ID, p1.ID, pt2.ID, "preprovisioned-host-4")
		Expect(err).To(BeNil())
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAll", func() {
		It("returns non-terminated pre-provisioned hosts in the given region and plan", func() {
			hosts, err := preprovisionedhost.FindAll(db, r1, p1)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(2))

			h := hosts[0]
			Expect(h.RegionID).To(Equal(r1.ID))
			Expect(h.PlanID).To(Equal(p1.ID))
			Expect(h.ProviderTemplateID).To(Equal(pt1.ID))
			Expect(h.Slug).To(Equal("preprovisioned-host-1"))

			h = hosts[1]
			Expect(h.RegionID).To(Equal(r1.ID))
			Expect(h.PlanID).To(Equal(p1.ID))
			Expect(h.ProviderTemplateID).To(Equal(pt1.ID))
			Expect(h.Slug).To(Equal("preprovisioned-host-2"))
		})

		It("returns an empty slice if there are no pre-provisioned hosts in the given region and plan", func() {
			hosts, err := preprovisionedhost.FindAll(db, r2, p2)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(0))
		})
	})
})
