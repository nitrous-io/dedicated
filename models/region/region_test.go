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

		_, err = db.Exec("INSERT INTO regions (name, description) values ($1, $2);", "apse1", "Southeast Asia")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO regions (name, description) values ($1, $2);", "uswest", "US West")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByName()", func() {
		Context("when a region record with a given name exists", func() {
			It("returns the region record", func() {
				r, err := region.FindByName(db, "apse1")
				Expect(err).To(BeNil())
				Expect(r).NotTo(BeNil())
				Expect(r.ID).NotTo(BeZero())
				Expect(r.Name).To(Equal("apse1"))
				Expect(r.Description).To(Equal("Southeast Asia"))
			})
		})

		Context("when a region record with a given name does not exist", func() {
			It("returns nil", func() {
				r, err := region.FindByName(db, "non-existent-region")
				Expect(err).To(BeNil())
				Expect(r).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				r, err := region.FindByName(db, "apse1")
				Expect(r).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var (
			r *region.Region
		)

		BeforeEach(func() {
			var err error
			r = &region.Region{
				Name:        "kalim",
				Description: "Kalimdor",
			}
			err = r.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the region in the database", func() {
			r2, err := region.FindByName(db, "kalim")
			Expect(err).To(BeNil())
			Expect(r2).NotTo(BeNil())
			Expect(r2.ID).NotTo(BeZero())
			Expect(r2.Name).To(Equal("kalim"))
			Expect(r2.Description).To(Equal("Kalimdor"))
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
