package plan_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/plan")
}

var _ = Describe("models/plan", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO plans (slug, name, max_containers, price, cpu, memory, storage, disabled) values ($1, $2, $3, $4, $5, $6, $7, $8);", "1gb", "Micro", 4, 1995, "2", "1 GB", "20 GB", false)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO plans (slug, name, max_containers, price, cpu, memory, storage, disabled) values ($1, $2, $3, $4, $5, $6, $7, $8);", "2gb", "Small", 8, 3995, "4", "2 GB", "30 GB", false)
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
			err := db.QueryRow(`SELECT id FROM plans WHERE slug = $1;`, "1gb").Scan(&id)
			Expect(err).To(BeNil())
			Expect(id).NotTo(BeZero())
		})

		Context("when a plan record with a given id exists", func() {
			It("returns the plan record", func() {
				p, err := plan.FindByID(db, id)
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).To(Equal(id))
			})
		})

		Context("when a plan record with a given id does not exist", func() {
			It("returns nil", func() {
				p, err := plan.FindByID(db, 9001)
				Expect(err).To(BeNil())
				Expect(p).To(BeNil())
			})
		})
	})

	Describe("FindBySlug()", func() {
		Context("when a plan record with a given slug exists", func() {
			It("returns the plan record", func() {
				p, err := plan.FindBySlug(db, "1gb", false)
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).NotTo(BeZero())
				Expect(p.Slug).To(Equal("1gb"))
				Expect(p.Name).To(Equal("Micro"))
				Expect(p.MaxContainers).To(Equal(int64(4)))
				Expect(p.Price).To(Equal(int64(1995)))
				Expect(p.CPU).To(Equal("2"))
				Expect(p.Memory).To(Equal("1 GB"))
				Expect(p.Storage).To(Equal("20 GB"))
			})
		})

		Context("when a plan record with a given slug does not exist", func() {
			It("returns nil", func() {
				p, err := plan.FindBySlug(db, "non-existent-slug", false)
				Expect(err).To(BeNil())
				Expect(p).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				p, err := plan.FindBySlug(db, "1gb", false)
				Expect(p).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})

		Context("when querying for a disabled plan", func() {
			BeforeEach(func() {
				// add a disabled plan
				p := &plan.Plan{Slug: "3gb", Name: "Medium", MaxContainers: 1, Price: 500, Disabled: true}
				err := p.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns nil if includeDisabled arg is false", func() {
				p, err := plan.FindBySlug(db, "3gb", false)
				Expect(err).To(BeNil())
				Expect(p).To(BeNil())
			})

			It("returns the disabled plan if includeDisabled arg is true", func() {
				p, err := plan.FindBySlug(db, "3gb", true)
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).NotTo(BeZero())
				Expect(p.Slug).To(Equal("3gb"))
				Expect(p.Name).To(Equal("Medium"))
			})
		})
	})

	Describe("All()", func() {
		It("returns all plans", func() {
			pls, err := plan.All(db)
			Expect(err).To(BeNil())
			Expect(pls).To(HaveLen(2))
			Expect([]string{pls[0].Slug, pls[1].Slug}).To(ConsistOf([]string{"1gb", "2gb"}))
			Expect([]string{pls[0].Name, pls[1].Name}).To(ConsistOf([]string{"Micro", "Small"}))
		})
	})

	Describe("Enabled()", func() {
		It("returns all enabled plans", func() {
			// add couple of disabled plans
			_, err := db.Exec("INSERT INTO plans (slug, name, max_containers, price, cpu, memory, storage, disabled) values ($1, $2, $3, $4, $5, $6, $7, $8);", "3gb", "Medium", 4, 4995, "6", "3 GB", "40 GB", true)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO plans (slug, name, max_containers, price, cpu, memory, storage, disabled) values ($1, $2, $3, $4, $5, $6, $7, $8);", "4gb", "Large", 8, 5995, "8", "4 GB", "30 GB", true)
			Expect(err).To(BeNil())

			pls, err := plan.Enabled(db)
			Expect(err).To(BeNil())
			Expect(pls).To(HaveLen(2))
			Expect([]string{pls[0].Slug, pls[1].Slug}).To(ConsistOf([]string{"1gb", "2gb"}))
			Expect([]string{pls[0].Name, pls[1].Name}).To(ConsistOf([]string{"Micro", "Small"}))
		})
	})

	Describe("Insert()", func() {
		var (
			p *plan.Plan
		)

		BeforeEach(func() {
			var err error
			p = &plan.Plan{
				Slug:          "512mb",
				Name:          "Nano",
				MaxContainers: 2,
				Price:         995,
				CPU:           "1",
				Memory:        "512 MB",
				Storage:       "20 GB",
			}
			err = p.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the plan in the database", func() {
			p2, err := plan.FindBySlug(db, "512mb", false)
			Expect(err).To(BeNil())
			Expect(p2).NotTo(BeNil())
			Expect(p2.ID).NotTo(BeZero())
			Expect(p2.Slug).To(Equal("512mb"))
			Expect(p2.Name).To(Equal("Nano"))
			Expect(p2.MaxContainers).To(Equal(int64(2)))
			Expect(p2.Price).To(Equal(int64(995)))
			Expect(p2.CPU).To(Equal("1"))
			Expect(p2.Memory).To(Equal("512 MB"))
			Expect(p2.Storage).To(Equal("20 GB"))
			Expect(p2.CreatedAt).NotTo(BeNil())
			Expect(p2.UpdatedAt).NotTo(BeNil())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(p.ID).NotTo(BeZero())
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})
	})
})
