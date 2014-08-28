package image_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/nitrous-io/freighter/provider/docker/parsers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/test"
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

	Describe("Find()", func() {
		It("returns the image record when given its Freighter ID", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO images (user_id, host_id, freighter_id, name, tag) values ($1, $2, $3, $4, $5);", hs.U.ID, hs.H.ID, "cafebabe", "scratch", "latest")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "cafebabe")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.UserID).To(Equal(hs.U.ID))
			Expect(img.HostID).To(Equal(hs.H.ID))
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Name).To(Equal("scratch"))
			Expect(img.Tag).To(Equal("latest"))
		})

		It("returns the image record when given its name", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO images (user_id, host_id, freighter_id, name, tag) values ($1, $2, $3, $4, $5);", hs.U.ID, hs.H.ID, "cafebabe", "scratch", "latest")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "scratch")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.UserID).To(Equal(hs.U.ID))
			Expect(img.HostID).To(Equal(hs.H.ID))
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Name).To(Equal("scratch"))
			Expect(img.Tag).To(Equal("latest"))
		})

		It("returns the image record when given its name with a tag", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO images (user_id, host_id, freighter_id, name, tag) values ($1, $2, $3, $4, $5);", hs.U.ID, hs.H.ID, "cafebabe", "scratch", "mytag")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "scratch:mytag")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.UserID).To(Equal(hs.U.ID))
			Expect(img.HostID).To(Equal(hs.H.ID))
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Name).To(Equal("scratch"))
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
				Name:        "scratch",
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
		It("removes the image", func() {
			hs, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO images (user_id, host_id, freighter_id, name, tag) values ($1, $2, $3, $4, $5);", hs.U.ID, hs.H.ID, "cafebabe", "scratch", "latest")
			Expect(err).To(BeNil())

			img, err := image.Find(db, hs.U, hs.H, "cafebabe")
			Expect(err).To(BeNil())
			Expect(img).NotTo(BeNil())
			Expect(img.FreighterID).To(Equal("cafebabe"))

			err = img.Delete(db)
			Expect(err).To(BeNil())

			err = db.Get(img, `SELECT * FROM images WHERE id = $1`, img.ID)
			Expect(err).To(MatchError(sql.ErrNoRows))
		})
	})

	Describe("ParseRepoAndTag", func() {
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
