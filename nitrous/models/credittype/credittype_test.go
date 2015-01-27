package credittype_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/credittype")
}

var _ = Describe("models/credittype", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	Describe("Insert()", func() {
		It("should able to insert a record", func() {
			c := &credittype.CreditType{
				Name:         "Signup",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			r := &credittype.CreditType{}
			err = db.QueryRowx("SELECT * FROM credit_types WHERE id = $1", c.ID).StructScan(r)
			Expect(err).To(BeNil())
			Expect(c).To(Equal(r))
		})
	})

	Describe("FindValidCreditType()", func() {
		var futureTime time.Time
		verifyNone := func(code string) {
			ct, err := credittype.FindValidCreditType(db, code)
			Expect(err).To(BeNil())
			Expect(ct).To(BeNil())
			futureTime = time.Now().Add(1 * time.Hour)
		}

		It("should respond with a bad request if the promo code is used", func() {
			c := &credittype.CreditType{
				Name:         "nye2012",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
				ExpiresAt:    pq.NullTime{futureTime, true},
				Stock:        sql.NullInt64{20, true},
				Used:         sql.NullInt64{20, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			verifyNone("nye2012")
		})

		It("should respond with a bad request if the promo code is not active", func() {
			c := &credittype.CreditType{
				Name:         "nye2012",
				Description:  sql.NullString{"Desc", true},
				Active:       false,
				DefaultValue: 900,
				ExpiresAt:    pq.NullTime{futureTime, true},
				Stock:        sql.NullInt64{20, true},
				Used:         sql.NullInt64{19, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			verifyNone("nye2012")
		})

		It("should respond with a bad request if the promo code has expired", func() {
			t := time.Now().Add(-1 * time.Hour)
			c := &credittype.CreditType{
				Name:         "nye2012",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
				ExpiresAt:    pq.NullTime{t, true},
				Stock:        sql.NullInt64{20, true},
				Used:         sql.NullInt64{19, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			verifyNone("nye2012")
		})

		It("should find a record that never expires", func() {
			c := &credittype.CreditType{
				Name:         "nye2013",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
				Stock:        sql.NullInt64{20, true},
				Used:         sql.NullInt64{19, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			ct, err := credittype.FindValidCreditType(db, "nye2013")
			Expect(err).To(BeNil())
			Expect(ct).To(Equal(c))
		})

		It("should find a record with null stock", func() {
			c := &credittype.CreditType{
				Name:         "nye2013",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
				ExpiresAt:    pq.NullTime{futureTime, true},
				Used:         sql.NullInt64{19, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			ct, err := credittype.FindValidCreditType(db, "nye2013")
			Expect(err).To(BeNil())
			Expect(ct).To(Equal(c))
		})

		It("should find a record with null used", func() {
			c := &credittype.CreditType{
				Name:         "nye2013",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
				ExpiresAt:    pq.NullTime{futureTime, true},
				Stock:        sql.NullInt64{19, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			ct, err := credittype.FindValidCreditType(db, "nye2013")
			Expect(err).To(BeNil())
			Expect(ct).To(Equal(c))
		})
	})

	Describe("constraints", func() {
		It("should not allow you to use more than the available stock", func() {
			c := &credittype.CreditType{
				Name:         "Signup",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
				Stock:        sql.NullInt64{20, true},
				Used:         sql.NullInt64{20, true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE credit_types SET used = used + 1 WHERE id = $1;`, c.ID)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("violates check constraint"))
		})

		It("should allow you to increment used if stock is null", func() {
			c := &credittype.CreditType{
				Name:         "Signup",
				Description:  sql.NullString{"Desc", true},
				Active:       true,
				DefaultValue: 900,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE credit_types SET used = 92 WHERE id = $1;`, c.ID)
			Expect(err).To(BeNil())

			var used int64
			err = db.QueryRow(`SELECT used FROM credit_types WHERE id = $1;`, c.ID).Scan(&used)
			Expect(err).To(BeNil())
			Expect(used).To(Equal(int64(92)))
		})
	})
})
