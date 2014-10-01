package token_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/token")
}

var _ = Describe("models/token", func() {
	var (
		u1 *user.User
		u2 *user.User
		u3 *user.User
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u1 = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = WrapInTransaction(db, u1.Insert)
		Expect(err).To(BeNil())

		u2 = &user.User{
			Username: "bar",
			Email:    "bar@example.com",
			Password: "123456",
		}
		err = WrapInTransaction(db, u2.Insert)
		Expect(err).To(BeNil())

		u3 = &user.User{
			Username: "baz",
			Email:    "baz@example.com",
			Password: "123456",
		}
		err = WrapInTransaction(db, u3.Insert)
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u1.ID, "foofoo")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u2.ID, "barbar")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByToken()", func() {
		Context("when a token record with a given token exists", func() {
			It("returns the token record", func() {
				t, err := token.FindByToken(db, "foofoo")
				Expect(err).To(BeNil())
				Expect(t).NotTo(BeNil())
				Expect(t.ID).NotTo(BeZero())
				Expect(t.UserID).To(Equal(u1.ID))
				Expect(t.Token).To(Equal("foofoo"))

				t, err = token.FindByToken(db, "barbar")
				Expect(err).To(BeNil())
				Expect(t).NotTo(BeNil())
				Expect(t.ID).NotTo(BeZero())
				Expect(t.UserID).To(Equal(u2.ID))
				Expect(t.Token).To(Equal("barbar"))
			})
		})

		Context("when a token record with a given user ID does not exist", func() {
			It("returns nil", func() {
				t, err := token.FindByToken(db, "batbat")
				Expect(t).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				t, err := token.FindByToken(db, "foofoo")
				Expect(t).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByUser()", func() {
		Context("when a token record with a given user id exists", func() {
			It("returns the token record", func() {
				t, err := token.FindByUser(db, u1.ID)
				Expect(err).To(BeNil())
				Expect(t).NotTo(BeNil())
				Expect(t.ID).NotTo(BeZero())
				Expect(t.UserID).To(Equal(u1.ID))
				Expect(t.Token).To(Equal("foofoo"))

				t, err = token.FindByUser(db, u2.ID)
				Expect(err).To(BeNil())
				Expect(t).NotTo(BeNil())
				Expect(t.ID).NotTo(BeZero())
				Expect(t.UserID).To(Equal(u2.ID))
				Expect(t.Token).To(Equal("barbar"))
			})
		})

		Context("when a token record with a given user id does not exist", func() {
			It("returns nil", func() {
				t, err := token.FindByUser(db, u3.ID)
				Expect(t).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				t, err := token.FindByUser(db, u1.ID)
				Expect(t).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var t *token.Token

		BeforeEach(func() {
			t = &token.Token{UserID: u1.ID}
		})

		Context("when no error happens", func() {
			var t2 *token.Token

			Context("when Token field is not empty", func() {
				BeforeEach(func() {
					t.Token = "foobar"
					err := t.Insert(db)
					Expect(err).To(BeNil())
					t2, err = token.FindByToken(db, "foobar")
					Expect(err).To(BeNil())
					Expect(t2).NotTo(BeNil())
				})

				It("persists the token in the database", func() {
					Expect(t2.ID).NotTo(BeZero())
					Expect(t2.UserID).To(Equal(u1.ID))
					Expect(t2.Token).To(Equal("foobar"))
					Expect(t2.CreatedAt).NotTo(BeNil())
				})

				It("fills up ID and other fields in original struct", func() {
					Expect(t.ID).NotTo(BeZero())
					Expect(t.CreatedAt).NotTo(BeNil())
				})
			})

			Context("when Token field is empty", func() {
				BeforeEach(func() {
					t.Token = ""
					err := t.Insert(db)
					Expect(err).To(BeNil())
					Expect(t.Token).NotTo(BeEmpty())
					t2, err = token.FindByToken(db, t.Token)
					Expect(err).To(BeNil())
					Expect(t2).NotTo(BeNil())
				})

				It("persists the token in the database", func() {
					Expect(t2.ID).NotTo(BeZero())
					Expect(t2.UserID).To(Equal(u1.ID))
					Expect(t2.CreatedAt).NotTo(BeNil())
				})

				It("fills up ID and other fields in original struct", func() {
					Expect(t.ID).NotTo(BeZero())
					Expect(t.CreatedAt).NotTo(BeNil())
				})

				It("generates a random secure token", func() {
					Expect(t2.Token).NotTo(BeEmpty())
					Expect(t2.Token).To(HaveLen(255))
					Expect(t2.Token).To(Equal(t.Token))

					t3 := &token.Token{UserID: u1.ID}
					err := t3.Insert(db)
					Expect(err).To(BeNil())
					Expect(t3.Token).NotTo(BeEmpty())
					Expect(t3.Token).To(HaveLen(255))
					Expect(t3.Token).NotTo(Equal(t2.Token))
				})
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				err := t.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
