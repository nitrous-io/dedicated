package github_test

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	gh "github.com/nitrous-io/dedicated/nitrous/models/github"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/github")
}

var _ = Describe("models/github", func() {
	var (
		db *ndb.Database
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	Describe("Exceeded", func() {
		It("adds new record if does not exists or updates if exists", func() {
			exceeded, err := gh.Exceeded(db, u.ID)
			Expect(err).To(BeNil())

			Expect(exceeded).To(BeFalse())
			g, err := gh.FindByUserID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(g.UserID).To(Equal(u.ID))
			Expect(g.Threshold).To(Equal(10))

			timestamps, err := g.Accesses()
			Expect(err).To(BeNil())
			Expect(timestamps).To(ConsistOf(BeNumerically("~", time.Now().Unix(), 1)))

			exceeded1, err := gh.Exceeded(db, u.ID)
			Expect(err).To(BeNil())

			Expect(exceeded1).To(BeFalse())
			g1, err := gh.FindByUserID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(g1.UserID).To(Equal(u.ID))
			Expect(g1.Threshold).To(Equal(10))

			timestamps1, err := g1.Accesses()
			Expect(err).To(BeNil())
			Expect(timestamps1).To(ConsistOf(
				BeNumerically("~", time.Now().Unix(), 1),
				BeNumerically("~", time.Now().Unix(), 1),
			))
		})

		It("returns true if threshold exceeded", func() {
			g := &gh.GitHub{
				UserID: u.ID,
			}
			err := g.Insert(db, strconv.Itoa(int(time.Now().Add(-30*time.Minute).Unix())))
			Expect(err).To(BeNil())
			err = g.Update(db, strconv.Itoa(int(time.Now().Add(-25*time.Minute).Unix())))
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE github SET threshold=1 where user_id=$1;`, u.ID)
			Expect(err).To(BeNil())

			exceeded, err := gh.Exceeded(db, u.ID)
			Expect(err).To(BeNil())

			Expect(exceeded).To(BeTrue())
		})

		It("removes expired unix timestamp from accesses_at", func() {
			g := &gh.GitHub{
				UserID: u.ID,
			}
			now := time.Now()

			err := g.Insert(db, strconv.Itoa(int(now.Unix())))
			Expect(err).To(BeNil())
			err = g.Update(db, strconv.Itoa(int(now.Add(-2*time.Hour).Unix())))
			Expect(err).To(BeNil())

			_, err = gh.Exceeded(db, u.ID)
			Expect(err).To(BeNil())

			g1, err := gh.FindByUserID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(g1.AccessesAt).To(ContainSubstring(strconv.Itoa(int(now.Unix()))))
			Expect(g1.AccessesAt).To(ContainSubstring(","))

			timestamps, err := g1.Accesses()
			Expect(err).To(BeNil())
			Expect(timestamps).To(ConsistOf(
				BeNumerically("~", now.Unix(), 1),
				BeNumerically("~", now.Unix(), 1),
			))
		})
	})

	Describe("Insert", func() {
		It("adds a new github record", func() {
			g := &gh.GitHub{
				UserID: u.ID,
			}
			err := g.Insert(db, strconv.Itoa(int(time.Now().Add(-30*time.Minute).Unix())))
			Expect(err).To(BeNil())

			g1 := &gh.GitHub{}
			Expect(db.Get(g1, "SELECT * FROM github WHERE user_id = $1;", u.ID)).To(BeNil())
			Expect(g1).To(Equal(g))
			Expect(g1.UserID).To(Equal(u.ID))
			Expect(g1.Threshold).To(Equal(10))

			timestamps, err := g1.Accesses()
			Expect(err).To(BeNil())
			Expect(timestamps).To(ConsistOf(BeNumerically("~", time.Now().Add(-30*time.Minute).Unix(), 1)))
		})
	})

	Describe("Update", func() {
		It("updates the existing github record by appending to accesses_at", func() {
			g := &gh.GitHub{
				UserID: u.ID,
			}
			err := g.Insert(db, strconv.Itoa(int(time.Now().Add(-30*time.Minute).Unix())))
			Expect(err).To(BeNil())

			err = g.Update(db, strconv.Itoa(int(time.Now().Add(-25*time.Minute).Unix())))
			Expect(err).To(BeNil())

			g1 := &gh.GitHub{}
			Expect(db.Get(g1, "SELECT * FROM github WHERE user_id = $1;", u.ID)).To(BeNil())
			Expect(g1).To(Equal(g))
			Expect(g1.UserID).To(Equal(u.ID))
			Expect(g1.Threshold).To(Equal(10))

			timestamps, err := g1.Accesses()
			Expect(err).To(BeNil())
			Expect(timestamps).To(ConsistOf(
				BeNumerically("~", time.Now().Add(-30*time.Minute).Unix(), 1),
				BeNumerically("~", time.Now().Add(-25*time.Minute).Unix(), 1),
			))
		})
	})

	Describe("FindByUserID", func() {
		var g *gh.GitHub

		BeforeEach(func() {
			g = &gh.GitHub{
				UserID: u.ID,
			}

			err := g.Insert(db, strconv.Itoa(int(time.Now().Add(-30*time.Minute).Unix())))
			Expect(err).To(BeNil())
		})

		It("returns existing github record by user ID", func() {
			g1, err := gh.FindByUserID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(g1).To(Equal(g))
			Expect(g1.UserID).To(Equal(u.ID))
			Expect(g1.Threshold).To(Equal(10))

			timestamps, err := g1.Accesses()
			Expect(err).To(BeNil())
			Expect(timestamps).To(ConsistOf(BeNumerically("~", time.Now().Add(-30*time.Minute).Unix(), 1)))
		})

		It("does not return github record if does not exist", func() {
			_, err := gh.FindByUserID(db, 12345)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

	})
})
