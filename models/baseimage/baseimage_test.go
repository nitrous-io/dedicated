package baseimage_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/baseimage"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/baseimage")
}

var _ = Describe("models/baseimage", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("All()", func() {
		BeforeEach(func() {
			var err error
			_, err = db.Exec("INSERT INTO base_images (name, repo, description) values ($1, $2, $3);", "Go", "nitrousio/golang:latest", "")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO base_images (name, repo, description) values ($1, $2, $3);", "Node.js", "nitrousio/nodejs:latest", "")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO base_images (name, repo, description) values ($1, $2, $3);", "PHP", "nitrousio/php:latest", "")
			Expect(err).To(BeNil())
		})

		It("returns all base images", func() {
			imgs, err := baseimage.All(db)
			Expect(err).To(BeNil())
			Expect(imgs).To(HaveLen(3))
			Expect([]string{imgs[0].Name, imgs[1].Name, imgs[2].Name}).To(ConsistOf([]string{"Go", "Node.js", "PHP"}))
		})
	})

	Describe("Insert()", func() {
		It("persists the base image in the database", func() {
			var err error
			img := &baseimage.BaseImage{
				Name:        "Go",
				Repo:        "nitrousio/golang:latest",
				Description: "Go development environment",
			}
			err = img.Insert(db)
			Expect(err).To(BeNil())

			imgs, err := baseimage.All(db)
			b := imgs[0]
			Expect(err).To(BeNil())
			Expect(b).NotTo(BeNil())
			Expect(b.ID).NotTo(BeZero())
			Expect(b.Name).To(Equal("Go"))
			Expect(b.Repo).To(Equal("nitrousio/golang:latest"))
			Expect(b.Description).To(Equal("Go development environment"))
			Expect(b.CreatedAt).NotTo(BeNil())
			Expect(b.UpdatedAt).NotTo(BeNil())
		})
	})
})
