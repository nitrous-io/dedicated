package container_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/container")
}

var _ = Describe("models/container", func() {
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
		err = u.Insert(db)
		Expect(err).To(BeNil())

		r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
		err = r.Insert(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
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
			Name:               "my-host-1",
			RegionID:           r.ID,
			PlanID:             p.ID,
			ProviderTemplateID: pt.ID,
		}
		err = h.Insert(db)
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO containers (id, user_id, host_id, image, name) values ($1, $2, $3, $4, $5);",
			"deadbeef", u.ID, h.ID, "nitrous-base", "my-1st-container")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByID()", func() {
		Context("when a container record with a given id exists", func() {
			It("returns the container record", func() {
				c, err := container.FindByID(db, "deadbeef")
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.ID).To(Equal("deadbeef"))
				Expect(c.Image).To(Equal("nitrous-base"))
				Expect(c.Name).To(Equal("my-1st-container"))
			})
		})

		Context("when a container record with a given id does not exist", func() {
			It("returns nil", func() {
				c, err := container.FindByID(db, "non-existent-id")
				Expect(err).To(BeNil())
				Expect(c).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				c, err := container.FindByID(db, "deadbeef")
				Expect(err).NotTo(BeNil())
				Expect(c).To(BeNil())
			})
		})
	})

	Describe("FindByHostIDAndName()", func() {
		Context("when a container record with a given host ID and name exists", func() {
			It("returns the host record", func() {
				c, err := container.FindByHostIDAndName(db, h.ID, "my-1st-container")
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.ID).To(Equal("deadbeef"))
				Expect(c.Image).To(Equal("nitrous-base"))
				Expect(c.Name).To(Equal("my-1st-container"))
			})
		})

		Context("when a container record with a given host ID and name does not exist", func() {
			It("returns nil", func() {
				c, err := container.FindByHostIDAndName(db, h.ID, "non-existent-containername")
				Expect(err).To(BeNil())
				Expect(c).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				c, err := container.FindByHostIDAndName(db, h.ID, "my-1st-container")
				Expect(err).NotTo(BeNil())
				Expect(c).To(BeNil())
			})
		})
	})

	Describe("FindAllByHostID()", func() {
		Context("when the given host has container records", func() {
			It("returns the container records", func() {
				containers, err := container.FindAllByHostID(db, h.ID)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(1))

				Expect(containers[0].ID).To(Equal("deadbeef"))
				Expect(containers[0].Image).To(Equal("nitrous-base"))
				Expect(containers[0].Name).To(Equal("my-1st-container"))
			})
		})

		Context("when the given host does not have any container records", func() {
			var h2 *host.Host

			BeforeEach(func() {
				var err error
				h2 = &host.Host{
					UserID:             u.ID,
					Name:               "my-host-2",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
				}
				err = h2.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns an empty slice", func() {
				containers, err := container.FindAllByHostID(db, h2.ID)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(0))
			})
		})

		Context("when the given host does not exist", func() {
			It("returns an empty slice", func() {
				containers, err := container.FindAllByHostID(db, 9001)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(0))
			})
		})
	})

	Describe("Insert()", func() {
		var (
			c *container.Container
		)

		BeforeEach(func() {
			var err error
			c = &container.Container{
				ID:     "cafebabe",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Name:   "java-test-container",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the container in the database", func() {
			cntnr, err := container.FindByID(db, "cafebabe")
			Expect(err).To(BeNil())
			Expect(cntnr).NotTo(BeNil())
			Expect(cntnr.ID).To(Equal("cafebabe"))
			Expect(cntnr.UserID).To(Equal(u.ID))
			Expect(cntnr.Image).To(Equal("nitrous-base"))
			Expect(cntnr.Name).To(Equal("java-test-container"))
		})

		It("fills up the fields in original struct", func() {
			Expect(c.CreatedAt).NotTo(BeNil())
			Expect(c.UpdatedAt).NotTo(BeNil())
		})
	})
})
