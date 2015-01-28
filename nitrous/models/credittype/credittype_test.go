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
		It("should find a regular credit type", func() {
			_, err := db.Exec("INSERT INTO credit_types (name, active, default_value) VALUES ($1, TRUE, 500);", credittype.SignupCreditType)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, credittype.SignupCreditType)
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal(credittype.SignupCreditType))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(500)))
			Expect(c.Stock).To(Equal(sql.NullInt64{0, false}))
			Expect(c.Used).To(Equal(sql.NullInt64{0, false}))
			Expect(c.ExpiresAt.Valid).To(BeFalse())
		})

		It("should find a credit type with zero used", func() {
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value, stock, used)
				VALUES ('10BONUS', TRUE, 1000, 300, 0);`)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "10BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("10BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(1000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{int64(300), true}))
			Expect(c.Used).To(Equal(sql.NullInt64{int64(0), true}))
			Expect(c.ExpiresAt.Valid).To(BeFalse())
		})

		It("should find a credit type where used is NULL", func() {
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value, stock)
				VALUES ('50BONUS', TRUE, 5000, 300);`)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "50BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("50BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(5000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{int64(300), true}))
			Expect(c.Used).To(Equal(sql.NullInt64{0, false}))
			Expect(c.ExpiresAt.Valid).To(BeFalse())
		})

		It("should find a credit type where stock is NULL", func() {
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value, used)
				VALUES ('60BONUS', TRUE, 6000, 300);`)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "60BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("60BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(6000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{0, false}))
			Expect(c.Used).To(Equal(sql.NullInt64{int64(300), true}))
			Expect(c.ExpiresAt.Valid).To(BeFalse())
		})

		It("should find a credit type where expires at is set in the future", func() {
			nextHour := time.Now().Add(1 * time.Hour)
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value, expires_at)
				VALUES ('70BONUS', TRUE, 7000, $1);`, pq.NullTime{nextHour, true})
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "70BONUS")
			Expect(err).To(BeNil())
			Expect(c).NotTo(BeNil())
			Expect(c.Name).To(Equal("70BONUS"))
			Expect(c.Active).To(BeTrue())
			Expect(c.DefaultValue).To(Equal(int64(7000)))
			Expect(c.Stock).To(Equal(sql.NullInt64{0, false}))
			Expect(c.Used).To(Equal(sql.NullInt64{0, false}))
			Expect(c.ExpiresAt.Valid).To(BeTrue())
			Expect(c.ExpiresAt.Time.Unix()).To(Equal(nextHour.Unix()))
		})

		It("should not find a credit type that isn't active", func() {
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value)
				VALUES ('act1', FALSE, 500);`)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "act1")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
		})

		It("should not find a credit type that has expired", func() {
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value, expires_at)
				VALUES ('act2', TRUE, 500, now() - interval '1 hour');`)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "act2")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
		})

		It("should not find a credit type that has reached its stock level", func() {
			_, err := db.Exec(`INSERT INTO credit_types (name, active, default_value, stock, used)
				VALUES ('act3', TRUE, 500, 10, 10);`)
			Expect(err).To(BeNil())

			c, err := credittype.FindValidCreditTypeByName(db, "act3")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
		})
	})
})
