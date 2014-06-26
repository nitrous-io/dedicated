package usertest

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/user")
}

var _ = Describe("pkg/user", func() {
	var db *db.Database

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

	Describe("FindByUsername()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "foo", "foo@example.com", "foo bar", "foofoo")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "bar", "bar@example.com", "bar baz", "barbar")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "baz", "baz@example.com", "baz qux", "bazbaz")
			Expect(err).To(BeNil())
		})

		Context("when a user record with a given username exists", func() {
			It("finds a user record by a given username", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(int32(1)))
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))

				u, err = user.FindByUsername(db, "bar")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(int32(2)))
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))

				u, err = user.FindByUsername(db, "baz")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(int32(3)))
				Expect(u.Username).To(Equal("baz"))
				Expect(u.Email).To(Equal("baz@example.com"))
				Expect(u.Name).To(Equal("baz qux"))
				Expect(u.EncryptedPassword).To(Equal("bazbaz"))
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByUsername(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByUsername(db, "foo")
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
