package verification_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/verification"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/verification")
}

func isNumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

var _ = Describe("models/verification", func() {
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
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Generate()", func() {
		It("generates a verification for the user with a 6-character numeric code", func() {
			v, err := verification.Generate(db, u.ID, "+6588884444")
			Expect(err).To(BeNil())

			Expect(v.Code).To(HaveLen(6))
			Expect(isNumeric(v.Code)).To(BeTrue())
			Expect(v.UserID).To(Equal(u.ID))

			// Should be saved to database.
			v2, err := verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(BeNil())
			Expect(v).To(Equal(v2))
		})

		It("removes any existing verification for the user", func() {
			// Create an existing verification first.
			existingPhone := "+6588884444"
			_, err := verification.Generate(db, u.ID, existingPhone)
			Expect(err).To(BeNil())

			// Generate a new verification with a different phone.
			_, err = verification.Generate(db, u.ID, "+12345678")
			Expect(err).To(BeNil())

			_, err = verification.Find(db, u.ID, existingPhone)
			Expect(err).To(HaveOccurred())
		})
	})
})
