package container_test

import (
	"database/sql"
	"testing"
	"time"

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

		_, err = db.Exec("INSERT INTO containers (cid, user_id, host_id, image, name) values ($1, $2, $3, $4, $5);",
			"deadbeef", u.ID, h.ID, "nitrous-base", "my-1st-container")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GenerateRandomName()", func() {
		It("returns 'container-' suffixed with a timestamp", func() {
			name := container.GenerateRandomName()
			Expect(name).To(MatchRegexp(`\Acontainer-[0-9]+\z`))
		})

		It("differs when called a second time", func() {
			Expect(container.GenerateRandomName()).NotTo(Equal(container.GenerateRandomName()))
		})
	})

	Describe("FindByCID()", func() {
		Context("when a container record with a given cid exists", func() {
			It("returns the container record", func() {
				c, err := container.FindByCID(db, "deadbeef")
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.CID).To(Equal("deadbeef"))
				Expect(c.Image).To(Equal("nitrous-base"))
				Expect(c.Name).To(Equal("my-1st-container"))
			})
		})

		Context("when a container record with a given cid does not exist", func() {
			It("returns nil", func() {
				c, err := container.FindByCID(db, "non-existent-id")
				Expect(err).To(BeNil())
				Expect(c).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				c, err := container.FindByCID(db, "deadbeef")
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
				Expect(c.CID).To(Equal("deadbeef"))
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

				Expect(containers[0].CID).To(Equal("deadbeef"))
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
				CID:    "cafebabe",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Name:   "java-test-container",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the container in the database", func() {
			cntnr, err := container.FindByCID(db, "cafebabe")
			Expect(err).To(BeNil())
			Expect(cntnr).NotTo(BeNil())
			Expect(cntnr.CID).To(Equal("cafebabe"))
			Expect(cntnr.UserID).To(Equal(u.ID))
			Expect(cntnr.HostID).To(Equal(h.ID))
			Expect(cntnr.Image).To(Equal("nitrous-base"))
			Expect(cntnr.Name).To(Equal("java-test-container"))
		})

		It("fills up the fields in original struct", func() {
			Expect(c.CreatedAt).NotTo(BeNil())
			Expect(c.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Update()", func() {
		var (
			c   *container.Container
			cid = "cafebabe"
		)

		BeforeEach(func() {
			var err error
			c = &container.Container{
				CID:    cid,
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Name:   "java-test-container",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the container in the database", func() {
			c.Name = "jboss"
			c.IPAddress = sql.NullString{"1.2.3.4", true}
			touchedAt := time.Date(2013, time.November, 10, 0, 0, 0, 0, time.UTC)
			c.TouchedAt = &touchedAt
			err := c.Update(db)
			Expect(err).To(BeNil())

			cntnr, err := container.FindByCID(db, cid)
			Expect(err).To(BeNil())
			Expect(cntnr).NotTo(BeNil())
			Expect(cntnr.CID).To(Equal(cid))

			// Updated fields.
			Expect(cntnr.Name).To(Equal("jboss"))
			Expect(cntnr.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(cntnr.TouchedAt.Unix()).To(Equal(touchedAt.Unix()))

			// Unchanged fields.
			Expect(cntnr.UserID).To(Equal(u.ID))
			Expect(cntnr.HostID).To(Equal(h.ID))
			Expect(cntnr.Image).To(Equal("nitrous-base"))
		})

		It("fills up the fields in original struct", func() {
			c.Name = "jboss"
			err := c.Update(db)
			Expect(err).To(BeNil())

			Expect(c.CreatedAt).NotTo(BeNil())
			Expect(c.UpdatedAt).NotTo(BeNil())
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := c.UpdatedAt

			c.Name = "jboss"
			err := c.Update(db)
			Expect(err).To(BeNil())

			Expect(c.UpdatedAt).NotTo(Equal(origUpdatedAt))
		})

		Context("unchangeable fields", func() {
			Context("user ID", func() {
				It("does not get updated", func() {
					u2 := &user.User{
						Username: "bar",
						Email:    "bar@example.com",
						Password: "123456",
					}
					err := u2.Insert(db)
					Expect(err).To(BeNil())

					c.UserID = u2.ID
					err = c.Update(db)
					Expect(err).To(BeNil())

					cntnr, err := container.FindByCID(db, cid)
					Expect(err).To(BeNil())
					Expect(cntnr.UserID).To(Equal(u.ID))
				})
			})

			Context("host ID", func() {
				It("does not get updated", func() {
					h2 := &host.Host{
						UserID:             u.ID,
						Name:               "my-host-2",
						RegionID:           r.ID,
						PlanID:             p.ID,
						ProviderTemplateID: pt.ID,
					}
					err := h2.Insert(db)
					Expect(err).To(BeNil())

					c.HostID = h2.ID
					err = c.Update(db)
					Expect(err).To(BeNil())

					cntnr, err := container.FindByCID(db, cid)
					Expect(err).To(BeNil())
					Expect(cntnr.HostID).To(Equal(h.ID))
				})
			})

			Context("image", func() {
				It("does not get updated", func() {
					c.Image = "nitrous-base-v2"
					err := c.Update(db)
					Expect(err).To(BeNil())

					cntnr, err := container.FindByCID(db, cid)
					Expect(err).To(BeNil())
					Expect(cntnr.Image).To(Equal("nitrous-base"))
				})
			})

			Context("created at timestamp", func() {
				It("does not get updated", func() {
					origCreatedAt := c.CreatedAt

					newCreatedAt := time.Date(2013, time.November, 10, 0, 0, 0, 0, time.UTC)
					c.CreatedAt = &newCreatedAt
					err := c.Update(db)
					Expect(err).To(BeNil())

					cntnr, err := container.FindByCID(db, cid)
					Expect(err).To(BeNil())
					Expect(cntnr.CreatedAt).NotTo(Equal(newCreatedAt))
					Expect(cntnr.CreatedAt).To(Equal(origCreatedAt))
				})
			})
		})
	})

	Describe("Validate()", func() {
		var (
			c *container.Container
		)

		BeforeEach(func() {
			c = &container.Container{
				UserID: u.ID,
				HostID: h.ID,
				Name:   "foo",
				Image:  "nitrous-base",
			}
			Expect(c.Validate()).To(BeNil())
		})

		It("validates name", func() {
			buf := make([]byte, 256)
			for i := range buf {
				buf[i] = 'f'
			}
			s := string(buf) // "f" * 256.

			tests := []struct {
				input string
				err   error
			}{
				{"", container.ErrNameBlank},
				{"foo", nil},
				{"f00", nil},
				{"foo-bar", nil},
				{"fo", nil},
				{"f", container.ErrNameTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, container.ErrNameTooLong},
				{s[:255], nil},
				{"foo-", container.ErrNameInvalid},
				{"f-o-0-", container.ErrNameInvalid},
				{"-foo", container.ErrNameInvalid},
				{"FOO", container.ErrNameInvalid},
			}

			for _, t := range tests {
				c.Name = t.input
				Expect(c.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})

		It("validates image", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", container.ErrImageBlank},
				{"foo", nil},
			}

			for _, t := range tests {
				c.Image = t.input
				Expect(c.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})
})
