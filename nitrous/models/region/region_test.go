package region_test

import (
	"database/sql"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	. "github.com/nitrous-io/dedicated/nitrous/test"
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
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", "apse1", "Southeast Asia", 2)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", "useast", "US East", 1)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO regions (slug, name, rank, disabled_at) values ($1, $2, $3, now());", "fiji", "Fiji", 1)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO regions (slug, name, rank, disabled_at) values ($1, $2, $3, now() + interval '1 hour');", "vegas", "Vegas", 3)
		Expect(err).To(BeNil())
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

	Describe("All()", func() {
		It("returns all regions, ordered by rank", func() {
			rs, err := region.All(db)
			Expect(err).To(BeNil())
			Expect(rs).To(HaveLen(3))
			Expect([]string{rs[0].Slug, rs[1].Slug, rs[2].Slug}).To(Equal([]string{"useast", "apse1", "vegas"}))
			Expect([]string{rs[0].Name, rs[1].Name, rs[2].Name}).To(Equal([]string{"US East", "Southeast Asia", "Vegas"}))
		})
	})

	Describe("AllForProvider()", func() {
		It("returns all regions for the given provider, ordered by rank", func() {
			// Add associated provider templates for the regions in the database.
			type row struct {
				ID int64 `db:"id"`
			}
			var rows []row
			err := db.Select(&rows, `SELECT id FROM regions;`)
			Expect(err).To(BeNil())
			Expect(rows).To(HaveLen(4))
			pl := plan.Plan{Slug: "Shift"}
			Expect(pl.Insert(db)).To(BeNil())
			for _, row := range rows {
				t := providertemplate.ProviderTemplate{
					PlanID:   pl.ID,
					RegionID: row.ID,
					Provider: "digitalocean",
				}
				Expect(t.Insert(db)).To(BeNil())
			}

			// Add a region belonging to another provider - it should not be returned.
			r := region.Region{Slug: "azure-redmond"}
			Expect(r.Insert(db)).To(BeNil())
			t := providertemplate.ProviderTemplate{
				PlanID:   pl.ID,
				RegionID: r.ID,
				Provider: "windows-azure",
			}
			Expect(t.Insert(db)).To(BeNil())

			rs, err := region.AllForProvider(db, "digitalocean")
			Expect(err).To(BeNil())
			Expect(rs).To(HaveLen(3))
			slugs := []string{}
			for _, r := range rs {
				slugs = append(slugs, r.Slug)
			}
			Expect(slugs).ToNot(ContainElement("azure-redmond"))
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
