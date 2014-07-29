package volume_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/nitrous-io/nitrous/models/volume"

	ndb "github.com/nitrous-io/nitrous/db"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/volume")
}

var _ = Describe("models/volume", func() {
	var (
		db *ndb.Database
		hs *HostStack
	)

	Context("with db", func() {
		BeforeEach(func() {
			var err error
			db, err = TestDatabase("nitrous_test")
			Expect(err).To(BeNil())

			hs, err = CreateHostStack(db, nil)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if db != nil {
				db.Close()
			}
		})

		newVolume := func(slug string) *volume.Volume {
			return &volume.Volume{
				HostID: hs.H.ID,
				Slug:   slug,
			}
		}

		newHostVolume := func(slug string) (*HostStack, *volume.Volume) {
			hs2, err := CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			v := newVolume("data") // same slug
			v.HostID = hs2.H.ID
			err = v.Insert(db)
			Expect(err).To(BeNil())
			Expect(v.ID).NotTo(BeZero())
			return hs2, v
		}

		Describe("Insert()", func() {
			It("sets name and ID", func() {
				v := newVolume("docs")
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())
				Expect(v.Name).To(Equal("docs"))
			})
		})

		Describe("Delete()", func() {
			It("removes the volume", func() {
				v := newVolume("data")
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())

				id := v.ID
				err = v.Delete(db)
				Expect(err).To(BeNil())

				err = db.Get(v, `SELECT * FROM volumes WHERE id = $1`, id)
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Describe("FindByHostIDAndSlug()", func() {
			It("finds within host id", func() {
				v := newVolume("data")
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())
				hs2, v2 := newHostVolume("data")

				foundV, err := volume.FindByHostIDAndSlug(db, hs.H.ID, "data")
				Expect(err).To(BeNil())
				Expect(foundV.ID).To(Equal(v.ID))

				foundV2, err := volume.FindByHostIDAndSlug(db, hs2.H.ID, "data")
				Expect(err).To(BeNil())
				Expect(foundV2.ID).To(Equal(v2.ID))
			})

			It("returns (nil, nil) on not found", func() {
				v, err := volume.FindByHostIDAndSlug(db, 12345, "nope")
				Expect(err).To(BeNil())
				Expect(v).To(BeNil())
			})
		})

		Describe("FindAllByHostID()", func() {
			It("finds within host id", func() {
				v := newVolume("data")
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())
				hs2, v2 := newHostVolume("data")

				foundV, err := volume.FindAllByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(foundV).To(HaveLen(1))
				Expect(foundV[0].ID).To(Equal(v.ID))
				Expect(foundV[0].HostID).To(Equal(hs.H.ID))

				foundV2, err := volume.FindAllByHostID(db, hs2.H.ID)
				Expect(err).To(BeNil())
				Expect(foundV2).To(HaveLen(1))
				Expect(foundV2[0].ID).To(Equal(v2.ID))
				Expect(foundV2[0].HostID).To(Equal(hs2.H.ID))
			})

			It("returns empty slice on not found", func() {
				v, err := volume.FindAllByHostID(db, 12345)
				Expect(err).To(BeNil())
				Expect(v).To(BeEmpty())
			})
		})
	})

	Describe("Validate()", func() {
		It("validates slug", func() {
			s := strings.Repeat("a", 128)

			tests := []struct {
				input string
				err   error
			}{
				{"", volume.ErrSlugBlank},
				{"f", volume.ErrSlugTooShort},
				{s, volume.ErrSlugTooLong},
				{s[:127], nil},
				{"f00", nil},
				{"f-o-0-", nil},
				{"-foo", volume.ErrSlugInvalid},
				{"foo/bar", volume.ErrSlugInvalid},
			}

			for _, t := range tests {
				v := &volume.Volume{Slug: t.input}
				Expect(v.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})
})
