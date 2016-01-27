package bonus_test

import (
	"database/sql"
	"testing"

	"github.com/lib/pq"
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
		db    *db.Database
		u, u2 *user.User
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

		bonuses := [][]interface{}{
			{"follow-twitter", int64(3 * 24 * 60 * 60)},
			{"tweet", int64(5 * 24 * 60 * 60)},
			{"like-facebook", int64(3 * 24 * 60 * 60)},
			{"post-facebook", int64(5 * 24 * 60 * 60)},
		}

		for _, bonus := range bonuses {
			_, err := db.Exec("INSERT INTO bonus (slug, amount) VALUES ($1, $2);", bonus[0].(string), bonus[1].(int64))
			Expect(err).To(BeNil())
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
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
			Expect(bn.Amount).To(Equal(int64(3 * 24 * 60 * 60)))
		})
	})

	Describe("Add", func() {
		It("creates a new record if does not exist and return error if record exists", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			bnu := &bonus.BonusUse{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = bnu.Add(db)
			Expect(err).To(BeNil())
			Expect(bnu.UserID).To(Equal(u.ID))
			Expect(bnu.BonusID).To(Equal(bn.ID))

			bnu2 := &bonus.BonusUse{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = bnu2.Add(db)
			Expect(err).ToNot(BeNil())
			Expect(err.(*pq.Error).Code.Name()).To(Equal("unique_violation"))
			Expect(bnu2.ID).To(BeZero())
		})

		It("creates a new record when there is an existing record for the user but for another bonus", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			bnu := &bonus.BonusUse{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = bnu.Add(db)
			Expect(err).To(BeNil())
			Expect(bnu.UserID).To(Equal(u.ID))
			Expect(bnu.BonusID).To(Equal(bn.ID))

			bn2, err := bonus.FindBySlug(db, "tweet")
			Expect(err).To(BeNil())

			bnu2 := &bonus.BonusUse{
				UserID:  u.ID,
				BonusID: bn2.ID,
			}

			err = bnu2.Add(db)
			Expect(err).To(BeNil())
			Expect(bnu2.UserID).To(Equal(u.ID))
			Expect(bnu2.BonusID).To(Equal(bn2.ID))
		})

		It("creates a new record when there is an existing record for the bonus but for another user", func() {
			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			bnu := &bonus.BonusUse{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = bnu.Add(db)
			Expect(err).To(BeNil())
			Expect(bnu.UserID).To(Equal(u.ID))
			Expect(bnu.BonusID).To(Equal(bn.ID))

			bnu2 := &bonus.BonusUse{
				UserID:  u2.ID,
				BonusID: bn.ID,
			}

			err = bnu2.Add(db)
			Expect(err).To(BeNil())
			Expect(bnu2.UserID).To(Equal(u2.ID))
			Expect(bnu2.BonusID).To(Equal(bn.ID))
		})
	})
})
