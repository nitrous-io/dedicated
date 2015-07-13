package coupon_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/coupon"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/coupon")
}

var _ = Describe("models/coupon", func() {
	var (
		db *db.Database
		u  *user.User
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
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Find()", func() {
		It("returns the coupon belonging to the user with the given ID", func() {
			// Insert a coupon.
			_, err := db.Exec(`INSERT INTO coupons (user_id, code) VALUES ($1, $2);`, u.ID, "test-coupon")
			Expect(err).To(BeNil())

			cp, err := coupon.Find(db, u.ID)
			Expect(err).To(BeNil())

			Expect(cp.UserID).To(Equal(u.ID))
			Expect(cp.Code).To(Equal("test-coupon"))
		})

		It("returns an error when the user has no coupons", func() {
			_, err := coupon.Find(db, u.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("returns an error when the user has a used coupon", func() {
			// Insert a used coupon.
			_, err := db.Exec(`INSERT INTO coupons (user_id, code, used_at) VALUES ($1, $2, now());`, u.ID, "test-coupon")
			Expect(err).To(BeNil())

			_, err = coupon.Find(db, u.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})
	})

	Describe("Insert()", func() {
		It("persists the coupon in the database", func() {
			cp := &coupon.Coupon{
				UserID: u.ID,
				Code:   "test-code",
			}
			err := cp.Insert(db)
			Expect(err).To(BeNil())

			cp2 := &coupon.Coupon{}
			err = db.Get(cp2, `SELECT * FROM coupons WHERE id = $1;`, cp.ID)
			Expect(err).To(BeNil())

			Expect(cp2.UserID).To(Equal(u.ID))
			Expect(cp2.Code).To(Equal("test-code"))
			Expect(cp2.CreatedAt.Time).ToNot(BeZero())
			Expect(cp2.UpdatedAt.Time).ToNot(BeZero())
		})

		It("fills up ID and other fields in the original struct", func() {
			cp := &coupon.Coupon{
				UserID: u.ID,
				Code:   "test-code",
			}
			err := cp.Insert(db)
			Expect(err).To(BeNil())

			Expect(cp.ID).ToNot(BeZero())
			Expect(cp.CreatedAt.Time).ToNot(BeZero())
			Expect(cp.UpdatedAt.Time).ToNot(BeZero())
		})
	})

	Describe("MarkAsUsed()", func() {
		It("updates the used_at field", func() {
			// Insert a coupon.
			_, err := db.Exec(`INSERT INTO coupons (user_id, code) VALUES ($1, $2);`, u.ID, "test-coupon")
			Expect(err).To(BeNil())

			cp, err := coupon.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(cp.UsedAt.Valid).To(BeFalse())
			Expect(cp.UsedAt.Time).To(BeZero())

			err = cp.MarkAsUsed(db)
			Expect(err).To(BeNil())
			Expect(cp.UsedAt.Valid).To(BeTrue())
			Expect(cp.UsedAt.Time).ToNot(BeZero())
		})
	})
})
