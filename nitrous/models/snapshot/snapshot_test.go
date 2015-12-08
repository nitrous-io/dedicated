package snapshot_test

import (
	"database/sql"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/snapshot")
}

var _ = Describe("models/snapshot", func() {
	var (
		db *ndb.Database

		u          *user.User
		o1, o2     *org.Org
		i1, i2, i3 *image.Image
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "aroon",
			Email:    "aroonthampy@zerobotlabs.com",
			Password: "123456",
			Name:     "Aroon Thampy",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		o1 = &org.Org{
			Name:  "ze robot labs",
			Email: "admin@zerobotlabs.com",
			Slug:  "zerobotlabs",
		}
		Expect(o1.Insert(db)).To(BeNil())

		o2 = &org.Org{
			Name:  "Path",
			Email: "admin@path.com",
			Slug:  "path",
		}
		Expect(o2.Insert(db)).To(BeNil())

		hs1, err := test.CreateHostStack(db, &test.HostStack{U: u, O: o1})
		Expect(err).To(BeNil())

		hs2, err := test.CreateHostStack(db, &test.HostStack{U: u, O: o2})
		Expect(err).To(BeNil())

		i1 = &image.Image{
			UserID:      u.ID,
			HostID:      hs1.H.ID,
			FreighterID: "cafebabe",
			Name:        "nestorbot",
			Slug:        "nestorbot",
			Tag:         "latest",
		}
		Expect(i1.Insert(db)).To(BeNil())

		i2 = &image.Image{
			UserID:      u.ID,
			HostID:      hs2.H.ID,
			FreighterID: "cafedead",
			Name:        "car-os",
			Slug:        "car-os",
			Tag:         "latest",
		}
		Expect(i2.Insert(db)).To(BeNil())

		i3 = &image.Image{
			UserID:      u.ID,
			HostID:      hs2.H.ID,
			FreighterID: "cafebeef",
			Name:        "ubuntu",
			Slug:        "ubuntu",
			Tag:         "latest",
		}
		Expect(i3.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAllByOrgID()", func() {
		Context("when a non-existent user ID is given", func() {
			It("returns no snapshots", func() {
				snapshots, err := snapshot.FindAllByOrgID(db, u.ID+9000, o1.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(snapshots).To(HaveLen(0))
			})
		})

		Context("when a non-existent org ID is given", func() {
			It("returns no snapshots", func() {
				snapshots, err := snapshot.FindAllByOrgID(db, u.ID, o1.ID+9000)
				Expect(err).NotTo(HaveOccurred())
				Expect(snapshots).To(HaveLen(0))
			})
		})

		Context("when given a user that does not belong to the given org", func() {
			It("returns no snapshots", func() {
				snapshots, err := snapshot.FindAllByOrgID(db, u.ID, o1.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(snapshots).To(HaveLen(0))
			})
		})

		Context("when given a user who belongs to the given org", func() {
			BeforeEach(func() {
				err := orgmembership.AddMember(db, o1.ID, u.ID, "admin")
				Expect(err).To(BeNil())

				// Insert some snapshots belonging to the org.
				_, err = db.Exec(`INSERT INTO snapshots
					(image_id, user_id, org_id, slug, namespace, name, tag, repository, display_name, original_image)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`,
					i1.ID, u.ID, o1.ID, "zerobotlabs/nestorbot", "zerobotlabs", "nestorbot", "latest", "registry.n2odev.com/zerobotlabs/nestorbot:latest", "Nestorbot Dev", "golang:latest")
				Expect(err).To(BeNil())

				// Insert snapshot belonging to org but a different user.
				u2 := &user.User{
					Username: "elonmusk",
					Email:    "elonmusk@tesla.com",
					Password: "musky",
					Name:     "Elon Musk",
				}
				Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())
				_, err = db.Exec(`INSERT INTO snapshots
					(image_id, user_id, org_id, slug, namespace, name, tag, repository, display_name, original_image)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`,
					i2.ID, u2.ID, o1.ID, "path/car-os", "path", "car-os", "latest", "registry.n2odev.com/path/car-os:latest", "Car OS", "rust:latest")
				Expect(err).To(BeNil())

				// Insert snapshot in a different org.
				_, err = db.Exec(`INSERT INTO snapshots
					(image_id, user_id, org_id, slug, namespace, name, tag, repository, display_name, original_image)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`,
					i3.ID, u.ID, o2.ID, "path/addressgate", "path", "addressgate", "latest", "registry.n2odev.com/path/addressgate:latest", "Addressgate", "ruby:latest")
				Expect(err).To(BeNil())
			})

			It("returns snapshots belonging to the org", func() {
				snapshots, err := snapshot.FindAllByOrgID(db, u.ID, o1.ID)
				Expect(err).NotTo(HaveOccurred())

				var slugs []string
				for _, s := range snapshots {
					slugs = append(slugs, s.Slug)
				}
				Expect(slugs).To(ConsistOf("zerobotlabs/nestorbot", "path/car-os"))
			})
		})
	})

	Describe("FindByImageID()", func() {
		It("returns an error when there's no snapshot with the given image ID", func() {
			_, err := snapshot.FindByImageID(db, i1.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("returns the snapshot with the given image ID", func() {
			_, err := db.Exec(`INSERT INTO snapshots
				(image_id, user_id, org_id, slug, namespace, name, tag, repository, display_name, original_image)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`,
				i1.ID, u.ID, o1.ID, "zerobotlabs/nestorbot", "zerobotlabs", "nestorbot", "latest", "registry.n2odev.com/zerobotlabs/nestorbot:latest", "Nestorbot Dev", "golang:latest")
			Expect(err).To(BeNil())

			s, err := snapshot.FindByImageID(db, i1.ID)
			Expect(err).To(BeNil())

			Expect(s.ImageID).To(Equal(i1.ID))
			Expect(s.UserID).To(Equal(u.ID))
			Expect(s.OrgID.Int64).To(Equal(o1.ID))
			Expect(s.Slug).To(Equal("zerobotlabs/nestorbot"))
			Expect(s.Namespace).To(Equal("zerobotlabs"))
			Expect(s.Name).To(Equal("nestorbot"))
			Expect(s.Tag).To(Equal("latest"))
			Expect(s.Repository).To(Equal("registry.n2odev.com/zerobotlabs/nestorbot:latest"))
			Expect(s.DisplayName).To(Equal("Nestorbot Dev"))
			Expect(s.OriginalImage).To(Equal("golang:latest"))
		})
	})

	Describe("Insert()", func() {
		It("persists the snapshot in the database", func() {
			s := &snapshot.Snapshot{
				ImageID:       i1.ID,
				UserID:        u.ID,
				OrgID:         sql.NullInt64{o1.ID, true},
				Namespace:     "zerobotlabs",
				Name:          "nestorbot",
				Tag:           "latest",
				Repository:    "zerobotlabs/nestorbot",
				DisplayName:   "Nestorbot dev",
				OriginalImage: "golang:latest",
			}
			err := s.Insert(db)
			Expect(err).To(BeNil())

			// Timestamps are set.
			Expect(s.CreatedAt).NotTo(BeNil())
			Expect(s.UpdatedAt).NotTo(BeNil())

			s2 := &snapshot.Snapshot{}
			err = db.QueryRowx("SELECT * FROM snapshots WHERE id = $1", s.ID).StructScan(s2)
			Expect(err).To(BeNil())
			Expect(s).To(Equal(s2))
		})
	})

	Describe("Stack()", func() {
		It("returns stack for known Nitrous images", func() {
			testCases := []struct {
				image    string
				expected string
			}{
				{
					image:    "quay.io/nitrous/rails:latest",
					expected: "rails",
				},
				{
					image:    "quay.io/nitrous/ubuntu:latest",
					expected: "ubuntu",
				},
				{
					image:    "quay.io/nitrous/jekyll:latest",
					expected: "jekyll",
				},
			}

			for _, tc := range testCases {
				s := &snapshot.Snapshot{OriginalImage: tc.image}
				Expect(s.Stack()).To(Equal(tc.expected))
			}
		})

		It("guesses the stack from the original image name", func() {
			testCases := []struct {
				image    string
				expected string
			}{
				{
					image:    "rails:4.2",
					expected: "rails",
				},
				{
					image:    "ruby-on-rails:latest",
					expected: "rails",
				},
				{
					image:    "django:1.9",
					expected: "django",
				},
			}

			for _, tc := range testCases {
				s := &snapshot.Snapshot{OriginalImage: tc.image}
				Expect(s.Stack()).To(Equal(tc.expected))
			}
		})

		It("returns an empty string for an unknown original image name", func() {
			s := &snapshot.Snapshot{OriginalImage: "busybox:latest"}
			Expect(s.Stack()).To(Equal(""))
		})
	})
})
