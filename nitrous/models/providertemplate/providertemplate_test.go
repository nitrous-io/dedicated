package providertemplate_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/providertemplate")
}

var _ = Describe("models/providertemplate", func() {
	var (
		db *db.Database
		r  *region.Region
		p1 *plan.Plan
		p2 *plan.Plan
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		r = &region.Region{Slug: "apse1", Name: "Southeast Asia"}
		Expect(r.Insert(db)).To(BeNil())

		p1 = &plan.Plan{Slug: "512mb", Name: "micro", MaxContainers: 1, Price: 500}
		Expect(p1.Insert(db)).To(BeNil())
		p2 = &plan.Plan{Slug: "1gb", Name: "small", MaxContainers: 3, Price: 1000}
		Expect(p2.Insert(db)).To(BeNil())

		_, err = db.Exec(`INSERT INTO provider_templates (region_id, plan_id, name, provider, config)
			values ($1, $2, $3, $4, $5);`, r.ID, p1.ID, "do-512MB", "digitalocean", "")
		Expect(err).To(BeNil())
		_, err = db.Exec(`INSERT INTO provider_templates (region_id, plan_id, name, provider, config, disabled)
			values ($1, $2, $3, $4, $5, $6);`, r.ID, p2.ID, "do-hotswap-1GB", "digitalocean", "", true)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("All()", func() {
		It("returns all", func() {
			pts, err := providertemplate.All(db)
			Expect(err).To(BeNil())

			Expect(pts).To(HaveLen(2))

			pt1, pt2 := pts[0], pts[1]

			Expect([]string{
				pt1.Name, pt1.Provider,
				pt2.Name, pt2.Provider,
			}).To(ConsistOf([]string{
				"do-512MB", "digitalocean",
				"do-hotswap-1GB", "digitalocean",
			}))
		})
	})

	Describe("FindByID()", func() {
		var ptID int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM provider_templates WHERE region_id = $1 AND plan_id = $2 AND provider = $3", r.ID, p1.ID, "digitalocean").Scan(&ptID)
			Expect(err).To(BeNil())
		})

		Context("when a provider template record with a given id exists", func() {
			It("returns the provider template record", func() {
				pt, err := providertemplate.FindByID(db, ptID)
				Expect(err).To(BeNil())
				Expect(pt).NotTo(BeNil())
				Expect(pt.ID).To(Equal(ptID))
			})
		})

		Context("when a provider template record with a given id does not exist", func() {
			It("returns an error", func() {
				pt, err := providertemplate.FindByID(db, 9001)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(pt).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				pt, err := providertemplate.FindByID(db, ptID)
				Expect(err).NotTo(BeNil())
				Expect(pt).To(BeNil())
			})
		})
	})

	Describe("Find()", func() {
		It("returns the first matched provider template record", func() {
			pt, err := providertemplate.Find(db, r, p1, "digitalocean")
			Expect(err).To(BeNil())

			Expect(pt.ID).NotTo(BeZero())
			Expect(pt.RegionID).To(Equal(r.ID))
			Expect(pt.PlanID).To(Equal(p1.ID))
			Expect(pt.Name).To(Equal("do-512MB"))
			Expect(pt.Provider).To(Equal("digitalocean"))
		})

		It("does not include disabled provider templates", func() {
			orig, err := providertemplate.Find(db, r, p1, "digitalocean")
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO provider_templates (region_id, plan_id, name, provider, config, disabled)
				VALUES ($1, $2, $3, $4, $5, $6);`,
				r.ID, p1.ID, "do-512MB-disabled", "digitalocean", "", true)
			Expect(err).To(BeNil())

			pt, err := providertemplate.Find(db, r, p1, "digitalocean")
			Expect(err).To(BeNil())
			Expect(pt.ID).To(Equal(orig.ID))
		})

		It("returns an error when wanted provider template record does not exist", func() {
			pt, err := providertemplate.Find(db, &region.Region{ID: 9001}, &plan.Plan{ID: 9002}, "digitalocean")
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(pt).To(BeNil())
		})

		It("returns encountered error", func() {
			db.Close()
			pt, err := providertemplate.Find(db, r, p1, "digitalocean")
			Expect(pt).To(BeNil())
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("FindByName()", func() {
		It("finds the template by its name", func() {
			pt, err := providertemplate.FindByName(db, "do-512MB")
			Expect(err).To(BeNil())

			Expect(pt.ID).NotTo(BeZero())
			Expect(pt.RegionID).To(Equal(r.ID))
			Expect(pt.PlanID).To(Equal(p1.ID))
			Expect(pt.Name).To(Equal("do-512MB"))
			Expect(pt.Provider).To(Equal("digitalocean"))
		})
	})

	Describe("RespecTemplate()", func() {
		It("returns the first provider template for the given region / plan / provider combination", func() {
			pt := &providertemplate.ProviderTemplate{
				RegionID: r.ID,
				PlanID:   p1.ID,
				Provider: "digitalocean",
				Name:     "",
				Disabled: false,
			}

			newPT, err := pt.RespecTemplate(db, p2)
			Expect(err).To(BeNil())

			Expect(newPT.Name).To(Equal("do-hotswap-1GB"))
			Expect(newPT.ID).NotTo(BeZero())
			Expect(newPT.RegionID).To(Equal(r.ID))
			Expect(newPT.PlanID).To(Equal(p2.ID))
			Expect(newPT.Provider).To(Equal("digitalocean"))
		})

		It("returns a disabled provider template", func() {
			pt := &providertemplate.ProviderTemplate{
				RegionID: r.ID,
				PlanID:   p1.ID,
				Provider: "digitalocean",
				Name:     "",
				Disabled: true,
			}

			newPT, err := pt.RespecTemplate(db, p2)
			Expect(err).To(BeNil())

			Expect(newPT.Name).To(Equal("do-hotswap-1GB"))
			Expect(newPT.Disabled).To(BeTrue())
			Expect(newPT.ID).NotTo(BeZero())
			Expect(newPT.RegionID).To(Equal(r.ID))
			Expect(newPT.PlanID).To(Equal(p2.ID))
			Expect(newPT.Provider).To(Equal("digitalocean"))
		})

		It("returns an error when no provider record matches the conditions", func() {
			r2 := &region.Region{Slug: "apne1", Name: "Northeast Asia"}
			Expect(r2.Insert(db)).To(BeNil())

			pt := &providertemplate.ProviderTemplate{
				RegionID: r2.ID,
				PlanID:   p1.ID,
				Provider: "digitalocean",
				Name:     "",
				Disabled: false,
			}

			newPT, err := pt.RespecTemplate(db, p2)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(newPT).To(BeNil())
		})
	})

	Describe("Insert()", func() {
		var pt *providertemplate.ProviderTemplate

		BeforeEach(func() {
			var err error
			pt = &providertemplate.ProviderTemplate{
				RegionID:        r.ID,
				PlanID:          p1.ID,
				Name:            "ec2-micro-apse1",
				Provider:        "ec2",
				PreprovisionMax: 2,
				Disabled:        false,
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the provider template in the database", func() {
			pt, err := providertemplate.FindByID(db, pt.ID)
			Expect(err).To(BeNil())
			Expect(pt).NotTo(BeNil())
			Expect(pt.ID).NotTo(BeZero())
			Expect(pt.RegionID).To(Equal(r.ID))
			Expect(pt.PlanID).To(Equal(p1.ID))
			Expect(pt.Name).To(Equal("ec2-micro-apse1"))
			Expect(pt.Provider).To(Equal("ec2"))
			Expect(pt.PreprovisionMax).To(Equal(2))
			Expect(pt.Disabled).To(BeFalse())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(pt.ID).NotTo(BeZero())
			Expect(pt.CreatedAt).NotTo(BeNil())
			Expect(pt.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Update()", func() {
		var pt *providertemplate.ProviderTemplate

		BeforeEach(func() {
			var err error
			pt = &providertemplate.ProviderTemplate{
				RegionID:        r.ID,
				PlanID:          p1.ID,
				Name:            "ec2-micro-apse1",
				Provider:        "ec2",
				PreprovisionMax: 2,
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the provider template in the database", func() {
			pt.Name = "do-sgp-512mb"
			pt.Provider = "digitalocean"
			pt.PreprovisionMax = 1

			err := pt.Update(db)
			Expect(err).To(BeNil())

			pt2, err := providertemplate.FindByID(db, pt.ID)
			Expect(err).To(BeNil())

			// Update fields.
			Expect(pt2.Name).To(Equal("do-sgp-512mb"))
			Expect(pt2.Provider).To(Equal("digitalocean"))
			Expect(pt2.PreprovisionMax).To(Equal(1))

			// Unchanged fields.
			Expect(pt2.RegionID).To(Equal(r.ID))
			Expect(pt2.PlanID).To(Equal(p1.ID))
		})

		It("fills up the fields in original struct", func() {
			pt.Name = "do-sgp-512mb"
			err := pt.Update(db)
			Expect(err).To(BeNil())

			Expect(pt.CreatedAt).NotTo(BeNil())
			Expect(pt.UpdatedAt).NotTo(BeNil())
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := pt.UpdatedAt

			pt.Name = "do-sgp-512mb"
			err := pt.Update(db)
			Expect(err).To(BeNil())

			Expect(pt.UpdatedAt).NotTo(Equal(origUpdatedAt))
		})
	})
})
