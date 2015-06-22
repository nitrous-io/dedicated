package token_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
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
		It("returns the token record when a token record with a given token exists", func() {
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

		It("returns error when a token record with a given user ID does not exist", func() {
			_, err := token.FindByToken(db, "batbat")
			Expect(err).To(Equal(token.ErrInvalidToken))
		})

		It("returns error when the token is expired", func() {
			t, _ := token.FindByToken(db, "barbar")
			db.Exec(`UPDATE tokens SET last_seen_at = $1 WHERE id = $2`,
				time.Now().Add(-token.MaxLastUsedDuration).Add(-time.Hour),
				t.ID)
			t, err := token.FindByToken(db, "barbar")
			Expect(err).To(Equal(token.ErrInvalidToken))
		})

		It("returns error when some error happens", func() {
			db.Close()
			t, err := token.FindByToken(db, "foofoo")
			Expect(t).To(BeNil())
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("FindByUser()", func() {
		It("returns the token record when a token record with a given user id exists", func() {
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

		It("returns err when a token record with a given user id does not exist ", func() {
			_, err := token.FindByUser(db, u3.ID)
			Expect(err).To(Equal(token.ErrInvalidToken))
		})

		It("returns error when the token is expired", func() {
			t, _ := token.FindByToken(db, "barbar")
			db.Exec(`UPDATE tokens SET last_seen_at = $1 WHERE id = $2`,
				time.Now().Add(-token.MaxLastUsedDuration).Add(-time.Hour),
				t.ID)
			_, err := token.FindByToken(db, "barbar")
			Expect(err).To(Equal(token.ErrInvalidToken))
		})

		It("returns error when some error happens", func() {
			db.Close()
			t, err := token.FindByUser(db, u1.ID)
			Expect(t).To(BeNil())
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("DeleteExpiredTokens()", func() {
		It("should delete outdated login tokens", func() {
			_, err := db.Exec("INSERT INTO tokens (user_id, token, last_seen_at) values ($1, $2, now() - interval '2 days');", u2.ID, "ABC2")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO tokens (user_id, token, last_seen_at) values ($1, $2, now() - interval '20 days');", u2.ID, "ABC3")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO tokens (user_id, token, last_seen_at) values ($1, $2, now() - interval '7 days');", u2.ID, "ABC4")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO tokens (user_id, token, last_seen_at) values ($1, $2, now() - interval '6 days');", u2.ID, "ABC5")
			Expect(err).To(BeNil())

			Expect(token.DeleteExpiredTokens(db)).To(BeNil())

			type con struct {
				Token string `db:"token"`
			}
			tokens := []con{}
			err = db.Select(&tokens, `SELECT token FROM tokens`)
			Expect(err).To(BeNil())

			Expect(tokens).NotTo(ContainElement(con{Token: "ABC3"}))
			Expect(tokens).NotTo(ContainElement(con{Token: "ABC4"}))
			Expect(tokens).To(ContainElement(con{Token: "ABC2"}))
			Expect(tokens).To(ContainElement(con{Token: "ABC5"}))
		})
	})

	Describe("Insert()", func() {
		var t *token.Token

		BeforeEach(func() {
			t = &token.Token{UserID: u1.ID, LastIP: sql.NullString{"127.0.0.1", true}}
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
					Expect(t2.LastIP.Valid).To(BeTrue())
					Expect(t2.LastIP.String).To(Equal("127.0.0.1"))
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

	Describe("Touch()", func() {
		It("should bump the last_seen_at", func() {
			t := &token.Token{UserID: u1.ID, Token: "foobar"}
			err := t.Insert(db)
			Expect(err).To(BeNil())

			Expect(t.Touch(db, "")).To(BeNil())
			now := time.Now()
			t2, err := token.FindByToken(db, "foobar")
			Expect(err).To(BeNil())
			Expect(t2).NotTo(BeNil())
			Expect(t2.LastSeenAt.Unix()).To(BeNumerically(">=", now.Unix()))
		})

		It("should bump the last_seen_at and set an IP if provided", func() {
			t := &token.Token{UserID: u1.ID, Token: "foobar"}
			err := t.Insert(db)
			Expect(err).To(BeNil())

			Expect(t.Touch(db, "192.168.1.254")).To(BeNil())
			now := time.Now()
			t2, err := token.FindByToken(db, "foobar")
			Expect(err).To(BeNil())
			Expect(t2).NotTo(BeNil())
			Expect(t2.LastSeenAt.Unix()).To(BeNumerically(">=", now.Unix()))
			Expect(t2.LastIP.Valid).To(BeTrue())
			Expect(t2.LastIP.String).To(Equal("192.168.1.254"))
		})
	})

	Describe("Delete()", func() {
		It("should delete the target token", func() {
			t := &token.Token{UserID: u1.ID, Token: "foobar"}
			err := t.Insert(db)
			Expect(err).To(BeNil())

			t2, err := token.FindByToken(db, "foobar")
			Expect(err).To(BeNil())
			Expect(t2).NotTo(BeNil())

			Expect(t2.Delete(db)).To(BeNil())
			_, err = token.FindByToken(db, "foobar")
			Expect(err).To(Equal(token.ErrInvalidToken))
		})
	})
})

// plain token tests
var _ = Describe("models/token", func() {
	var t *token.Token

	BeforeEach(func() {
		t = &token.Token{}
	})

	Describe("Scopes()", func() {
		It("should work for single scope", func() {
			t.Scope = "full"
			Expect(t.Scopes()).To(ConsistOf([]string{"full"}))

			Expect(t.HasScope("full")).To(BeTrue())
			Expect(t.HasScope("admin")).To(BeFalse())

			Expect(t.HasAllScopes([]string{"full"})).To(BeTrue())
			Expect(t.HasAllScopes([]string{"full", "admin"})).To(BeFalse())

			Expect(t.ExistScopes([]string{"full", "admin"})).To(BeTrue())
			Expect(t.ExistScopes([]string{"third_party", "admin"})).To(BeFalse())
		})

		It("should work for multiple scope", func() {
			t.Scope = "third_party container profile"
			Expect(t.Scopes()).To(ConsistOf([]string{"third_party", "container", "profile"}))

			Expect(t.HasScope("third_party")).To(BeTrue())
			Expect(t.HasScope("container")).To(BeTrue())
			Expect(t.HasScope("profile")).To(BeTrue())
			Expect(t.HasScope("full")).To(BeFalse())

			Expect(t.HasAllScopes([]string{"container", "profile"})).To(BeTrue())
			Expect(t.HasAllScopes([]string{"profile", "admin"})).To(BeFalse())

			Expect(t.ExistScopes([]string{"third_party", "full"})).To(BeTrue())
			Expect(t.ExistScopes([]string{"admin", "collab"})).To(BeFalse())
		})
	})

	Describe("IsValid()", func() {
		It("should be false if token is nil", func() {
			var t2 *token.Token
			Expect(t2.IsValid()).To(BeFalse())
		})

		It("should be true if the LastSeenAt is recent", func() {
			ti := time.Now()
			t.LastSeenAt = &ti
			Expect(t.IsValid()).To(BeTrue())
		})

		It("should be false if the LastSeenAt is not recent", func() {
			ti := time.Now().Add(token.MaxLastUsedDuration * -1)
			t.LastSeenAt = &ti
			Expect(t.IsValid()).To(BeFalse())
		})
	})
})
