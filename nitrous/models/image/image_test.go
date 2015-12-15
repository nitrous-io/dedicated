package image_test

import (
	"database/sql"
	"fmt"
	"strconv"
	"testing"

	"github.com/nitrous-io/dedicated/freighter/provider/docker/parsers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/image")
}

var _ = Describe("models/image", func() {
	var db *ndb.Database

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAll()", func() {
		Context("when a host is provided", func() {
			It("returns image records belong to the user and given host ordered by id", func() {
				hs, err := test.CreateHostStack(db, nil)
				Expect(err).To(BeNil())
				_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
					VALUES ($1, $2, $3, $4, $5, $6);`,
					hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "latest")
				Expect(err).To(BeNil())
				_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
					VALUES ($1, $2, $3, $4, $5, $6);`,
					hs.U.ID, hs.H.ID, "cafebabe v2", "scratch image v2", "scratchv2", "v2")
				Expect(err).To(BeNil())

				images, err := image.FindAll(db, hs.U, hs.H)
				Expect(err).To(BeNil())
				Expect(images).To(HaveLen(2))

				Expect(images[0].FreighterID).To(Equal("cafebabe"))
				Expect(images[0].Name).To(Equal("scratch image"))
				Expect(images[0].Slug).To(Equal("scratch"))
				Expect(images[0].Tag).To(Equal("latest"))

				Expect(images[1].FreighterID).To(Equal("cafebabe v2"))
				Expect(images[1].Name).To(Equal("scratch image v2"))
				Expect(images[1].Slug).To(Equal("scratchv2"))
				Expect(images[1].Tag).To(Equal("v2"))
			})
		})

		Context("when a host is not provided", func() {
			It("returns all image records belong to the user ordered by id", func() {
				hs, err := test.CreateHostStack(db, nil)
				Expect(err).To(BeNil())

				h2 := &host.Host{
					Slug:               "host-2",
					State:              host.StateRunning,
					ProviderIdentifier: sql.NullString{"host-0002", true},
					RegionID:           hs.R.ID,
					PlanID:             hs.P.ID,
					ProviderTemplateID: hs.PT.ID,
					UserID:             hs.U.ID,
				}
				err = h2.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
					VALUES ($1, $2, $3, $4, $5, $6);`,
					hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "latest")
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
					VALUES ($1, $2, $3, $4, $5, $6);`,
					hs.U.ID, h2.ID, "cafebabe v2", "scratch image v2", "scratchv2", "v2")
				Expect(err).To(BeNil())

				images, err := image.FindAll(db, hs.U, nil)
				Expect(err).To(BeNil())
				Expect(images).To(HaveLen(2))

				Expect(images[0].FreighterID).To(Equal("cafebabe"))
				Expect(images[0].Name).To(Equal("scratch image"))
				Expect(images[0].Slug).To(Equal("scratch"))
				Expect(images[0].Tag).To(Equal("latest"))
				Expect(images[0].UserID).To(Equal(hs.U.ID))
				Expect(images[0].HostID).To(Equal(hs.H.ID))

				Expect(images[1].FreighterID).To(Equal("cafebabe v2"))
				Expect(images[1].Name).To(Equal("scratch image v2"))
				Expect(images[1].Slug).To(Equal("scratchv2"))
				Expect(images[1].Tag).To(Equal("v2"))
				Expect(images[1].UserID).To(Equal(hs.U.ID))
				Expect(images[1].HostID).To(Equal(h2.ID))
			})
		})
	})

	Describe("Find()", func() {
		It("returns the image record when given its ID", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())

			var id int64
			nstmt, err := db.PrepareNamed(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
				VALUES (:user_id, :host_id, :freighter_id, :name, :slug, :tag) RETURNING id;`)
			Expect(err).To(BeNil())
			nstmt.QueryRowx(&image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Name:        "scratch image",
				Slug:        "scratch",
				Tag:         "latest",
			}).Scan(&id)

			img, err := image.Find(db, hs.U, hs.H, strconv.FormatInt(id, 10))
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.UserID).To(Equal(hs.U.ID))
			Expect(img.HostID).To(Equal(hs.H.ID))
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Slug).To(Equal("scratch"))
			Expect(img.Tag).To(Equal("latest"))
		})

		It("returns the image record when given its slug", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
				VALUES ($1, $2, $3, $4, $5, $6);`,
				hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "latest")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "scratch")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.UserID).To(Equal(hs.U.ID))
			Expect(img.HostID).To(Equal(hs.H.ID))
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Slug).To(Equal("scratch"))
			Expect(img.Tag).To(Equal("latest"))
		})

		It("returns the image record when given its slug with a tag", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
				VALUES ($1, $2, $3, $4, $5, $6);`,
				hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "mytag")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "scratch:mytag")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.UserID).To(Equal(hs.U.ID))
			Expect(img.HostID).To(Equal(hs.H.ID))
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Slug).To(Equal("scratch"))
			Expect(img.Tag).To(Equal("mytag"))
		})

		It("returns an error when the image doesn't exist", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = image.Find(db, hs.U, hs.H, "doesn't exist")
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("Insert()", func() {
		It("persists the image in the database", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())

			i := &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Slug:        "scratch",
				Tag:         "latest",
			}
			err = i.Insert(db)
			Expect(err).To(BeNil())

			// Timestamps are set.
			Expect(i.CreatedAt).NotTo(BeNil())
			Expect(i.UpdatedAt).NotTo(BeNil())

			i2 := &image.Image{}
			err = db.QueryRowx("SELECT * FROM images WHERE id = $1", i.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(i).To(Equal(i2))
		})
	})

	Describe("Delete()", func() {
		It("sets deleted_at", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
				VALUES ($1, $2, $3, $4, $5, $6);`,
				hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "latest")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "scratch:latest")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.DeletedAt.Valid).To(Equal(false))

			err = img.Delete(db)
			Expect(err).To(BeNil())

			err = db.Get(img, `SELECT * FROM images WHERE id = $1`, img.ID)
			Expect(err).To(BeNil())
			Expect(img.DeletedAt.Valid).To(Equal(true))
			Expect(img.DeletedAt.Time).NotTo(BeZero())
		})
	})

	Describe("Update()", func() {
		It("updates the image in the database", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
				VALUES ($1, $2, $3, $4, $5, $6);`,
				hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "latest")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "scratch:latest")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.FreighterID).To(Equal("cafebabe"))

			prevUpdatedAt := img.UpdatedAt

			img.FreighterID = "cafedead"
			img.Name = "scratch image v2"
			err = img.Update(db)
			Expect(err).To(BeNil())

			Expect(img.UpdatedAt).NotTo(Equal(prevUpdatedAt))

			i2 := &image.Image{}
			err = db.QueryRowx("SELECT * FROM images WHERE id = $1", img.ID).StructScan(i2)
			Expect(err).To(BeNil())
			Expect(img).To(Equal(i2))
		})
	})

	Describe("InsertOrUpdate()", func() {
		Context("when image ID is not set (i.e. zero value)", func() {
			It("persists a new image record in the database", func() {
				hs, err := test.CreateHostStack(db, nil)
				Expect(err).To(BeNil())

				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM images;").Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))

				i := &image.Image{
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err = i.InsertOrUpdate(db)
				Expect(err).To(BeNil())

				err = db.QueryRow("SELECT COUNT(*) FROM images;").Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
				i2 := &image.Image{}
				err = db.QueryRowx("SELECT * FROM images WHERE id = $1", i.ID).StructScan(i2)
				Expect(err).To(BeNil())
				Expect(i).To(Equal(i2))
			})
		})

		Context("when image ID is set to an existing ID", func() {
			It("updates the image in the database", func() {
				hs, err := test.CreateHostStack(db, nil)
				Expect(err).To(BeNil())
				_, err = db.Exec(`INSERT INTO images (user_id, host_id, freighter_id, name, slug, tag)
					VALUES ($1, $2, $3, $4, $5, $6);`,
					hs.U.ID, hs.H.ID, "cafebabe", "scratch image", "scratch", "latest")
				Expect(err).To(BeNil())

				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM images;").Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				img, err := image.Find(db, hs.U, hs.H, "scratch:latest")
				Expect(err).To(BeNil())
				Expect(img.FreighterID).To(Equal("cafebabe"))

				img.FreighterID = "cafedead"
				img.Name = "scratch image v2"
				err = img.InsertOrUpdate(db)
				Expect(err).To(BeNil())

				err = db.QueryRow("SELECT COUNT(*) FROM images;").Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				i2 := &image.Image{}
				err = db.QueryRowx("SELECT * FROM images WHERE id = $1", img.ID).StructScan(i2)
				Expect(err).To(BeNil())
				Expect(img).To(Equal(i2))
			})
		})

		Context("when image ID is set to an ID that does not exist", func() {
			It("returns an error", func() {
				hs, err := test.CreateHostStack(db, nil)
				Expect(err).To(BeNil())

				i := &image.Image{
					ID:     99999,
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err = i.InsertOrUpdate(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FullRepository()", func() {
		It("returns slug:tag", func() {
			i := &image.Image{Slug: "scratch", Tag: "mytag"}
			Expect(i.FullRepository()).To(Equal("scratch:mytag"))
		})

		It("defaults tag to 'latest'", func() {
			i := &image.Image{Slug: "scratch"}
			Expect(i.FullRepository()).To(Equal("scratch:latest"))
		})
	})

	Describe("ParseRepoAndTag()", func() {
		testCases := []struct {
			input, repo, tag string
		}{
			{input: "ubuntu", repo: "ubuntu", tag: ""},
			{input: "ubuntu:14.04", repo: "ubuntu", tag: "14.04"},
			{input: "nitrousio/ubuntu", repo: "nitrousio/ubuntu", tag: ""},
			{input: "nitrousio/ubuntu:14.04", repo: "nitrousio/ubuntu", tag: "14.04"},
			{input: "example.com:5000/ubuntu", repo: "example.com:5000/ubuntu", tag: ""},
			{input: "example.com:5000/ubuntu:14.04", repo: "example.com:5000/ubuntu", tag: "14.04"},
			{input: "example.com:5000/nitrousio/ubuntu", repo: "example.com:5000/nitrousio/ubuntu", tag: ""},
			{input: "example.com:5000/nitrousio/ubuntu:14.04", repo: "example.com:5000/nitrousio/ubuntu", tag: "14.04"},
		}
		for _, tc2 := range testCases {
			tc := tc2
			It(fmt.Sprintf("parses %#v to (%#v, %#v)", tc.input, tc.repo, tc.tag), func() {
				repo, tag := parsers.ParseRepoAndTag(tc.input)
				Expect(repo).To(Equal(tc.repo))
				Expect(tag).To(Equal(tc.tag))
			})
		}
	})
})
