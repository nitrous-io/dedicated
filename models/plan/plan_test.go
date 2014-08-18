package plan_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/plan"
	. "github.com/nitrous-io/nitrous/test"
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

		_, err = db.Exec("INSERT INTO plans (name, max_containers, price) values ($1, $2, $3);", "small", 3, 1000)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO plans (name, max_containers, price) values ($1, $2, $3);", "medium", 5, 2000)
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
			err := db.QueryRow(`SELECT id FROM plans WHERE name = $1;`, "small").Scan(&id)
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

	Describe("FindByName()", func() {
		Context("when a plan record with a given name exists", func() {
			It("returns the plan record", func() {
				p, err := plan.FindByName(db, "small")
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.ID).NotTo(BeZero())
				Expect(p.Name).To(Equal("small"))
				Expect(p.MaxContainers).To(Equal(int64(3)))
				Expect(p.Price).To(Equal(int64(1000)))
			})
		})

		Context("when a plan record with a given name does not exist", func() {
			It("returns nil", func() {
				p, err := plan.FindByName(db, "non-existent-plan")
				Expect(err).To(BeNil())
				Expect(p).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				p, err := plan.FindByName(db, "small")
				Expect(p).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("All()", func() {
		It("returns all plans", func() {
			pls, err := plan.All(db)
			Expect(err).To(BeNil())
			Expect(pls).To(HaveLen(2))
			Expect([]string{pls[0].Name, pls[1].Name}).To(ConsistOf([]string{"small", "medium"}))
		})
	})

	Describe("Insert()", func() {
		var (
			p *plan.Plan
		)

		BeforeEach(func() {
			var err error
			p = &plan.Plan{
				Name:          "micro",
				MaxContainers: 1,
				Price:         500,
			}
			err = p.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the plan in the database", func() {
			p2, err := plan.FindByName(db, "micro")
			Expect(err).To(BeNil())
			Expect(p2).NotTo(BeNil())
			Expect(p2.ID).NotTo(BeZero())
			Expect(p2.Name).To(Equal("micro"))
			Expect(p2.MaxContainers).To(Equal(int64(1)))
			Expect(p2.Price).To(Equal(int64(500)))
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
