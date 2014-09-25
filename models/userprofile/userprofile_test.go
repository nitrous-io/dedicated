package userprofile_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/models/userprofile"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/userprofile")
}

var _ = Describe("models/userprofile", func() {
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
		err = u.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Create()", func() {
		It("creates a new user profile", func() {
			tx, err := db.Beginx(db)
			Expect(err).To(BeNil())
			err := userprofile.Create(db, u.ID)
			Expect(err).To(BeNil())
			err = tx.Commit()
			Expect(err).To(BeNil())

			p := &userprofile.UserProfile{}
			err = db.Get(p, "SELECT * FROM user_profiles WHERE user_profiles.user_id = $1;", u.ID)
			Expect(err).To(BeNil())
			Expect(p.ID).NotTo(BeNil())
			Expect(p.UserID).To(Equal(u.ID))
			Expect(p.MaxHosts).To(Equal(int64(1)))
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})
	})
})
