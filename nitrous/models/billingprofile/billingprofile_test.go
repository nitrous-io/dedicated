package billingprofile_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/billingprofile")
}

var _ = Describe("models/billingprofile", func() {
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
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Find()", func() {
		It("returns the billing profile for the given user ID", func() {
			p, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(p.ID).NotTo(BeNil())
			Expect(p.UserID).To(Equal(u.ID))
			Expect(p.State).To(Equal("free"))
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})

		Context("when there's no billing profile for the given user id", func() {
			It("returns nil", func() {
				p, err := billingprofile.Find(db, 9999)
				Expect(p).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				p, err := billingprofile.Find(db, u.ID)
				Expect(p).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Update()", func() {
		It("updates the profile with given parameters", func() {
			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())

			oldUpdatedAt := bp.UpdatedAt
			bp.StripeID = sql.NullString{"sc_002", true}

			err = bp.Update(db)
			Expect(err).To(BeNil())

			Expect(bp.StripeID.String).To(Equal("sc_002"))
			Expect(bp.UpdatedAt).NotTo(Equal(oldUpdatedAt))
		})
	})
})
