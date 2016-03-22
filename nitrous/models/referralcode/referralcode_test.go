package referralcode_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/referralcode"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/referralcode")
}

var _ = Describe("models/referralcode", func() {
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
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Create", func() {
		It("created as part of user creation", func() {
			rc := &referralcode.ReferralCode{}
			err := db.Get(rc, "SELECT * FROM referral_codes WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())

			Expect(rc.UserID).To(Equal(u.ID))
			Expect(rc.Code).ToNot(BeNil())
			Expect(rc.Bonus).To(Equal(int64(10800)))
			Expect(rc.Cap).To(Equal(int64(10)))
			Expect(rc.CreatedAt).NotTo(BeNil())
			Expect(rc.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Use", func() {
		It("inserts record into referral code uses", func() {
			var code string
			err := db.Get(&code, `SELECT code FROM referral_codes WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			rcu, err := referralcode.Use(db, code, u2.ID)
			Expect(err).To(BeNil())

			rc := &referralcode.ReferralCode{}
			err = db.Get(rc, `SELECT * FROM referral_codes WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			Expect(rcu.UserID).To(Equal(u2.ID))
			Expect(rcu.ReferralCodeID).To(Equal(rc.ID))
		})

		It("grants bonus to referer and referee", func() {
			var origQuota int64
			err := db.Get(&origQuota, `SELECT usage_quota FROM users WHERE id = $1`, u.ID)
			Expect(err).To(BeNil())

			var code string
			err = db.Get(&code, `SELECT code FROM referral_codes WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			_, err = referralcode.Use(db, code, u2.ID)
			Expect(err).To(BeNil())

			rc := &referralcode.ReferralCode{}
			err = db.Get(rc, `SELECT * FROM referral_codes WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())
			Expect(rc.Bonus > 0).To(Equal(true))

			var updatedQuota int64
			err = db.Get(&updatedQuota, `SELECT usage_quota FROM users WHERE id = $1`, u.ID)
			Expect(err).To(BeNil())
			Expect(updatedQuota).To(Equal(origQuota + rc.Bonus))

			var updatedQuota2 int64
			err = db.Get(&updatedQuota2, `SELECT usage_quota FROM users WHERE id = $1`, u2.ID)
			Expect(err).To(BeNil())
			Expect(updatedQuota2).To(Equal(origQuota + rc.Bonus))
		})

		Context("when slug does not exist", func() {
			It("returns no rows", func() {
				_, err := referralcode.Use(db, "invalid-slug", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
			})

			It("does not insert referral code uses record", func() {
				rcu, err := referralcode.Use(db, "invalid-slug", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))

				Expect(rcu).To(BeNil())
			})

			It("does not grant bonuses", func() {
				var origQuota int64
				err := db.Get(&origQuota, `SELECT usage_quota FROM users WHERE id = $1`, u.ID)
				Expect(err).To(BeNil())

				_, err = referralcode.Use(db, "invalid-slug", u2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))

				rc := &referralcode.ReferralCode{}
				err = db.Get(rc, `SELECT * FROM referral_codes WHERE user_id = $1`, u.ID)
				Expect(err).To(BeNil())

				var updatedQuota int64
				err = db.Get(&updatedQuota, `SELECT usage_quota FROM users WHERE id = $1`, u.ID)
				Expect(err).To(BeNil())
				Expect(updatedQuota).To(Equal(origQuota))

				var updatedQuota2 int64
				err = db.Get(&updatedQuota2, `SELECT usage_quota FROM users WHERE id = $1`, u2.ID)
				Expect(err).To(BeNil())
				Expect(updatedQuota2).To(Equal(origQuota))
			})
		})

		Context("when exceeded cap", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE referral_codes SET cap = 0 WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("does not insert referral code uses record", func() {
				var code string
				err := db.Get(&code, `SELECT code FROM referral_codes WHERE user_id = $1`, u.ID)
				Expect(err).To(BeNil())

				rcu, err := referralcode.Use(db, code, u2.ID)
				Expect(err).To(BeNil())

				Expect(rcu).To(BeNil())
			})

			It("does not grant bonuses", func() {
				var origQuota int64
				err := db.Get(&origQuota, `SELECT usage_quota FROM users WHERE id = $1`, u.ID)
				Expect(err).To(BeNil())

				var code string
				err = db.Get(&code, `SELECT code FROM referral_codes WHERE user_id = $1`, u.ID)
				Expect(err).To(BeNil())

				_, err = referralcode.Use(db, code, u2.ID)
				Expect(err).To(BeNil())

				var updatedQuota int64
				err = db.Get(&updatedQuota, `SELECT usage_quota FROM users WHERE id = $1`, u.ID)
				Expect(err).To(BeNil())
				Expect(updatedQuota).To(Equal(origQuota))

				var updatedQuota2 int64
				err = db.Get(&updatedQuota2, `SELECT usage_quota FROM users WHERE id = $1`, u2.ID)
				Expect(err).To(BeNil())
				Expect(updatedQuota2).To(Equal(origQuota))
			})

			It("checks that referral code uses for a referral code does not exceed cap", func() {
				var code string
				err := db.Get(&code, `SELECT code FROM referral_codes WHERE user_id = $1`, u2.ID)
				Expect(err).To(BeNil())

				rcu, err := referralcode.Use(db, code, u2.ID)
				Expect(err).To(BeNil())

				rc := &referralcode.ReferralCode{}
				err = db.Get(rc, `SELECT * FROM referral_codes WHERE code = $1`, code)
				Expect(err).To(BeNil())

				Expect(rcu.UserID).To(Equal(u2.ID))
				Expect(rcu.ReferralCodeID).To(Equal(rc.ID))
			})

			It("inserts earned bonuses record if cap exceeded", func() {
				var code string
				err := db.Get(&code, `SELECT code FROM referral_codes WHERE user_id = $1`, u.ID)
				Expect(err).To(BeNil())

				_, err = referralcode.Use(db, code, u2.ID)
				Expect(err).To(BeNil())

				bn, err := bonus.FindBySlug(db, "refer-friend")
				Expect(err).To(BeNil())

				ebn := bonus.EarnedBonus{}
				err = db.Get(&ebn, `SELECT * FROM earned_bonuses WHERE user_id= $1`, u.ID)
				Expect(err).To(BeNil())

				Expect(ebn).ToNot(BeNil())
				Expect(ebn.BonusID).To(Equal(bn.ID))
			})

		})
	})
})
