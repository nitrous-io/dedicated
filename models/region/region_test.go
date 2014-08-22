package region_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/region"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/region")
}

var _ = Describe("models/region", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", "apse1", "Southeast Asia", 2)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", "uswest", "US West", 3)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO regions (slug, name, rank) values ($1, $2, $3);", "useast", "US East", 1)
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

		Context("when a region record with a given id does not exist", func() {
			It("returns nil", func() {
				r, err := region.FindByID(db, 9001)
				Expect(err).To(BeNil())
				Expect(r).To(BeNil())
			})
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

		Context("when a region record with a given slug does not exist", func() {
			It("returns nil", func() {
				r, err := region.FindBySlug(db, "non-existent-region")
				Expect(err).To(BeNil())
				Expect(r).To(BeNil())
			})
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
			Expect([]string{rs[0].Slug, rs[1].Slug, rs[2].Slug}).To(ConsistOf([]string{"useast", "apse1", "uswest"}))
			Expect([]string{rs[0].Name, rs[1].Name, rs[2].Name}).To(ConsistOf([]string{"US East", "Southeast Asia", "US West"}))
		})
	})

	Describe("Insert()", func() {
		var (
			r *region.Region
		)

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
})
