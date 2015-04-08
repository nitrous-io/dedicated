package cvolume_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/cvolume")
}

var _ = Describe("models/volume", func() {
	var (
		db  *ndb.Database
		hs  *test.HostStack
		cnr *container.Container
		vol *volume.Volume
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = test.CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		cnr = &container.Container{
			CID:    "cafebabe",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err = cnr.Insert(db)
		Expect(err).To(BeNil())
		vol = &volume.Volume{HostID: hs.H.ID, Slug: "music"}
		err = vol.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Find()", func() {
		It("finds given container and volume", func() {
			cv := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(cv.Insert(db)).To(BeNil())
			foundCV, err := cvolume.Find(db, cnr, vol)
			Expect(err).To(BeNil())
			Expect(foundCV.ID).To(Equal(cv.ID))
		})

		It("does not reveal associations of terminated containers and volume", func() {
			cv := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(cv.Insert(db)).To(BeNil())
			err := cnr.Delete(db)
			Expect(err).To(BeNil())
			_, err = cvolume.Find(db, cnr, vol)
			Expect(err).To(MatchError(sql.ErrNoRows))
		})

		It("returns ErrNoRows if not found", func() {
			cv, err := cvolume.Find(db, cnr, vol)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(cv).To(BeNil())
		})
	})

	Describe("FindAll()", func() {
		It("finds all the volumes associated with a given container", func() {
			cv := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(cv.Insert(db)).To(BeNil())

			vols, err := cvolume.FindAll(db, cnr)
			Expect(err).To(BeNil())
			Expect(vols[0].ID).To(Equal(vol.ID))
		})
	})

	Describe("DeleteContainer()", func() {
		It("deletes without removing other containers", func() {
			delCnr := &container.Container{
				CID:    "123456",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-staging-container",
			}
			err := delCnr.Insert(db)
			Expect(err).To(BeNil())

			toDelete := &cvolume.ContainerVolume{ContainerID: delCnr.ID, VolumeID: vol.ID}
			Expect(toDelete.Insert(db)).To(BeNil())
			toKeep := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(toKeep.Insert(db)).To(BeNil())

			err = cvolume.DeleteContainer(db, delCnr)
			Expect(err).To(BeNil())

			_, err = cvolume.Find(db, delCnr, vol)
			Expect(err).To(MatchError(sql.ErrNoRows))

			cv, err := cvolume.Find(db, cnr, vol)
			Expect(err).To(BeNil())
			Expect(cv.ID).To(Equal(toKeep.ID))
		})
	})

	Describe("Insert()", func() {
		It("saves", func() {
			cv := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(cv.Insert(db)).To(BeNil())
			Expect(cv.ContainerID).To(Equal(cnr.ID))
			Expect(cv.VolumeID).To(Equal(vol.ID))
		})

		It("returns error on duplicate", func() {
			cv := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(cv.Insert(db)).To(BeNil())
			err := cv.Insert(db)
			Expect(err).To(MatchError(cvolume.ErrAlreadyAssociated))
		})
	})

	Describe("Delete()", func() {
		It("removes", func() {
			cv := &cvolume.ContainerVolume{ContainerID: cnr.ID, VolumeID: vol.ID}
			Expect(cv.Insert(db)).To(BeNil())

			id := cv.ID

			Expect(cv.Delete(db)).To(BeNil())
			err := db.Get(cv, `SELECT * FROM container_volumes WHERE id = $1`, id)
			Expect(err).To(MatchError(sql.ErrNoRows))
		})
	})
})
