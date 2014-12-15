package snapshot_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	. "github.com/nitrous-io/dedicated/nitrous/test"
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
		hs *HostStack
	)

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

	newSnapshot := func(size int64, url string) *snapshot.Snapshot {
		return &snapshot.Snapshot{
			HostID: hs.H.ID,
			Size:   size,
			Url:    url,
		}
	}

	Describe("Insert()", func() {
		It("persists the snapshot in the database", func() {
			s := &snapshot.Snapshot{
				HostID: hs.H.ID,
				Size:   5,
				Url:    "foo",
			}
			err := s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())
			Expect(s.HostID).To(Equal(hs.H.ID))
			Expect(s.Size).To(Equal(int64(5)))
			Expect(s.Url).To(Equal("foo"))
			Expect(s.CreatedAt).NotTo(BeNil())
			Expect(s.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Delete()", func() {
		var s *snapshot.Snapshot

		BeforeEach(func() {
			s = newSnapshot(5, "foo")
			err := s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())
		})

		It("removes the snapshot", func() {
			id := s.ID
			err := s.Delete(db)
			Expect(err).To(BeNil())

			err = db.Get(s, `SELECT * FROM snapshots WHERE id = $1`, id)
			Expect(err).To(MatchError(sql.ErrNoRows))
		})
	})

	Describe("Find()", func() {
		It("returns the matching record scoped by host", func() {
			s := newSnapshot(5, "foo")
			err := s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())

			found, err := snapshot.Find(db, hs.H.ID, s.ID)
			Expect(err).To(BeNil())
			Expect(found.ID).To(Equal(s.ID))
		})

		It("returns (nil, nil) on not found", func() {
			v, err := snapshot.Find(db, 12345, 10001)
			Expect(err).To(BeNil())
			Expect(v).To(BeNil())
		})
	})

	Describe("FindAll()", func() {
		It("finds all snapshots scoped by host", func() {
			s := newSnapshot(5, "foo")
			err := s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())

			s = newSnapshot(6, "bar")
			err = s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())

			found, err := snapshot.FindAll(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(found).To(HaveLen(2))
			Expect(found[0].Size).To(Equal(int64(5)))
			Expect(found[0].Url).To(Equal("foo"))
			Expect(found[1].Size).To(Equal(int64(6)))
			Expect(found[1].Url).To(Equal("bar"))
		})

		It("returns empty slice on not found", func() {
			v, err := snapshot.FindAll(db, 12345)
			Expect(err).To(BeNil())
			Expect(v).To(BeEmpty())
		})
	})

	Describe("Validate()", func() {
		// It("validates slug", func() {
		//   s := strings.Repeat("a", 128)

		//   tests := []struct {
		//     input string
		//     err   error
		//   }{
		//     {"", snapshot.ErrSlugBlank},
		//     {"f", snapshot.ErrSlugTooShort},
		//     {s, snapshot.ErrSlugTooLong},
		//     {s[:127], nil},
		//     {"f00", nil},
		//     {"f-o-0-", nil},
		//     {"-foo", snapshot.ErrSlugInvalid},
		//     {"foo/bar", snapshot.ErrSlugInvalid},
		//   }

		//   for _, t := range tests {
		//     v := &snapshot.Snapshot{Slug: t.input}
		//     Expect(v.Validate() == t.err).To(BeTrue(),
		//       "Expected '%s' to return error '%v'", t.input, t.err)
		//   }
		// })
	})
})
