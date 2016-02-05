package bonus_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/bonus")
}

var _ = Describe("models/bonus", func() {
	var (
		db         *db.Database
		u, u2      *user.User
		threeHours = int64(3 * 60 * 60) // in seconds
		fiveHours  = int64(5 * 60 * 60)
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		u2 = &user.User{
			Username: "bar",
			Password: "123456",
			Email:    "bar@example.com",
		}
		Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

		bonuses := []struct {
			slug   string
			amount int64
		}{
			{"follow-twitter", threeHours},
			{"tweet", fiveHours},
			{"like-facebook", threeHours},
			{"share-facebook", fiveHours},
		}

		for _, bonus := range bonuses {
			_, err := db.Exec("INSERT INTO bonuses (slug, amount) VALUES ($1, $2);", bonus.slug, bonus.amount)
			Expect(err).To(BeNil())
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAll", func() {
		It("returns all bonus records", func() {
			bns, err := bonus.FindAll(db)
			Expect(err).To(BeNil())

			Expect(bns).To(HaveLen(4))
			Expect(bns[0].Slug).To(Equal("follow-twitter"))
			Expect(bns[0].Amount).To(Equal(threeHours))

			Expect(bns[1].Slug).To(Equal("tweet"))
			Expect(bns[1].Amount).To(Equal(fiveHours))
		})

		It("returns empty slice if record does not exist", func() {
			ebns, err := bonus.FindEarnedByUser(db, u.ID+9999)
			Expect(err).To(BeNil())
			Expect(ebns).To(HaveLen(0))
		})
	})

	Describe("FindBySlug", func() {
		It("should return an error if the slug does not exist", func() {
			bn, err := bonus.FindBySlug(db, "tweetter")
			Expect(err).To(Equal(sql.ErrNoRows))
			Expect(bn).To(BeNil())
		})

		It("should return the bonus if it exists", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())
			Expect(bn.Slug).To(Equal("follow-twitter"))
			Expect(bn.Amount).To(Equal(threeHours))
		})
	})

	Describe("Insert", func() {
		It("creates a new record if does not exist and returns error if record exists", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			ebn := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = ebn.Insert(db)
			Expect(err).To(BeNil())
			Expect(ebn.UserID).To(Equal(u.ID))
			Expect(ebn.BonusID).To(Equal(bn.ID))

			ebn2 := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = ebn2.Insert(db)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(errors.New("you have already earned this bonus")))
			Expect(ebn2.ID).To(BeZero())
		})

		It("creates a new record when there is an existing record for the user but for another bonus", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			ebn := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = ebn.Insert(db)
			Expect(err).To(BeNil())
			Expect(ebn.UserID).To(Equal(u.ID))
			Expect(ebn.BonusID).To(Equal(bn.ID))

			bn2, err := bonus.FindBySlug(db, "tweet")
			Expect(err).To(BeNil())

			ebn2 := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn2.ID,
			}

			err = ebn2.Insert(db)
			Expect(err).To(BeNil())
			Expect(ebn2.UserID).To(Equal(u.ID))
			Expect(ebn2.BonusID).To(Equal(bn2.ID))
		})

		It("creates a new record when there is an existing record for the bonus but for another user", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			ebn := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = ebn.Insert(db)
			Expect(err).To(BeNil())
			Expect(ebn.UserID).To(Equal(u.ID))
			Expect(ebn.BonusID).To(Equal(bn.ID))

			ebn2 := &bonus.EarnedBonus{
				UserID:  u2.ID,
				BonusID: bn.ID,
			}

			err = ebn2.Insert(db)
			Expect(err).To(BeNil())
			Expect(ebn2.UserID).To(Equal(u2.ID))
			Expect(ebn2.BonusID).To(Equal(bn.ID))
		})
	})

	Describe("FindEarnedByUser", func() {
		It("returns records belonging to user", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			ebn := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = ebn.Insert(db)
			Expect(err).To(BeNil())

			ebns, err := bonus.FindEarnedByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(ebns).To(HaveLen(1))
			Expect(ebns[0].BonusID).To(Equal(bn.ID))
		})

		It("returns empty slice if record does not exist", func() {
			ebns, err := bonus.FindEarnedByUser(db, u.ID+9999)
			Expect(err).To(BeNil())
			Expect(ebns).To(HaveLen(0))
		})
	})
})
