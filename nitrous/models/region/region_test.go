package region_test

import (
	"database/sql"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/region")
}

var _ = Describe("models/region", func() {
	var (
		db *ndb.Database
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		fixtures := []struct {
			stmt string
			args []interface{}
		}{
			{"INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", []interface{}{"apse1", "Southeast Asia", 2}},
			{"INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", []interface{}{"useast", "US East", 1}},
			{"INSERT INTO regions (slug, name, rank, disabled_at) values ($1, $2, $3, now());", []interface{}{"fiji", "Fiji", 1}},
			{"INSERT INTO regions (slug, name, rank, disabled_at) values ($1, $2, $3, now()+ interval '1 hour');", []interface{}{"vegas", "Vegas", 3}},
		}

		for _, f := range fixtures {
			_, err = db.Exec(f.stmt, f.args...)
			Expect(err).To(BeNil(), "failed to execute %s with %v", f.stmt, f.args)
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByID()", func() {
		var id int64

		BeforeEach(func() {
			err := db.QueryRow(`SELECT id FROM regions WHERE slug = $1;`, "apse1").Scan(&id)
			Expect(err).To(BeNil())
			Expect(id).NotTo(BeZero())
		})

		Context("when a region record with a given id exists", func() {
			It("returns the region record", func() {
				r, err := region.FindByID(db, id)
				Expect(err).To(BeNil())
				Expect(r).NotTo(BeNil())
				Expect(r.ID).To(Equal(id))
			})
		})

		It("returns error when a region record with a given id does not exist", func() {
			r, err := region.FindByID(db, 9001)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(r).To(BeNil())
		})
	})

	Describe("FindBySlug()", func() {
		Context("when a region record with a given slug exists", func() {
			It("returns the region record", func() {
				r, err := region.FindBySlug(db, "apse1")
				Expect(err).To(BeNil())
				Expect(r).NotTo(BeNil())
				Expect(r.ID).NotTo(BeZero())
				Expect(r.Slug).To(Equal("apse1"))
				Expect(r.Name).To(Equal("Southeast Asia"))
				Expect(r.Rank).To(Equal(int64(2)))
			})
		})

		It("returns error when a region record with a given slug does not exist", func() {
			r, err := region.FindBySlug(db, "non-existent-region")
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(r).To(BeNil())
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				r, err := region.FindBySlug(db, "apse1")
				Expect(r).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("AllForProvider()", func() {
		It("returns all regions for the given provider, ordered by rank", func() {
			pl := plan.Plan{Slug: "Shift"}
			Expect(pl.Insert(db)).To(BeNil())

			// Add associated provider templates for the regions in the database.
			var ids []int64
			db.Select(&ids, `SELECT id FROM regions;`)
			for _, id := range ids {
				t := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: id,
					Provider: "digitalocean",
				}
				Expect(t.Insert(db)).To(BeNil())
			}

			// Add a region belonging to a disabled provider template - it
			// should not be returned
			var (
				r  *region.Region
				pt *providertemplate.ProviderTemplate
			)

			r = &region.Region{Slug: "apse1-disabled"}
			Expect(r.Insert(db)).To(BeNil())
			pt = &providertemplate.ProviderTemplate{
				PlanID:   pl.ID,
				RegionID: r.ID,
				Provider: "digitalocean",
				Disabled: true,
			}
			Expect(pt.Insert(db)).To(BeNil())

			// Add a region belonging to another provider - it should not be returned.
			r = &region.Region{Slug: "azure-redmond"}
			Expect(r.Insert(db)).To(BeNil())
			pt = &providertemplate.ProviderTemplate{
				PlanID:   pl.ID,
				RegionID: r.ID,
				Provider: "windows-azure",
			}
			Expect(pt.Insert(db)).To(BeNil())

			rs, _ := region.AllForProvider(db, "digitalocean")
			slugs := []string{}
			for _, r := range rs {
				slugs = append(slugs, r.Slug)
			}
			Expect(slugs).To(Equal([]string{"useast", "apse1", "vegas"}))
		})
	})

	Describe("Insert()", func() {
		var r *region.Region

		BeforeEach(func() {
			var err error
			r = &region.Region{
				Slug: "kalim",
				Name: "Kalimdor",
				Rank: 33,
			}
			err = r.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the region in the database", func() {
			r2, err := region.FindBySlug(db, "kalim")
			Expect(err).To(BeNil())
			Expect(r2).NotTo(BeNil())
			Expect(r2.ID).NotTo(BeZero())
			Expect(r2.Slug).To(Equal("kalim"))
			Expect(r2.Name).To(Equal("Kalimdor"))
			Expect(r2.Rank).To(Equal(int64(33)))
			Expect(r2.CreatedAt).NotTo(BeNil())
			Expect(r2.UpdatedAt).NotTo(BeNil())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(r.ID).NotTo(BeZero())
			Expect(r.CreatedAt).NotTo(BeNil())
			Expect(r.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Disabled()", func() {
		It("should be true for regions currently disabled", func() {
			r, err := region.FindBySlug(db, "fiji")
			Expect(err).To(BeNil())
			Expect(r).ToNot(BeNil())
			Expect(r.Disabled()).To(BeTrue())
		})

		It("should be false for regions disabled in the future", func() {
			r, err := region.FindBySlug(db, "vegas")
			Expect(err).To(BeNil())
			Expect(r).ToNot(BeNil())
			Expect(r.Disabled()).To(BeFalse())
		})

		It("should be false for regions that are not disabled", func() {
			r, err := region.FindBySlug(db, "apse1")
			Expect(err).To(BeNil())
			Expect(r).ToNot(BeNil())
			Expect(r.Disabled()).To(BeFalse())
		})
	})
})
