package user_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/invitecode"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/user")
}

var _ = Describe("models/user", func() {
	var db *db.Database

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "foo", "foo@example.com", "foo bar", "foofoo")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "bar", "bar@example.com", "bar baz", "barbar")
		Expect(err).To(BeNil())
		// setting admin flag to true on last user
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password, admin) values ($1, $2, $3, $4, $5);", "baz", "baz@example.com", "baz qux", "bazbaz", true)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Encode as JSON", func() {
		It("returns the JSON record", func() {
			u, _ := user.FindByUsername(db, "foo")
			d, _ := json.Marshal(u)
			r := make(map[string]interface{})
			json.Unmarshal(d, &r)
			exposed := []string{"id", "name", "username", "email", "admin", "created_at", "updated_at"}
			for _, p := range exposed {
				Expect(r[p]).NotTo(BeNil(), "User JSON should contain %s", p)
				delete(r, p)
			}
			Expect(r).To(BeEmpty(), "Exposed more than what we need %v", r)
		})
	})

	Describe("FindByID()", func() {
		var fooID, barID, bazID int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM users WHERE username = $1", "foo").Scan(&fooID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "bar").Scan(&barID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "baz").Scan(&bazID)
			Expect(err).To(BeNil())
		})

		Context("when a user record with a given id exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByID(db, fooID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(fooID))
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))
				Expect(u.Admin).To(BeFalse())

				u, err = user.FindByID(db, barID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(barID))
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))
				Expect(u.Admin).To(BeFalse())

				u, err = user.FindByID(db, bazID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(bazID))
				Expect(u.Username).To(Equal("baz"))
				Expect(u.Email).To(Equal("baz@example.com"))
				Expect(u.Name).To(Equal("baz qux"))
				Expect(u.EncryptedPassword).To(Equal("bazbaz"))
				Expect(u.Admin).To(BeTrue())
			})
		})

		Context("when a user record with a given id does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByID(db, 9001)
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByID(db, fooID)
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByUsername()", func() {
		Context("when a user record with a given username exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))
				Expect(u.Admin).To(BeFalse())

				u, err = user.FindByUsername(db, "bar")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))
				Expect(u.Admin).To(BeFalse())

				u, err = user.FindByUsername(db, "baz")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("baz"))
				Expect(u.Email).To(Equal("baz@example.com"))
				Expect(u.Name).To(Equal("baz qux"))
				Expect(u.EncryptedPassword).To(Equal("bazbaz"))
				Expect(u.Admin).To(BeTrue())
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

	Describe("FindByUsernameOrEmail()", func() {
		Context("when a user record with a given username or email exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByUsernameOrEmail(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))
				Expect(u.Admin).To(BeFalse())

				u, err = user.FindByUsernameOrEmail(db, "bar@example.com")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))
				Expect(u.Admin).To(BeFalse())
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByUsernameOrEmail(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())

				u, err = user.FindByUsernameOrEmail(db, "bat@example.com")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByUsernameOrEmail(db, "foo")
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("GetDelay", func() {
		Context("when a user record with a given username or email exists", func() {
			AfterEach(func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
				Expect(err).To(BeNil())
			})

			It("returns a 0 delay when user has 0 login attempts", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
				Expect(err).To(BeNil())

				Expect(user.GetDelay(db, "foo")).To(Equal(time.Duration(0)))
			})

			It("returns a 0 delay when user has < 3 login attempts", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 2 WHERE username = $1", "foo")
				Expect(err).To(BeNil())

				Expect(user.GetDelay(db, "foo")).To(Equal(time.Duration(0)))
			})

			It("returns a 1 second delay when user has 3 < > 6 login attempts", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 5 WHERE username = $1", "foo")
				Expect(err).To(BeNil())

				Expect(user.GetDelay(db, "foo")).To(Equal(time.Second * 1))
			})

			It("returns a 3 second delay when user has 6 < > 9 login attempts", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 7 WHERE username = $1", "foo")
				Expect(err).To(BeNil())

				Expect(user.GetDelay(db, "foo")).To(Equal(time.Second * 3))
			})

			It("returns a 5 second delay when user has > 9 login attempts", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 15 WHERE username = $1", "foo")
				Expect(err).To(BeNil())

				Expect(user.GetDelay(db, "foo")).To(Equal(time.Second * 5))
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns a 0 delay ", func() {
				value, err := user.GetDelay(db, "bat")
				Expect(value).To(Equal(time.Duration(0)))
				Expect(err).To(BeNil())

				value, err = user.GetDelay(db, "bat@example.com")
				Expect(value).To(Equal(time.Duration(0)))
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				v, err := user.GetDelay(db, "foo")
				Expect(v).To(Equal(time.Duration(0)))
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("IncrementLoginAttempts", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 93 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 0 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		It("should allow for incrementing", func() {
			err := user.IncrementLoginAttempts(db, "foo")
			Expect(err).To(BeNil())

			err = user.IncrementLoginAttempts(db, "bar@example.com")
			Expect(err).To(BeNil())

			var value int
			err = db.QueryRowx("SELECT login_attempts FROM users WHERE username = 'foo'").Scan(&value)
			Expect(err).To(BeNil())
			Expect(value).To(Equal(1))

			err = db.QueryRowx("SELECT login_attempts FROM users WHERE email = 'bar@example.com'").Scan(&value)
			Expect(err).To(BeNil())
			Expect(value).To(Equal(94))
		})

		It("should not allow for overflows on the smallint", func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 32767 WHERE username = $1", "foo")
			Expect(err).To(BeNil())

			err = user.IncrementLoginAttempts(db, "foo")
			Expect(err).To(BeNil())

			var value int
			err = db.QueryRowx("SELECT login_attempts FROM users WHERE username = 'foo'").Scan(&value)
			Expect(err).To(BeNil())
			Expect(value).To(Equal(32767))
		})
	})

	Describe("ResetLoginAttempts", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 92 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 93 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 0 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		It("should allow for resetting", func() {
			err := user.ResetLoginAttempts(db, "foo")
			Expect(err).To(BeNil())

			err = user.ResetLoginAttempts(db, "bar@example.com")
			Expect(err).To(BeNil())

			var value int
			err = db.QueryRowx("SELECT login_attempts FROM users WHERE username = 'foo'").Scan(&value)
			Expect(err).To(BeNil())
			Expect(value).To(Equal(0))

			err = db.QueryRowx("SELECT login_attempts FROM users WHERE email = 'bar@example.com'").Scan(&value)
			Expect(err).To(BeNil())
			Expect(value).To(Equal(0))
		})
	})

	Describe("Insert()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
		})

		Context("when no error happens", func() {
			var u2 *user.User

			BeforeEach(func() {
				err := WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				u2, err = user.FindByUsername(db, "qux")
				Expect(err).To(BeNil())
				Expect(u2).NotTo(BeNil())
			})

			It("persists the user in the database", func() {
				Expect(u2.Username).To(Equal("qux"))
				Expect(u2.Name).To(Equal("quxboy"))
				Expect(u2.Email).To(Equal("qux@example.com"))
				Expect(u2.CreatedAt).NotTo(BeNil())
				Expect(u2.UpdatedAt).NotTo(BeNil())
			})

			It("encrypts the password", func() {
				Expect(u2.EncryptedPassword).NotTo(Equal("123456"))
				Expect(bcrypt.Verify(u2.EncryptedPassword, "123456")).To(BeTrue())
			})

			It("fills up ID and other fields in original struct", func() {
				Expect(u.ID).NotTo(BeZero())
				Expect(u2.Admin).To(BeFalse())
				Expect(u.CreatedAt).NotTo(BeNil())
				Expect(u.UpdatedAt).NotTo(BeNil())
				Expect(u.Password).To(BeEmpty()) // clears unencrypted password
				Expect(u.EncryptedPassword).NotTo(BeEmpty())
				Expect(u.InviteCodeID.Valid).To(BeFalse())
			})

			It("creates a user profile record", func() {
				up := &userprofile.UserProfile{}
				err := db.Get(up, "SELECT * FROM user_profiles WHERE user_profiles.user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(up.ID).NotTo(BeNil())
				Expect(up.UserID).To(Equal(u.ID))
				Expect(up.MaxHosts).To(Equal(int64(1)))
				Expect(up.CreatedAt).NotTo(BeNil())
				Expect(up.UpdatedAt).NotTo(BeNil())
			})

			It("creates a billing profile record", func() {
				bp := &billingprofile.BillingProfile{}
				err := db.Get(bp, "SELECT * FROM billing_profiles WHERE billing_profiles.user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(bp.ID).NotTo(BeNil())
				Expect(bp.UserID).To(Equal(u.ID))
				Expect(bp.State).To(Equal("free"))
				Expect(bp.CreatedAt).NotTo(BeNil())
				Expect(bp.UpdatedAt).NotTo(BeNil())
			})
		})

		Context("when some error occurrs during the transaction", func() {
			It("returns an the error", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				err = tx.Rollback()
				Expect(err).To(BeNil())

				err = u.Insert(tx)
				Expect(err).NotTo(BeNil())
			})
		})

		Context("with an invite code", func() {
			var ic *invitecode.InviteCode
			var u2 *user.User

			BeforeEach(func() {
				var err error
				ic, err = invitecode.GenerateInvite(db)
				Expect(err).To(BeNil())
				Expect(ic).ToNot(BeNil())
				u.InviteCodeID = sql.NullInt64{ic.ID, true}

				err = WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				u2, err = user.FindByUsername(db, "qux")
				Expect(err).To(BeNil())
				Expect(u2).NotTo(BeNil())
			})

			It("should assign the invite code", func() {
				Expect(u2.InviteCodeID.Int64).To(Equal(ic.ID))
			})
		})

		It("does not set the admin flag", func() {
			u.Admin = true

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			u2, err := user.FindByUsername(db, "qux")
			Expect(err).To(BeNil())
			Expect(u2).NotTo(BeNil())
			Expect(u2.Admin).To(BeFalse())
		})
	})

	Describe("Update()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates name, email, encrypted_password, updated_at", func() {
			u.Username = "nochange"
			u.Name = "qux jr"
			u.Password = "abcdef"
			u.Email = "quxjr@example.com"
			oldCreated, oldUpdated := u.CreatedAt, u.UpdatedAt
			t := time.Now()
			u.CreatedAt = &t

			Expect(u.Update(db)).To(BeNil())
			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Username).To(Equal("qux"))
			Expect(updated.Name).To(Equal("qux jr"))
			Expect(updated.Email).To(Equal("quxjr@example.com"))
			Expect(updated.Password).To(BeEmpty())
			Expect(bcrypt.Verify(updated.EncryptedPassword, "abcdef")).To(BeTrue())
			Expect(updated.CreatedAt).To(Equal(oldCreated))
			Expect(*updated.UpdatedAt).To(BeTemporally(">", *oldUpdated))
		})

		It("does not update encrypted password if password left blank", func() {
			u.Password = ""
			u.Email = "quxnopass@example.com"
			Expect(u.Update(db)).To(BeNil())
			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bcrypt.Verify(updated.EncryptedPassword, "123456")).To(BeTrue())
		})

		It("does not set the admin flag", func() {
			u.Admin = true
			Expect(u.Update(db)).To(BeNil())
			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Admin).To(BeFalse())
		})
	})

	Describe("Validate()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Email:    "foo@gmail.com",
				Username: "foo",
				Password: "123456",
			}
			Expect(u.Validate()).To(BeNil())
		})

		It("validates email", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", user.ErrEmailBlank},
				{"foo", user.ErrEmailInvalid},
				{"foo@gmail.com", nil},
				{"foo+bar@gmail.com", nil},
				{"foo+bar@hello.com.sg", nil},
			}

			for _, t := range tests {
				u.Email = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates username", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", user.ErrUsernameBlank},
				{"f", nil},
				{"ffffffffffffffffffffffffffffff", nil},
				{"fffffffffffffffffffffffffffffff", user.ErrUsernameTooLong},
				{"f00", nil},
				{"f-o-0-", nil},
				{"-foo", user.ErrUsernameInvalid},
				{"FOO", user.ErrUsernameInvalid},
			}

			for _, t := range tests {
				u.Username = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates password", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", user.ErrPasswordBlank},
				{"12345", user.ErrPasswordTooShort},
				{"123456", nil},
				{"fQ@*($&($()ZXv897ua]oo", nil},
				{"ffffffffffffffffffffffffffffff", nil},
				{"fffffffffffffffffffffffffffffff", user.ErrPasswordTooLong},
			}

			for _, t := range tests {
				u.Password = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}

			u.Password = ""
			u.EncryptedPassword = "something"
			Expect(u.Validate()).To(BeNil())
		})
	})

	Describe("Topic()", func() {
		It("returns a Topic based on the user's ID", func() {
			u := user.User{ID: 123}
			Expect(u.Topic()).To(Equal(pubsub.Topic("user-123")))
			Expect(u.Topic()).To(BeEquivalentTo("user-123"))
		})
	})

	Describe("CanCreateHosts()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET max_hosts = 2 WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("returns false, when owned hosts == max hosts", func() {
			hs, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())

			_, err = CreateHostStack(db, &HostStack{
				U:  u,
				R:  hs.R,
				P:  hs.P,
				PT: hs.PT,
			})
			Expect(err).To(BeNil())

			// max hosts for the user is set to 2
			// and user already has 2 hosts
			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeFalse())
		})

		It("returns true, when owned hosts < max hosts", func() {
			_, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())

			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeTrue())
		})

		It("returns true, when user has no hosts", func() {
			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeTrue())
		})

		It("ignores terminated hosts", func() {
			hs, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())
			err = hs.H.Delete(db)
			Expect(err).To(BeNil())

			hs2, err := CreateHostStack(db, &HostStack{
				U:  u,
				R:  hs.R,
				P:  hs.P,
				PT: hs.PT,
			})
			Expect(err).To(BeNil())
			err = hs2.H.Delete(db)
			Expect(err).To(BeNil())

			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeTrue())
		})
	})

	Describe("ConfirmationLapsed()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("should be false when the confirmation_sent_at is null", func() {
			fU, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fU.ConfirmationLapsed()).To(BeFalse())
		})

		It("should be false when the confirmation_sent_at is recent", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now() WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fU.ConfirmationLapsed()).To(BeFalse())
		})

		It("should be true when the confirmation_sent_at is later than allowed", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now() - interval '26 hours' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fU.ConfirmationLapsed()).To(BeTrue())
		})
	})

	Describe("FindByResetPasswordToken()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET max_hosts = 2 WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		Context("when a reset password token exist and it is not expired", func() {
			It("returns the matching user", func() {
				err := u.SetResetPasswordToken(db)
				Expect(err).To(BeNil())

				Expect(u.ResetPasswordToken.String).NotTo(BeEmpty())
				Expect(u.ResetPasswordSentAt).NotTo(BeNil())

				u1, err := user.FindByResetPasswordToken(db, u.ResetPasswordToken.String)
				Expect(err).To(BeNil())
				Expect(u1.ID).To(Equal(u.ID))
			})
		})

		Context("when a reset password token is expired", func() {
			It("returns nil", func() {
				err := u.SetResetPasswordToken(db)
				Expect(err).To(BeNil())

				Expect(u.ResetPasswordToken.String).NotTo(BeEmpty())
				Expect(u.ResetPasswordSentAt).NotTo(BeNil())

				_, err = db.Exec("UPDATE users SET reset_password_sent_at = now() - interval '23 hours' WHERE id = $1", u.ID)
				Expect(err).To(BeNil())

				u1, err := user.FindByResetPasswordToken(db, u.ResetPasswordToken.String)
				Expect(err).To(BeNil())
				Expect(u1).To(BeNil())
			})
		})
	})

	Describe("FindByConfirmationToken()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("returns the matching user if the token is valid", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now(), confirmation_token = 'ABCD' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByConfirmationToken(db, "ABCD")
			Expect(err).To(BeNil())
			Expect(fU.ID).To(Equal(u.ID))
		})

		It("returns the no user if the token is invalid", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now(), confirmation_token = 'ABCD' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByConfirmationToken(db, "ABCDE")
			Expect(err).To(BeNil())
			Expect(fU).To(BeNil())
		})
	})

	Describe("ConfirmEmail", func() {
		var u *user.User
		curTime := time.Now()

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("should successfully confirm when the time is within range", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			now := time.Now()
			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u.ConfirmationToken.String).To(BeEmpty())
			Expect(*u.ConfirmedAt).To(BeTemporally(">=", now))
			Expect(u.ConfirmationSentAt).To(BeNil())
		})

		It("should return an error if the user has already confirmed", func() {
			_, err := db.Exec(`UPDATE users SET confirmed_at = now() WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(Equal(user.ErrConfirmationAlreadyCompleted))
		})

		It("should return an error if confirmation has lapsed", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-73*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(Equal(user.ErrConfirmationLapsed))
			Expect(u.ConfirmationSentAt).ToNot(BeNil())
		})
	})

	Describe("SendConfirmationEmail", func() {
		var (
			u          *user.User
			fakeMailer *FakeMailer
			origMailer mailer.Mailer
		)

		BeforeEach(func() {
			origMailer = user.Mailer
			fakeMailer = &FakeMailer{}
			user.Mailer = fakeMailer

			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			user.Mailer = origMailer
		})

		It("should generate a confirmation token", func() {
			err := u.SendConfirmationEmail(db, true)
			Expect(err).To(BeNil())

			u, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(u.ConfirmationToken.String).ToNot(BeEmpty())
			Expect(u.ConfirmationToken.String).To(HaveLen(255))
		})

		It("should send the email", func() {
			os.Setenv("WEB_HOSTNAME", "nitrous.example")
			err := u.SendConfirmationEmail(db, true)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal("\"Nitrous Mailer\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"quxboy\" <qux@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal("\"Nitrous Mailer\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Subject).To(Equal("Nitrous - confirm your email"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello quxboy!"))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app#confirm-email?token=%s", u.ConfirmationToken.String)))
			Expect(fakeMailer.HTML).To(ContainSubstring("<h3>Hello quxboy!</h3>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("<a href=\"https://nitrous.example/app#confirm-email?token=%s\" target=\"_blank\"", u.ConfirmationToken.String)))
		})

		It("should return an error if already confirmed", func() {
			_, err := db.Exec(`UPDATE users SET confirmed_at = now() WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.SendConfirmationEmail(db, true)
			Expect(err).To(Equal(user.ErrConfirmationAlreadyCompleted))
		})

		It("should not reset the confirmation_sent_at if specified", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now() - interval '26 hours' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.SendConfirmationEmail(db, false)
			Expect(err).To(BeNil())

			u, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(u.ConfirmationLapsed()).To(BeTrue())
		})
	})
})
