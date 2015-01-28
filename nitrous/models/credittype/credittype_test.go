package credittype_test

import (
	"database/sql"
	"testing"
	"time"

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

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Insert()", func() {
		It("inserts a record", func() {
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

	Describe("FindValidCreditTypeByName()", func() {
		nextHour := time.Now().Add(time.Hour).Truncate(time.Second)

		BeforeEach(func() {
			var err error
			// default sign up bonus
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "default", true, 500, nil, nil, nil)
			Expect(err).To(BeNil())
			// valid promo code
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "10BONUS", true, 1000, sql.NullInt64{int64(300), true}, sql.NullInt64{int64(0), true}, nil)
			Expect(err).To(BeNil())
			// valid promo code - used is nil
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "50BONUS", true, 5000, sql.NullInt64{int64(300), true}, nil, nil)
			Expect(err).To(BeNil())
			// valid promo code - stock is nil
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "60BONUS", true, 6000, nil, sql.NullInt64{int64(300), true}, nil)
			Expect(err).To(BeNil())
			// valid promo code - not yet expired
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "70BONUS", true, 7000, nil, nil, nextHour)
			Expect(err).To(BeNil())
			// invalid promo code - inactive
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "11BONUS", false, 1100, sql.NullInt64{int64(300), true}, sql.NullInt64{int64(0), true}, nil)
			Expect(err).To(BeNil())
			// invalid promo code - used up
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "15BONUS", true, 1500, sql.NullInt64{int64(300), true}, sql.NullInt64{int64(300), true}, nil)
			Expect(err).To(BeNil())
			// invalid promo code - expired
			_, err = db.Exec("INSERT INTO credit_types (name, active, default_value, stock, used, expires_at) values ($1, $2, $3, $4, $5, $6);", "20BONUS", true, 2000, sql.NullInt64{int64(300), true}, sql.NullInt64{int64(0), true}, time.Now())
			Expect(err).To(BeNil())
		})

		It("returns a record if it is valid", func() {
			c, err := credittype.FindValidCreditTypeByName(db, "default")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("default"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(500)))
			Expect(c.Stock).To(Equal(sql.NullInt64{0, false}))
			Expect(c.Used).To(Equal(sql.NullInt64{0, false}))
			Expect(c.ExpiresAt).To(BeNil())

			c, err = credittype.FindValidCreditTypeByName(db, "10BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("10BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(1000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{int64(300), true}))
			Expect(c.Used).To(Equal(sql.NullInt64{int64(0), true}))
			Expect(c.ExpiresAt).To(BeNil())

			c, err = credittype.FindValidCreditTypeByName(db, "50BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("50BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(5000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{int64(300), true}))
			Expect(c.Used).To(Equal(sql.NullInt64{0, false}))
			Expect(c.ExpiresAt).To(BeNil())

			c, err = credittype.FindValidCreditTypeByName(db, "60BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("60BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(6000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{0, false}))
			Expect(c.Used).To(Equal(sql.NullInt64{int64(300), true}))
			Expect(c.ExpiresAt).To(BeNil())

			c, err = credittype.FindValidCreditTypeByName(db, "70BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("70BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(7000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{0, false}))
			Expect(c.Used).To(Equal(sql.NullInt64{0, false}))
			Expect(c.ExpiresAt).NotTo(BeNil())
			Expect(c.ExpiresAt.Unix()).To(Equal(nextHour.Unix()))
		})

		It("does not return a record if invalid", func() {
			c, err := credittype.FindValidCreditTypeByName(db, "11BONUS")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())

			c, err = credittype.FindValidCreditTypeByName(db, "15BONUS")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())

			c, err = credittype.FindValidCreditTypeByName(db, "20BONUS")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				c, err := credittype.FindValidCreditTypeByName(db, "default")
				Expect(c).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
