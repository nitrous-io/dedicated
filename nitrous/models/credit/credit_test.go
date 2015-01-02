package credit_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/credit")
}

var _ = Describe("models/credit", func() {
	var (
		db *db.Database
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
			Name:     "Steve",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	Describe(".AvailableCredits()", func() {
		It("should return 0 if there are no credits", func() {
			sum, err := credit.AvailableCredits(db, u.ID)
			Expect(err).To(BeNil())
			Expect(sum).To(Equal(int64(0)))
		})

		It("should return the amount of available credits", func() {
			u2 := &user.User{
				Username: "fooey",
				Email:    "fooey@example.com",
				Password: "123456",
				Name:     "Steve",
			}
			err := WrapInTransaction(db, u2.Insert)

			_, err = db.Exec("INSERT INTO credits (user_id, amount) values ($1, 100);", u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO credits (user_id, amount) values ($1, -20);", u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO credits (user_id, amount) values ($1, 5);", u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO credits (user_id, amount) values ($1, 300);", u2.ID)
			Expect(err).To(BeNil())

			sum, err := credit.AvailableCredits(db, u.ID)
			Expect(err).To(BeNil())
			Expect(sum).To(Equal(int64(85)))
		})
	})

	Describe("Insert()", func() {
		It("should able to insert a record", func() {
			c := &credit.Credit{
				UserID: u.ID,
				Amount: 300,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			r := &credit.Credit{}
			err = db.QueryRowx("SELECT * FROM credits WHERE id = $1", c.ID).StructScan(r)
			Expect(err).To(BeNil())
			Expect(c).To(Equal(r))
		})

		It("should be able to reference credit types", func() {
			ct := credittype.CreditType{
				Name: "Something",
			}
			err := ct.Insert(db)
			Expect(err).To(BeNil())

			c := &credit.Credit{
				UserID:       u.ID,
				Amount:       300,
				CreditTypeID: sql.NullInt64{ct.ID, true},
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())

			r := &credit.Credit{}
			err = db.QueryRowx("SELECT credits.* FROM credits INNER JOIN credit_types ON credit_types.id = credits.credit_type_id WHERE credit_types.id = $1", ct.ID).StructScan(r)
			Expect(err).To(BeNil())
			Expect(c).To(Equal(r))
		})
	})
})
