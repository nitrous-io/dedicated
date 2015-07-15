package container_test

import (
	"database/sql"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	. "github.com/nitrous-io/dedicated/nitrous/test"
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
		c  *container.Container
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
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		r = &region.Region{Slug: "apse1", Name: "Southeast Asia"}
		err = r.Insert(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{Slug: "512mb", Name: "micro", MaxContainers: 1, Price: 500}
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
			Slug:               "my-host-1",
			RegionID:           r.ID,
			PlanID:             p.ID,
			ProviderTemplateID: pt.ID,
		}
		err = h.Insert(db)
		Expect(err).To(BeNil())

		c = &container.Container{
			CID:    "deadbeef",
			UserID: u.ID,
			HostID: h.ID,
			Image:  "nitrous-base",
			Slug:   "my-1st-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("String()", func() {
		It("redacts container secret", func() {
			s := fmt.Sprintf("%+v", c)
			Expect(s).NotTo(ContainSubstring(c.Secret))
			Expect(s).To(ContainSubstring("Secret:(REDACTED)"))

			s = fmt.Sprintf("%+v", c)
			Expect(s).NotTo(ContainSubstring(c.Secret))
			Expect(s).To(ContainSubstring("Secret:(REDACTED)"))
		})
	})

	Describe("Find()", func() {
		Context("when finding by slug", func() {
			It("returns the container record matching the slug that belongs to the user", func() {
				c, err := container.Find(db, u, c.Slug)
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.UserID).To(Equal(u.ID))
				Expect(c.HostID).To(Equal(h.ID))
				Expect(c.CID).To(Equal("deadbeef"))
				Expect(c.Slug).To(Equal("my-1st-container"))
			})

			It("returns an error if the container matching the given slug does not belong to the user", func() {
				u2 := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				c, err := container.Find(db, u2, c.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})

			It("returns an error when there's no matching container for the given slug'", func() {
				c, err := container.Find(db, u, "no-such-container")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})
		})

		Context("when finding by ID", func() {
			It("returns the container record matching the ID that belongs to the user", func() {
				c, err := container.Find(db, u, strconv.FormatInt(c.ID, 10))
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.UserID).To(Equal(u.ID))
				Expect(c.HostID).To(Equal(h.ID))
				Expect(c.CID).To(Equal("deadbeef"))
				Expect(c.Slug).To(Equal("my-1st-container"))
			})

			It("returns an error if the container matching the given slug does not belong to the user", func() {
				u2 := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				c, err := container.Find(db, u2, strconv.FormatInt(c.ID, 10))
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})

			It("returns an error when there's no matching container for the given slug'", func() {
				c, err := container.Find(db, u, "10001")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				c, err := container.Find(db, u, c.Slug)
				Expect(err).NotTo(BeNil())
				Expect(c).To(BeNil())
			})
		})
	})

	Describe("FindShared()", func() {
		var recv *user.User

		BeforeEach(func() {
			recv = &user.User{
				Username: "shared",
				Email:    "shared@example.com",
				Password: "123456",
			}
			err := WrapInTransaction(db, recv.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, recv.ID)
			Expect(err).To(BeNil())
		})

		Context("when finding by slug", func() {
			It("returns the container record shared with the user", func() {
				c, err := container.FindShared(db, recv, c.Slug)
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.UserID).To(Equal(u.ID))
				Expect(c.HostID).To(Equal(h.ID))
				Expect(c.CID).To(Equal("deadbeef"))
				Expect(c.Slug).To(Equal(c.Slug))
			})

			It("returns an error if the container matching the given slug is not shared with the user", func() {
				u2 := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				c, err := container.FindShared(db, u2, c.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})

			It("returns an error when there's no matching container for the given slug'", func() {
				c, err := container.Find(db, recv, "no-such-container")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})
		})

		Context("when finding by ID", func() {
			It("returns the container record shared with the user", func() {
				c, err := container.FindShared(db, recv, strconv.FormatInt(c.ID, 10))
				Expect(err).To(BeNil())
				Expect(c).NotTo(BeNil())
				Expect(c.UserID).To(Equal(u.ID))
				Expect(c.HostID).To(Equal(h.ID))
				Expect(c.CID).To(Equal("deadbeef"))
				Expect(c.Slug).To(Equal(c.Slug))
			})

			It("returns an error if the container matching the given slug is not shared with the user", func() {
				u2 := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				c, err := container.FindShared(db, u2, strconv.FormatInt(c.ID, 10))
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})

			It("returns an error when there's no matching container for the given slug'", func() {
				c, err := container.FindShared(db, recv, "10001")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c).To(BeNil())
			})
		})
	})

	Describe("FindAllShared()", func() {
		var (
			recv       *user.User
			cnr1, cnr2 *container.Container
		)

		BeforeEach(func() {
			recv = &user.User{
				Username: "shared",
				Email:    "shared@example.com",
				Password: "123456",
			}
			err := WrapInTransaction(db, recv.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, recv.ID)
			Expect(err).To(BeNil())

			cnr1 = &container.Container{
				CID:    "thingo3",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Slug:   "coffee-test-container",
			}
			err = cnr1.Insert(db)
			Expect(err).To(BeNil())

			cnr2 = &container.Container{
				CID:    "thingo8",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Slug:   "swift-test-container",
			}
			err = cnr2.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, cnr2.ID, recv.ID)
			Expect(err).To(BeNil())
		})

		It("should list all containers shared with the user", func() {
			cns, err := container.FindAllShared(db, recv.ID)
			Expect(err).To(BeNil())
			Expect(cns).To(HaveLen(2))
			Expect(cns).To(ConsistOf(cnr2, c))
		})

		It("should not include terminated containers", func() {
			_, err := db.Exec(`UPDATE containers SET terminated_at = now() WHERE id = $1;`, cnr2.ID)
			Expect(err).To(BeNil())

			cns, err := container.FindAllShared(db, recv.ID)
			Expect(err).To(BeNil())
			Expect(cns).To(HaveLen(1))
			Expect(cns).To(ConsistOf(c))
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
				Expect(c.Slug).To(Equal(c.Slug))
			})

			Context("when container has been terminated", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE containers SET terminated_at = now() WHERE cid = $1;`, "deadbeef")
					Expect(err).To(BeNil())
				})

				It("returns an error", func() {
					c, err := container.FindByCID(db, "deadbeef")
					Expect(err).To(MatchError(sql.ErrNoRows))
					Expect(c).To(BeNil())
				})
			})
		})

		Context("when a container record with a given cid does not exist", func() {
			It("returns an error", func() {
				c, err := container.FindByCID(db, "non-existent-id")
				Expect(err).To(MatchError(sql.ErrNoRows))
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

	Describe("FindAllByHostID()", func() {
		Context("when the given host has container records", func() {
			It("returns the container records", func() {
				containers, err := container.FindAllByHostID(db, h.ID)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(1))

				Expect(containers[0].CID).To(Equal("deadbeef"))
				Expect(containers[0].Image).To(Equal("nitrous-base"))
				Expect(containers[0].Slug).To(Equal(c.Slug))
			})

			Context("when some containers have been terminated", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE containers SET terminated_at = now() WHERE cid = $1;`, "deadbeef")
					Expect(err).To(BeNil())
				})

				It("does not return those containers", func() {
					containers, err := container.FindAllByHostID(db, h.ID)
					Expect(err).To(BeNil())
					Expect(containers).To(HaveLen(0))
				})
			})
		})

		Context("when the given host does not have any container records", func() {
			var h2 *host.Host

			BeforeEach(func() {
				var err error
				h2 = &host.Host{
					UserID:             u.ID,
					Slug:               "my-host-2",
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

	Describe("DeleteAllByHostID()", func() {
		var c *container.Container

		BeforeEach(func() {
			var err error
			c, err = container.Find(db, u, "my-1st-container")
			Expect(err).To(BeNil())
		})

		It("sets terminated_at of all containers with the given host ID", func() {
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM containers WHERE host_id = $1 AND terminated_at IS NULL;", h.ID).Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))

			err = container.DeleteAllByHostID(db, h.ID)
			Expect(err).To(BeNil())

			err = db.QueryRow("SELECT COUNT(*) FROM containers WHERE host_id = $1 AND terminated_at IS NULL;", h.ID).Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
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
				Slug:   "java-test-container",
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"Key":  {"Value", true},
						"Some": {"Thing", true},
					},
				},
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
			Expect(cntnr.Slug).To(Equal("java-test-container"))
			Expect(cntnr.Metadata.Map["Key"].Valid).To(BeTrue())
			Expect(cntnr.Metadata.Map["Key"].String).To(Equal("Value"))
			Expect(cntnr.Metadata.Map["Some"].Valid).To(BeTrue())
			Expect(cntnr.Metadata.Map["Some"].String).To(Equal("Thing"))
		})

		It("fills up the fields in original struct", func() {
			Expect(c.CreatedAt).NotTo(BeNil())
			Expect(c.UpdatedAt).NotTo(BeNil())
		})

		It("generates a new secret", func() {
			Expect(c.Secret).NotTo(BeEmpty())
			Expect(c.Secret).To(HaveLen(255))

			c2 := &container.Container{
				CID:    "deadbeef",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container-2",
			}
			err := c2.Insert(db)
			Expect(err).To(BeNil())
			Expect(c2.Secret).NotTo(BeEmpty())
			Expect(c2.Secret).To(HaveLen(255))
			Expect(c2.Secret).NotTo(Equal(c.Secret))
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
				Slug:   "java-test-container",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the container in the database", func() {
			c.Slug = "jboss"
			c.Name = sql.NullString{"JBoss Enterprise Server", true}
			touchedAt := time.Date(2013, time.November, 10, 0, 0, 0, 0, time.UTC)
			c.TouchedAt = &touchedAt
			err := c.Update(db)
			Expect(err).To(BeNil())

			cntnr, err := container.FindByCID(db, cid)
			Expect(err).To(BeNil())
			Expect(cntnr).NotTo(BeNil())
			Expect(cntnr.CID).To(Equal(cid))

			// Updated fields.
			Expect(cntnr.Slug).To(Equal("jboss"))
			Expect(cntnr.Name.String).To(Equal("JBoss Enterprise Server"))
			Expect(cntnr.TouchedAt.Unix()).To(Equal(touchedAt.Unix()))

			// Unchanged fields.
			Expect(cntnr.UserID).To(Equal(u.ID))
			Expect(cntnr.HostID).To(Equal(h.ID))
			Expect(cntnr.Image).To(Equal("nitrous-base"))
		})

		It("fills up the fields in original struct", func() {
			c.Slug = "jboss"
			err := c.Update(db)
			Expect(err).To(BeNil())

			Expect(c.CreatedAt).NotTo(BeNil())
			Expect(c.UpdatedAt).NotTo(BeNil())
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := c.UpdatedAt

			c.Slug = "jboss"
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
					err := WrapInTransaction(db, u2.Insert)
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
						Slug:               "my-host-2",
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

	Describe("Delete()", func() {
		var c *container.Container

		BeforeEach(func() {
			c = &container.Container{
				CID:          "beefface",
				UserID:       u.ID,
				HostID:       h.ID,
				Image:        "nitrous-base",
				Slug:         "java-test-container",
				TerminatedAt: nil,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			c, err = container.Find(db, u, "java-test-container")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.CID).To(Equal("beefface"))
		})

		It("sets terminated_at instead of removing the container from the database", func() {
			err := c.Delete(db)
			Expect(err).To(BeNil())
			Expect(c.TerminatedAt).NotTo(BeNil())

			c2 := &container.Container{}
			err = db.QueryRowx("SELECT * FROM containers WHERE id = $1", c.ID).StructScan(c2)
			Expect(err).To(BeNil())
			Expect(c2).To(Equal(c))
		})

		It("fills up the fields in the struct", func() {
			c2 := &container.Container{ID: c.ID}
			err := c2.Delete(db)
			Expect(err).To(BeNil())

			Expect(c2.CreatedAt).NotTo(BeNil())
			Expect(c2.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("DeletePermanently()", func() {
		It("deletes the container from the database", func() {
			c := &container.Container{
				UserID:       u.ID,
				HostID:       h.ID,
				Image:        "nitrous-base",
				Slug:         "beef",
				TerminatedAt: nil,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			// ensure association are deleted
			v := &volume.Volume{Slug: "beefyvolume", HostID: h.ID}
			err = v.Insert(db)
			Expect(err).To(BeNil())
			cv := &cvolume.ContainerVolume{ContainerID: c.ID, VolumeID: v.ID}
			err = cv.Insert(db)
			Expect(err).To(BeNil())

			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM containers WHERE slug = $1;`, "beef").Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))

			err = c.DeletePermanently(db)
			Expect(err).To(BeNil())

			err = db.QueryRow(`SELECT COUNT(*) FROM containers WHERE slug = $1;`, "beef").Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
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
				Slug:   "foo",
				Image:  "nitrous-base",
			}
			Expect(c.Validate()).To(BeNil())
		})

		It("validates name", func() {
			buf := make([]byte, 64)
			for i := range buf {
				buf[i] = 'f'
			}
			s := string(buf) // "f" * 64.

			tests := []struct {
				input string
				err   error
			}{
				{"", container.ErrSlugBlank},
				{"foo", nil},
				{"f00", nil},
				{"foo-bar", nil},
				{"fo", nil},
				{"f", container.ErrSlugTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, container.ErrSlugTooLong},
				{s[:63], nil},
				{"foo-", container.ErrSlugInvalid},
				{"f-o-0-", container.ErrSlugInvalid},
				{"-foo", container.ErrSlugInvalid},
				{"FOO", container.ErrSlugInvalid},
			}

			for _, t := range tests {
				c.Slug = t.input
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

	Describe("SetState()", func() {
		It("sets state from string", func() {
			testCases := []struct {
				input         string
				expectedState container.State
				ok            bool
			}{
				{"", container.StateUnknown, false},
				{"running", container.StateRunning, true},
				{"stopped", container.StateStopped, true},
				{"terminated", container.StateUnknown, false},
				{"unknown", container.StateUnknown, false},
				{"rubbish", container.StateUnknown, false},
			}

			for _, tc := range testCases {
				c := container.Container{}
				ok := c.SetState(tc.input)
				Expect(c.State).To(Equal(tc.expectedState), "Unexpected state for '%s'", tc.expectedState)
				Expect(ok).To(Equal(tc.ok))
			}
		})
	})

	Describe("ResolvedState()", func() {
		Context("when both TempState and State is set", func() {
			It("returns temp state if container is locked", func() {
				timeout := time.Now().Add(5 * time.Second)

				c := container.Container{
					State:       container.StateRunning,
					TempState:   sql.NullInt64{int64(container.StateStopping), true},
					LockedUntil: &timeout,
				}

				Expect(c.ResolvedState()).To(Equal(container.StateStopping))

				c.State = container.StateStopped
				Expect(c.ResolvedState()).To(Equal(container.StateStopping))
			})

			It("returns original state if container lock is expired", func() {
				timeout := time.Now().Add(-1 * time.Second)

				c := container.Container{
					State:       container.StateRunning,
					TempState:   sql.NullInt64{int64(container.StateStopping), true},
					LockedUntil: &timeout,
				}

				Expect(c.ResolvedState()).To(Equal(container.StateRunning))
			})
		})

		Context("when TempState is not set, but State is set", func() {
			It("returns state", func() {
				c := container.Container{}

				c.State = container.StateRunning
				Expect(c.ResolvedState()).To(Equal(container.StateRunning))

				c.State = container.StateStopped
				Expect(c.ResolvedState()).To(Equal(container.StateStopped))
			})
		})

		Context("when TerminatedAt is not nil", func() {
			It("returns StateTerminated", func() {
				terminatedAt := time.Date(2013, time.November, 3, 0, 0, 0, 0, time.UTC)
				c := container.Container{TerminatedAt: &terminatedAt}
				Expect(c.ResolvedState()).To(Equal(container.StateTerminated))
			})
		})

		Context("when State or TempState is not set", func() {
			It("returns unknown state", func() {
				c := container.Container{}
				Expect(c.ResolvedState()).To(Equal(container.StateUnknown))
			})
		})
	})

	Describe("Lock()", func() {
		var c *container.Container

		BeforeEach(func() {
			c = &container.Container{
				CID:    "cafebabe",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("locks the container to given state if it is not already locked", func() {
			expiresIn := 5 * time.Second
			err := c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(BeNil())
			Expect(c.LockedUntil).NotTo(BeNil())
			Expect(c.TempState.Int64).To(Equal(int64(container.StateStarting)))
		})

		It("returns ErrLocked when the container is locked and lock is not expired", func() {
			expiresIn := 5 * time.Second
			err := c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(BeNil())

			err = c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(Equal(container.ErrLocked))
		})

		It("locks the container if existing lock is expired", func() {
			expiresIn := -1 * time.Minute
			err := c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(BeNil())

			err = c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(BeNil())
			Expect(c.LockedUntil).NotTo(BeNil())
			Expect(c.TempState.Int64).To(Equal(int64(container.StateStarting)))
		})

		It("returns the error if Lock() fails for some other reason", func() {
			db.Close()

			expiresIn := time.Duration(0) * time.Second
			err := c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("Unlock()", func() {
		var c *container.Container

		BeforeEach(func() {
			c = &container.Container{
				CID:       "cafebabe",
				UserID:    u.ID,
				HostID:    h.ID,
				Image:     "nitrous-base",
				Slug:      "java-test-container",
				TempState: sql.NullInt64{int64(container.StateStarting), true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("unlocks the container and clears the temp state, allowing it to be locked again", func() {
			expiresIn := 5 * time.Second
			err := c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(BeNil())

			err = c.Unlock(db)
			Expect(err).To(BeNil())
			Expect(c.LockedUntil).To(BeNil())
			Expect(c.TempState.Valid).To(BeFalse())

			err = c.Lock(container.StateStarting, expiresIn, db)
			Expect(err).To(BeNil())
		})

		It("returns the error if Unlock() fails for some reason", func() {
			db.Close()

			err := c.Unlock(db)
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("IsLocked()", func() {
		var c *container.Container

		BeforeEach(func() {
			c = &container.Container{
				CID:    "cafebabe",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when container is not locked", func() {
			It("returns false and no error", func() {
				locked := c.IsLocked()
				Expect(locked).To(BeFalse())
			})
		})

		Context("when container is locked", func() {
			BeforeEach(func() {
				expiresIn := 5 * time.Second
				err := c.Lock(container.StateStarting, expiresIn, db)
				Expect(err).To(BeNil())
			})

			It("returns true and no error", func() {
				locked := c.IsLocked()
				Expect(locked).To(BeTrue())
			})
		})

		Context("when container was locked, but lock is expired", func() {
			BeforeEach(func() {
				expiresIn := -1 * time.Second
				err := c.Lock(container.StateStarting, expiresIn, db)
				Expect(err).To(BeNil())
			})

			It("returns false and no error", func() {
				locked := c.IsLocked()
				Expect(locked).To(BeFalse())
			})
		})
	})

	Describe("PreviewHostname()", func() {
		It("return the container's preview hostname", func() {
			orig := envvars.PreviewHostname
			envvars.PreviewHostname = "n2otest.com"
			defer func() { envvars.PreviewHostname = orig }()
			c := &container.Container{Slug: "hello"}
			Expect(c.PreviewHostname()).To(Equal("hello.n2otest.com"))
		})
	})

	Describe("FindAllByHostIDs()", func() {
		var (
			h2 *host.Host
			c2 *container.Container
		)

		BeforeEach(func() {
			h2 = &host.Host{
				UserID:             u.ID,
				Slug:               "my-host-2",
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
			}
			err := h2.Insert(db)
			Expect(err).To(BeNil())

			c2 = &container.Container{
				CID:    "abcabc",
				UserID: u.ID,
				HostID: h.ID,
				Image:  "nitrous-base",
				Slug:   "my-2nd-container",
			}
			err = c2.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when the given host has container records", func() {
			It("should return an empty set when the hostIDs are empty", func() {
				var hostIDs []int64
				containers, err := container.FindAllByHostIDs(db, hostIDs)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(0))
			})

			It("returns the container records", func() {
				var hostIDs []int64
				hostIDs = append(hostIDs, h.ID)
				hostIDs = append(hostIDs, h2.ID)
				containers, err := container.FindAllByHostIDs(db, hostIDs)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(2))

				cids := []string{}
				cids = append(cids, containers[0].CID)
				cids = append(cids, containers[1].CID)
				Expect(cids).To(ConsistOf("deadbeef", "abcabc"))
			})

			Context("when some containers have been terminated", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE containers SET terminated_at = now() WHERE cid = $1;`, "deadbeef")
					Expect(err).To(BeNil())
				})

				It("does not return those containers", func() {
					var hostIDs []int64
					hostIDs = append(hostIDs, h.ID)
					hostIDs = append(hostIDs, h2.ID)
					containers, err := container.FindAllByHostIDs(db, hostIDs)
					Expect(err).To(BeNil())
					Expect(containers).To(HaveLen(1))

					Expect(containers[0].CID).To(Equal("abcabc"))
				})
			})
		})

		Context("when the given host does not have any container records", func() {
			var h3 *host.Host

			BeforeEach(func() {
				var err error
				h3 = &host.Host{
					UserID:             u.ID,
					Slug:               "my-host-3",
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
				}
				err = h3.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns an empty slice", func() {
				var hostIDs []int64
				hostIDs = append(hostIDs, h3.ID)
				containers, err := container.FindAllByHostIDs(db, hostIDs)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(0))
			})
		})

		Context("when the given host does not exist", func() {
			It("returns an empty slice", func() {
				var hostIDs []int64
				hostIDs = append(hostIDs, 100000)
				containers, err := container.FindAllByHostIDs(db, hostIDs)
				Expect(err).To(BeNil())
				Expect(containers).To(HaveLen(0))
			})
		})
	})
})
