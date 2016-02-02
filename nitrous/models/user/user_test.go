package user_test

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailchimp"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	stripe "github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/user")
}

var _ = Describe("models/user", func() {
	var db *db.Database

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "foo", "foo@example.com", "foo bar", "foofoo")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "bar", "bar@example.com", "bar baz", "barbar")
		Expect(err).To(BeNil())
		// setting admin flag to true on last user
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password, admin, max_containers) values ($1, $2, $3, $4, $5, $6);", "baz", "baz@example.com", "baz qux", "bazbaz", true, 30)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password, deleted_at) values ($1, $2, $3, $4, now());", "dell", "dell@example.com", "baz qux", "bazbaz")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("RestoreUsageQuotas()", func() {
		var (
			u  *user.User
			hs *test.HostStack
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "aroon",
				Password: "123456",
				Email:    "aroon@zerobotlabs.com",
			}
			Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

			// Create a free host belonging to the user.
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: &host.Host{
					Slug: "free-host",
					Free: true,
				},
			})
			Expect(err).To(BeNil())
		})

		It("sets usage_notices_sent to 0 when it was last restored earlier than usage_quota_period ago", func() {
			quotaPeriod := time.Duration(u.UsageQuotaPeriod.Int64) * time.Second
			Expect(quotaPeriod > 0).To(Equal(true))
			now := time.Now()
			lastRestoredAt := now.Add(-quotaPeriod).Add(-time.Second)
			u.UsageQuotaRestoredAt = pq.NullTime{lastRestoredAt, true}
			Expect(u.Update(db)).To(BeNil())

			Expect(u.UsageNoticesSent.Int64).To(Equal(int64(0)))
			u.UsageNoticesSent = sql.NullInt64{user.USAGE_NOTICES_SECOND_WARNING, true}
			Expect(u.Update(db)).To(BeNil())
			Expect(u.UsageNoticesSent.Int64).To(Equal(int64(2)))

			_, err := user.RestoreUsageQuotas(db, now)
			Expect(err).To(BeNil())

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.UsageNoticesSent.Int64).To(Equal(int64(0)))
		})

		It("does not do anything if the usage was restored less than usage_quota_period ago", func() {
			// Set usage_quota_restored_at to later than usage_quota_period ago.
			quotaPeriod := time.Duration(u.UsageQuotaPeriod.Int64) * time.Second
			Expect(quotaPeriod > 0).To(Equal(true))
			now := time.Now()
			lastRestoredAt := now.Add(-quotaPeriod).Add(time.Second)
			u.UsageQuotaRestoredAt = pq.NullTime{lastRestoredAt, true}
			Expect(u.Update(db)).To(BeNil())

			numRestored, err := user.RestoreUsageQuotas(db, now)
			Expect(err).To(BeNil())
			Expect(numRestored).To(Equal(int64(0)))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			// The usage_quota_restored_at should be unchanged.
			Expect(updated.UsageQuotaRestoredAt.Time).To(BeTemporally("~", lastRestoredAt))
			Expect(*updated.UpdatedAt).To(BeTemporally("~", *u.UpdatedAt))
		})

		It("does not do anything if user's host is not free", func() {
			// Set user's host to "not free".
			_, err := db.Exec(`UPDATE hosts SET free = false WHERE ID = $1;`, hs.H.ID)
			Expect(err).To(BeNil())

			// Set usage_quota_restored_at to earlier than usage_quota_period ago.
			quotaPeriod := time.Duration(u.UsageQuotaPeriod.Int64) * time.Second
			Expect(quotaPeriod > 0).To(Equal(true))
			now := time.Now()
			lastRestoredAt := now.Add(-quotaPeriod).Add(-time.Second)
			u.UsageQuotaRestoredAt = pq.NullTime{lastRestoredAt, true}
			Expect(u.Update(db)).To(BeNil())

			numRestored, err := user.RestoreUsageQuotas(db, now)
			Expect(err).To(BeNil())
			Expect(numRestored).To(Equal(int64(0)))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			// The usage_quota_restored_at should be unchanged.
			Expect(updated.UsageQuotaRestoredAt.Time).To(BeTemporally("~", lastRestoredAt))
			Expect(*updated.UpdatedAt).To(BeTemporally("~", *u.UpdatedAt))
		})

		It("does not do anything if user does not have any hosts", func() {
			// Terminate user's free host.
			err := hs.H.Delete(db)
			Expect(err).To(BeNil())

			// Set usage_quota_restored_at to earlier than usage_quota_period ago.
			quotaPeriod := time.Duration(u.UsageQuotaPeriod.Int64) * time.Second
			Expect(quotaPeriod > 0).To(Equal(true))
			now := time.Now()
			lastRestoredAt := now.Add(-quotaPeriod).Add(-time.Second)
			u.UsageQuotaRestoredAt = pq.NullTime{lastRestoredAt, true}
			Expect(u.Update(db)).To(BeNil())

			numRestored, err := user.RestoreUsageQuotas(db, now)
			Expect(err).To(BeNil())
			Expect(numRestored).To(Equal(int64(0)))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			// The usage_quota_restored_at should be unchanged.
			Expect(updated.UsageQuotaRestoredAt.Time).To(BeTemporally("~", lastRestoredAt))
			Expect(*updated.UpdatedAt).To(BeTemporally("~", *u.UpdatedAt))
		})

		It("does not do anything if the usage_quota_restored_at is empty", func() {
			// Set usage_quota_restored_at to null.
			u.UsageQuotaRestoredAt = pq.NullTime{}
			Expect(u.Update(db)).To(BeNil())

			now := time.Now()
			numRestored, err := user.RestoreUsageQuotas(db, now)
			Expect(err).To(BeNil())
			Expect(numRestored).To(Equal(int64(0)))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			// The usage_quota_restored_at should be unchanged.
			Expect(updated.UsageQuotaRestoredAt.Valid).To(Equal(false))
			Expect(*updated.UpdatedAt).To(BeTemporally("~", *u.UpdatedAt))
		})
	})

	Describe("FindByID()", func() {
		var fooID, barID, bazID, dellID int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM users WHERE username = $1", "foo").Scan(&fooID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "bar").Scan(&barID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "baz").Scan(&bazID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "dell").Scan(&dellID)
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
				Expect(u.MaxContainers).To(Equal(sql.NullInt64{0, false}))

				u, err = user.FindByID(db, barID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(barID))
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))
				Expect(u.Admin).To(BeFalse())
				Expect(u.MaxContainers).To(Equal(sql.NullInt64{0, false}))

				u, err = user.FindByID(db, bazID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(bazID))
				Expect(u.Username).To(Equal("baz"))
				Expect(u.Email).To(Equal("baz@example.com"))
				Expect(u.Name).To(Equal("baz qux"))
				Expect(u.EncryptedPassword).To(Equal("bazbaz"))
				Expect(u.Admin).To(BeTrue())
				Expect(u.MaxContainers).To(Equal(sql.NullInt64{30, true}))
			})
		})

		Context("when a user record with a given id does not exist", func() {
			It("returns error", func() {
				u, err := user.FindByID(db, 9001)
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a user record has been deleted", func() {
			It("returns error", func() {
				u, err := user.FindByID(db, dellID)
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
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
			It("returns error", func() {
				u, err := user.FindByUsername(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a user record has been deleted", func() {
			It("returns error", func() {
				u, err := user.FindByUsername(db, "dell")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
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

	Describe("FindByEmail()", func() {
		Context("when a user record with a email exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByEmail(db, "bar@example.com")
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

		Context("when a username is provided", func() {
			It("returns error", func() {
				u, err := user.FindByEmail(db, "foo")
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns error", func() {
				u, err := user.FindByEmail(db, "bat@example.com")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a user record has been deleted", func() {
			It("returns error", func() {
				u, err := user.FindByEmail(db, "dell@example.com")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByEmail(db, "bar@example.com")
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
			It("returns error", func() {
				u, err := user.FindByUsernameOrEmail(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))

				u, err = user.FindByUsernameOrEmail(db, "bat@example.com")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a user record has been deleted", func() {
			It("returns error", func() {
				u, err := user.FindByUsernameOrEmail(db, "dell")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))

				u, err = user.FindByUsernameOrEmail(db, "dell@example.com")
				Expect(u).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
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
				Username:   "qux",
				Name:       "quxboy",
				Password:   "123456",
				Email:      "qux@example.com",
				Suspicious: sql.NullString{"shady", true},
			}
		})

		Context("when no error happens", func() {
			var u2 *user.User

			BeforeEach(func() {
				err := test.WrapInTransaction(db, u.Insert)
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
			})

			It("creates a user settings record", func() {
				settings := &user.Settings{}
				err := db.Get(settings, `SELECT * FROM user_settings WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())
				Expect(settings.UserID).To(Equal(u.ID))
				Expect(settings.CreatedAt).NotTo(BeNil())
				Expect(settings.UpdatedAt).NotTo(BeNil())
			})

			It("creates a user profile record", func() {
				up := &userprofile.UserProfile{}
				err := db.Get(up, "SELECT * FROM user_profiles WHERE user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(up.ID).NotTo(BeNil())
				Expect(up.UserID).To(Equal(u.ID))
				Expect(up.MaxHosts).To(Equal(int64(10)))
				Expect(up.CreatedAt).NotTo(BeNil())
				Expect(up.UpdatedAt).NotTo(BeNil())
			})

			It("creates a billing profile record", func() {
				bp := &billingprofile.BillingProfile{}
				err := db.Get(bp, "SELECT * FROM billing_profiles WHERE user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(bp.ID).NotTo(BeNil())
				Expect(bp.UserID).To(Equal(u.ID))
				Expect(bp.State).To(Equal("no_card"))
				Expect(bp.CreatedAt).NotTo(BeNil())
				Expect(bp.UpdatedAt).NotTo(BeNil())
			})

			It("writes suspicious field if populated", func() {
				Expect(u2.Username).To(Equal("qux"))
				Expect(u2.Suspicious.String).To(Equal("shady"))
			})
		})

		Context("when some error occurrs during the transaction", func() {
			It("returns the error", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				err = tx.Rollback()
				Expect(err).To(BeNil())

				err = u.Insert(tx)
				Expect(err).NotTo(BeNil())
			})
		})

		It("does not set the admin flag", func() {
			u.Admin = true

			err := test.WrapInTransaction(db, u.Insert)
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
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates name, email, unconfirmed_email, encrypted_password, confirmation_sent_at, confirmation_token, updated_at", func() {
			uTime := time.Now().Add(2 * time.Hour)
			u.Username = "nochange"
			u.Name = "qux jr"
			u.Password = "abcdef"
			u.Email = "quxjr@example.com"
			u.UnconfirmedEmail = sql.NullString{String: "un@example.com", Valid: true}
			u.ConfirmationSentAt = pq.NullTime{uTime, true}
			u.ConfirmationToken = sql.NullString{"hello", true}
			u.UsageQuotaRestoredAt = pq.NullTime{uTime.Add(time.Hour), true}
			oldCreated, oldUpdated := u.CreatedAt, u.UpdatedAt
			t := time.Now()
			u.CreatedAt = &t

			Expect(u.Update(db)).To(BeNil())
			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Username).To(Equal("qux"))
			Expect(updated.Name).To(Equal("qux jr"))
			Expect(updated.Email).To(Equal("quxjr@example.com"))
			Expect(updated.UnconfirmedEmail.String).To(Equal("un@example.com"))
			Expect(updated.Password).To(BeEmpty())
			Expect(bcrypt.Verify(updated.EncryptedPassword, "abcdef")).To(BeTrue())
			Expect(updated.ConfirmationSentAt.Valid).To(BeTrue())
			Expect(updated.ConfirmationSentAt.Time.Unix()).To(Equal(uTime.Unix()))
			Expect(updated.ConfirmationToken).To(Equal(sql.NullString{"hello", true}))
			Expect(updated.UsageQuotaRestoredAt.Time.Unix()).To(Equal(uTime.Add(time.Hour).Unix()))
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

	Describe("UpdatePhone()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates phone, phone_country_code and updated_at", func() {
			oldUpdated := u.UpdatedAt

			err := u.UpdatePhone(db, "62353535", "65")
			Expect(err).To(BeNil())

			Expect(u.Phone.String).To(Equal("62353535"))
			Expect(u.PhoneCountryCode.String).To(Equal("65"))
			Expect(*u.UpdatedAt).To(BeTemporally(">", *oldUpdated))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Phone.String).To(Equal("62353535"))
			Expect(updated.PhoneCountryCode.String).To(Equal("65"))
		})

		It("returns ErrPhoneInvalid if either phone or phone_country_code is empty", func() {
			err := u.UpdatePhone(db, "62353535", "")
			Expect(err).To(Equal(user.ErrPhoneInvalid))

			err = u.UpdatePhone(db, "", "65")
			Expect(err).To(Equal(user.ErrPhoneInvalid))

			err = u.UpdatePhone(db, "62353535", "65")
			Expect(err).To(BeNil())
		})

		It("does not update if phone_authed is already true", func() {
			_, err := db.Exec("UPDATE users SET phone_authed = $1 WHERE id = $2", true, u.ID)
			Expect(err).To(BeNil())

			err = u.UpdatePhone(db, "62353535", "65")
			Expect(err).To(Equal(sql.ErrNoRows))

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.Phone.Valid).To(BeFalse())
			Expect(u2.PhoneCountryCode.Valid).To(BeFalse())
		})
	})

	Describe("UpdatePhoneAuthed()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("returns ErrPhoneInvalid if user's phone or phone_country_code is empty", func() {
			err := u.UpdatePhoneAuthed(db, true)
			Expect(err).To(Equal(user.ErrPhoneInvalid))
		})

		It("updates phone_authed and updated_at", func() {
			// Update user's phone and phone_country_code first.
			err := u.UpdatePhone(db, "62353535", "65")
			Expect(err).To(BeNil())
			Expect(u.PhoneAuthed).To(BeFalse())
			oldUpdated := u.UpdatedAt

			err = u.UpdatePhoneAuthed(db, true)
			Expect(err).To(BeNil())

			Expect(u.PhoneAuthed).To(BeTrue())
			Expect(*u.UpdatedAt).To(BeTemporally(">", *oldUpdated))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.PhoneAuthed).To(BeTrue())
		})
	})

	Describe("FreeHostUsage()", func() {
		var (
			hs              *test.HostStack
			err             error
			quotaRestoredAt time.Time
		)

		BeforeEach(func() {
			hs, err = test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE hosts SET free = true WHERE id = $1`, hs.H.ID)
			Expect(err).To(BeNil())

			quotaRestoredAt = time.Date(2014, 1, 1, 0, 0, 0, 0, time.Local)
			_, err = db.Exec(`UPDATE users SET usage_quota_restored_at = $1 ;`, quotaRestoredAt)
			Expect(err).To(BeNil())
		})

		It(`returns sum of 'running' host transitions in seconds
			started at time before usage_quota_restored_at and
			ended at time after usage_quota_restored_at`, func() {
			startTime := quotaRestoredAt.Add(-1 * time.Hour)
			endTime := quotaRestoredAt.Add(1 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)
			Expect(err).To(BeNil())

			seconds, err := hs.U.FreeHostUsage(db, time.Now())
			Expect(err).To(BeNil())
			Expect(seconds).To(Equal(int64(3600)))
		})

		It(`returns 'running' host transition in seconds
			started at time before usage_quota_restored_at and still running`, func() {
			startTime := quotaRestoredAt.Add(-1 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at
				) values ($1, $2, $3, $4);
			`, hs.H.ID, "starting", "running", startTime)
			Expect(err).To(BeNil())

			fakeCurrentTime := quotaRestoredAt.Add(1000 * time.Hour)
			seconds, err := hs.U.FreeHostUsage(db, fakeCurrentTime)
			Expect(err).To(BeNil())
			Expect(seconds).To(Equal(int64(fakeCurrentTime.Sub(quotaRestoredAt).Seconds())))
		})

		It(`returns 'running' host transition in seconds
			started at time after usage_quota_restored_at and
			ended at sometime after started at`, func() {
			startTime := quotaRestoredAt.Add(2 * time.Hour)
			endTime := quotaRestoredAt.Add(3 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)
			Expect(err).To(BeNil())

			seconds, err := hs.U.FreeHostUsage(db, time.Now())
			Expect(err).To(BeNil())
			Expect(seconds).To(Equal(int64(3600)))
		})

		It(`returns 'running' host transtion in seconds
			started at time after usage_quota_restored_at and still running`, func() {
			startTime := quotaRestoredAt.Add(2 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at
				) values ($1, $2, $3, $4);
			`, hs.H.ID, "starting", "running", startTime)
			Expect(err).To(BeNil())

			fakeCurrentTime := quotaRestoredAt.Add(1000 * time.Hour)
			seconds, err := hs.U.FreeHostUsage(db, fakeCurrentTime)
			Expect(err).To(BeNil())
			Expect(seconds).To(Equal(int64(fakeCurrentTime.Sub(startTime).Seconds())))
		})

		It(`returns sum of a few 'running' host transtions in seconds
			started at time before usage_quota_restored_at`, func() {
			startTime := quotaRestoredAt.Add(-1 * time.Hour)
			endTime := quotaRestoredAt.Add(1 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "born", "running", startTime, endTime)

			startTime = quotaRestoredAt.Add(2 * time.Hour)
			endTime = quotaRestoredAt.Add(3 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)

			startTime = quotaRestoredAt.Add(5 * time.Hour)
			endTime = quotaRestoredAt.Add(7 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)

			seconds, err := hs.U.FreeHostUsage(db, time.Now())
			Expect(err).To(BeNil())
			Expect(seconds).To(Equal(int64(14400)))
		})

		It(`returns sum of a few 'running' host transtions of different hosts in seconds
			started at time before usage_quota_restored_at`, func() {
			startTime := quotaRestoredAt.Add(-1 * time.Hour)
			endTime := quotaRestoredAt.Add(1 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "born", "running", startTime, endTime)

			startTime = quotaRestoredAt.Add(2 * time.Hour)
			endTime = quotaRestoredAt.Add(3 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)

			startTime = quotaRestoredAt.Add(5 * time.Hour)
			endTime = quotaRestoredAt.Add(7 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)

			// Add a terminated host to user with 3 hours usage
			_, err = db.Exec(`
				INSERT INTO hosts (
					user_id, slug, region_id, plan_id, provider_template_id
				) VALUES ($1, $2, $3, $4, $5);
			`, hs.U.ID, "second-host", hs.R.ID, hs.P.ID, hs.PT.ID)
			Expect(err).To(BeNil())
			secondHost, err := host.Find(db, hs.U, "second-host")
			secondHost.Delete(db)
			Expect(err).To(BeNil())

			startTime = quotaRestoredAt.Add(5 * time.Hour)
			endTime = quotaRestoredAt.Add(8 * time.Hour)
			_, err = db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, secondHost.ID, "starting", "running", startTime, endTime)

			_, err = db.Exec(`UPDATE hosts SET free = true WHERE user_id = $1`, hs.U.ID)
			Expect(err).To(BeNil())

			seconds, err := hs.U.FreeHostUsage(db, time.Now())
			Expect(err).To(BeNil())
			Expect(seconds).To(Equal(int64(25200)))
		})
	})

	Describe("HasUsageRemaining()", func() {
		var (
			hs              *test.HostStack
			err             error
			quotaRestoredAt time.Time
		)

		BeforeEach(func() {
			hs, err = test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE hosts SET free = true WHERE id = $1`, hs.H.ID)
			Expect(err).To(BeNil())

			quotaRestoredAt = time.Date(2014, 1, 1, 0, 0, 0, 0, time.Local)
			_, err = db.Exec(`UPDATE users SET usage_quota_restored_at = $1 ;`, quotaRestoredAt)
			Expect(err).To(BeNil())

			hs.U.UsageQuota = sql.NullInt64{180000, true}
		})

		It("returns true if the free host usage is less than usage quota", func() {
			startTime := quotaRestoredAt.Add(5 * time.Hour)
			endTime := quotaRestoredAt.Add(7 * time.Hour)
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", hs.H.ID, "starting", "running", startTime, endTime)
			Expect(err).To(BeNil())
			Expect(hs.U.HasUsageRemaining(db)).To(Equal(true))
		})

		It("returns false if the free host usage is greater than usage quota", func() {
			startTime := quotaRestoredAt.Add(1 * time.Hour)
			endTime := quotaRestoredAt.Add(52 * time.Hour)
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", hs.H.ID, "starting", "running", startTime, endTime)
			Expect(err).To(BeNil())
			Expect(hs.U.HasUsageRemaining(db)).To(Equal(false))
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
				{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", user.ErrPasswordTooLong},
			}

			for _, t := range tests {
				u.Password = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}

			u.Password = ""
			u.EncryptedPassword = "something"
			Expect(u.Validate()).To(BeNil())
		})

		It("validates email with email blacklist", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"foo@yopmail.com", user.ErrEmailBlacklisted},
				{"foo@yopmail.fr", user.ErrEmailBlacklisted},
				{"foo@gmail.com", nil},
			}

			for _, t := range tests {
				u.Email = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}
		})
	})

	Describe("PhoneNumber()", func() {
		It("returns user's phone number with country code", func() {
			testCases := []struct {
				countryCode string
				phone       string
				expected    string
			}{
				{"1", "6175551212", "+16175551212"},
				{"65", "88884444", "+6588884444"},
				{"", "6175551212", ""},
				{"1", "", ""},
			}

			for _, t := range testCases {
				u := &user.User{
					Phone:            sql.NullString{t.phone, true},
					PhoneCountryCode: sql.NullString{t.countryCode, true},
				}
				Expect(u.PhoneNumber()).To(Equal(t.expected))
			}
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
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET max_hosts = 2 WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("does not return error for admin users", func() {
			u.Admin = true
			err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
		})

		It("returns no credit card error when billing profile is in no card state", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state = 'no_card' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = u.CanCreateHosts(db)
			Expect(err.Error()).To(Equal("not added credit card"))
		})

		It("returns max hosts error for non paid users when max hosts is reached", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state = 'exempt' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			// add 2 hosts, so we know it's not the MAX_HOSTS check lets him in.
			hs, err := test.CreateHostStack(db, &test.HostStack{U: u})
			Expect(err).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U:  u,
				R:  hs.R,
				P:  hs.P,
				PT: hs.PT,
			})
			Expect(err).To(BeNil())

			err = u.CanCreateHosts(db)
			Expect(err.Error()).To(Equal("reached maximum number of hosts"))
		})

		ItChecksNumberOfHosts := func() {
			It("returns returns max hosts error when owned hosts == max hosts", func() {
				hs, err := test.CreateHostStack(db, &test.HostStack{U: u})
				Expect(err).To(BeNil())

				_, err = test.CreateHostStack(db, &test.HostStack{
					U:  u,
					R:  hs.R,
					P:  hs.P,
					PT: hs.PT,
				})
				Expect(err).To(BeNil())

				// max hosts for the user is set to 2
				// and user already has 2 hosts
				err = u.CanCreateHosts(db)
				Expect(err.Error()).To(Equal("reached maximum number of hosts"))
			})

			It("does not return error when owned hosts < max hosts", func() {
				_, err := test.CreateHostStack(db, &test.HostStack{U: u})
				Expect(err).To(BeNil())

				err = u.CanCreateHosts(db)
				Expect(err).To(BeNil())
			})

			It("does not return error when user has no hosts", func() {
				err := u.CanCreateHosts(db)
				Expect(err).To(BeNil())
			})

			It("ignores terminated hosts", func() {
				hs, err := test.CreateHostStack(db, &test.HostStack{U: u})
				Expect(err).To(BeNil())
				err = hs.H.Delete(db)
				Expect(err).To(BeNil())

				hs2, err := test.CreateHostStack(db, &test.HostStack{
					U:  u,
					R:  hs.R,
					P:  hs.P,
					PT: hs.PT,
				})
				Expect(err).To(BeNil())
				err = hs2.H.Delete(db)
				Expect(err).To(BeNil())

				err = u.CanCreateHosts(db)
				Expect(err).To(BeNil())
			})
		}

		Context("for a paid user account", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = 'paid' WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			ItChecksNumberOfHosts()
		})

		Context("for a payment_due user account", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = 'payment_due' WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			ItChecksNumberOfHosts()
		})

		Context("for a exempt user account", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = 'exempt' WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			ItChecksNumberOfHosts()
		})
	})

	Describe("HasFailedPayments()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("should be false when no invoices exist", func() {
			state, err := u.HasFailedPayments(db)
			Expect(err).To(BeNil())
			Expect(state).To(BeFalse())
		})

		It("should be false if no invoices have failed", func() {
			_, err := db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'unpaid', now() - interval '2 hours', now());", u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'unpaid', now() - interval '4 hours', now() - interval '3 hours');", u.ID)
			Expect(err).To(BeNil())

			state, err := u.HasFailedPayments(db)
			Expect(err).To(BeNil())
			Expect(state).To(BeFalse())
		})

		It("should be true if any invoices have failed", func() {
			_, err := db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'failed', now() - interval '2 hours', now());", u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'unpaid', now() - interval '4 hours', now() - interval '3 hours');", u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'unpaid', now() - interval '8 hours', now() - interval '6 hours');", u.ID)
			Expect(err).To(BeNil())

			state, err := u.HasFailedPayments(db)
			Expect(err).To(BeNil())
			Expect(state).To(BeTrue())
		})
	})

	Describe("CanLogin()", func() {
		It("should be true if the account is a service account", func() {
			u := &user.User{
				Username: "qux",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE users SET service_account = true WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			// Make sure confirmation period has lapsed.
			_, err = db.Exec(`UPDATE users SET created_at = now() - interval '26 hours' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(u2.CanLogin()).To(BeTrue())
		})

		It("should be true if the account is confirmed", func() {
			u := &user.User{
				Username:    "qux",
				Password:    "123456",
				Email:       "qux@example.com",
				ConfirmedAt: pq.NullTime{Time: time.Now(), Valid: true},
			}

			Expect(u.CanLogin()).To(BeTrue())
		})

		It("should be true if the account has been recently created", func() {
			u := &user.User{
				Username: "qux",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(u2.CanLogin()).To(BeTrue())
		})

		It("should be false if the account has not been recently created", func() {
			u := &user.User{
				Username: "qux",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET created_at = now() - interval '26 hours' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(u2.CanLogin()).To(BeFalse())
		})
	})

	Describe("CheckCanUseDesktopApp()", func() {
		var (
			u *user.User
		)
		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		for _, state := range user.AllowedDesktopAppBillingStates {
			It(fmt.Sprintf("should be true for %s state", state), func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, state, u.ID)
				Expect(err).To(BeNil())

				can, err := u.CheckCanUseDesktopApp(db)
				Expect(err).To(BeNil())
				Expect(can).To(BeTrue())
			})
		}

		for _, state := range []string{billingprofile.StateFree, billingprofile.StateNoCard, billingprofile.StateDelinquent} {
			It(fmt.Sprintf("should be false for %s state", state), func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, state, u.ID)
				Expect(err).To(BeNil())

				can, err := u.CheckCanUseDesktopApp(db)
				Expect(err).To(BeNil())
				Expect(can).To(BeFalse())
			})
		}

		It("should be true if the column on the user overrides", func() {
			u.CanUseDesktopApp = sql.NullBool{true, true}
			_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())

			can, err := u.CheckCanUseDesktopApp(db)
			Expect(err).To(BeNil())
			Expect(can).To(BeTrue())
		})

		It("should be false if the column on the user overrides but is false", func() {
			u.CanUseDesktopApp = sql.NullBool{false, true}
			_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateFree, u.ID)
			Expect(err).To(BeNil())

			can, err := u.CheckCanUseDesktopApp(db)
			Expect(err).To(BeNil())
			Expect(can).To(BeFalse())
		})

		Context("user is a member of orgs", func() {
			var (
				o, o2 *org.Org
			)

			BeforeEach(func() {
				o = &org.Org{
					Slug:  "Foobar",
					Name:  "Foo Bar, Inc.",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(tx *sqlx.Tx) error {
					return billingprofile.Create(tx, u.ID, sql.NullInt64{o.ID, true})
				})
				Expect(err).To(BeNil())

				o2 = &org.Org{
					Name:  "Macrosoft Corp.",
					Slug:  "macrosoft",
					Email: "bill@macrosoft.com",
				}
				err = o2.Insert(db)
				Expect(err).To(BeNil())
				err = db.WithTransaction(func(tx *sqlx.Tx) error {
					return billingprofile.Create(tx, u.ID, sql.NullInt64{o2.ID, true})
				})
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "owner")
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o2.ID, u.ID, "member")
				Expect(err).To(BeNil())
			})
			It("should be false if none of the orgs are paid", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id IS NULL AND user_id = $2;`, billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())

				can, err := u.CheckCanUseDesktopApp(db)
				Expect(err).To(BeNil())
				Expect(can).To(BeFalse())
			})

			It("should be true if one of the orgs is paid", func() {
				_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id IS NULL AND user_id = $2;`, billingprofile.StateFree, u.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id = $2;`, billingprofile.StatePaid, o2.ID)
				Expect(err).To(BeNil())

				can, err := u.CheckCanUseDesktopApp(db)
				Expect(err).To(BeNil())
				Expect(can).To(BeTrue())
			})
		})
	})

	Describe("ConfirmationTokenExpired()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("should be true when the confirmation_sent_at is null", func() {
			fU, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fU.ConfirmationTokenExpired()).To(BeTrue())
		})

		It("should be false when the confirmation_sent_at is recent", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now() WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fU.ConfirmationTokenExpired()).To(BeFalse())
		})

		It("should be true when the confirmation_sent_at is later than allowed", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now() - interval '73 hours' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fU.ConfirmationTokenExpired()).To(BeTrue())
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
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET max_hosts = 2 WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("should find the user", func() {
			err := u.SetResetPasswordToken(db)
			Expect(err).To(BeNil())

			Expect(u.ResetPasswordToken.String).NotTo(BeEmpty())
			Expect(u.ResetPasswordSentAt).NotTo(BeNil())

			u1, err := user.FindByResetPasswordToken(db, u.ResetPasswordToken.String)
			Expect(err).To(BeNil())
			Expect(u1.ID).To(Equal(u.ID))
		})

		It("should return an error when the token has expired", func() {
			err := u.SetResetPasswordToken(db)
			Expect(err).To(BeNil())

			Expect(u.ResetPasswordToken.String).NotTo(BeEmpty())
			Expect(u.ResetPasswordSentAt).NotTo(BeNil())

			_, err = db.Exec("UPDATE users SET reset_password_sent_at = now() - interval '23 hours' WHERE id = $1", u.ID)
			Expect(err).To(BeNil())

			u1, err := user.FindByResetPasswordToken(db, u.ResetPasswordToken.String)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(u1).To(BeNil())
		})

		It("should return an error when the user has been deleted", func() {
			err := u.SetResetPasswordToken(db)
			Expect(err).To(BeNil())

			Expect(u.ResetPasswordToken.String).NotTo(BeEmpty())
			Expect(u.ResetPasswordSentAt).NotTo(BeNil())

			_, err = db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			u1, err := user.FindByResetPasswordToken(db, u.ResetPasswordToken.String)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(u1).To(BeNil())
		})

		It("should return an error when the token is empty", func() {
			_, err := user.FindByResetPasswordToken(db, "")
			Expect(err).To(Equal(user.ErrResetPasswordTokenRequired))
		})
	})

	Describe("ClearResetPasswordToken()", func() {
		It("sets reset_password_token to null", func() {
			u := &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			err = u.SetResetPasswordToken(db)
			Expect(err).To(BeNil())

			Expect(u.ResetPasswordToken.String).NotTo(BeEmpty())

			err = u.ClearResetPasswordToken(db)
			Expect(err).To(BeNil())
			Expect(u.ResetPasswordToken.String).To(BeEmpty())
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
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("returns the matching user if the token is valid", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now(), confirmation_token = 'ABCD' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByConfirmationToken(db, "ABCD")
			Expect(err).To(BeNil())
			Expect(fU.ID).To(Equal(u.ID))
		})

		It("returns no user if the token is invalid", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = now(), confirmation_token = 'ABCD' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByConfirmationToken(db, "ABCDE")
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(fU).To(BeNil())
		})

		It("returns no user if the user has been deleted", func() {
			_, err := db.Exec(`UPDATE users SET deleted_at = now(), confirmation_sent_at = now(), confirmation_token = 'ABCD' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			fU, err := user.FindByConfirmationToken(db, "ABCD")
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(fU).To(BeNil())
		})
	})

	Describe("FindAllWithFreeHost()", func() {
		It("returns users with unterminated free hosts", func() {
			// User with free, running host - will be returned.
			hs1, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{Free: true, State: host.StateRunning},
			})
			Expect(err).To(BeNil())

			// User with free, stopped host - will be returned.
			hs2, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{Free: true, State: host.StateStopped},
			})
			Expect(err).To(BeNil())

			// User with free, terminated host - will not be returned.
			hs3, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())
			err = hs3.H.Delete(db)
			Expect(err).To(BeNil())

			// User with paid, terminated host - will not be returned.
			hs4, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{Free: false},
			})
			Expect(err).To(BeNil())
			err = hs4.H.Delete(db)
			Expect(err).To(BeNil())

			// User with paid, running host - will not be returned.
			_, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{Free: false, State: host.StateRunning},
			})
			Expect(err).To(BeNil())

			users, err := user.FindAllWithFreeHost(db)
			Expect(err).To(BeNil())

			userIDs := []int64{}
			for _, u := range users {
				userIDs = append(userIDs, u.ID)
			}

			Expect(userIDs).To(ConsistOf(hs1.U.ID, hs2.U.ID))
		})
	})

	Describe("FindAllByNoticeSent()", func() {
		var (
			u1, u2, u3, u4, u5, u6 *user.User
		)

		BeforeEach(func() {
			u1 = &user.User{
				Username: "qux1",
				Password: "123456",
				Email:    "qux1@example.com",
			}
			Expect(test.WrapInTransaction(db, u1.Insert)).To(BeNil())

			_, err := test.CreateHostStack(db, &test.HostStack{
				U: u1,
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET usage_notices_sent = $1 WHERE id = $2;`,
				user.USAGE_NOTICES_NO_WARNING, u1.ID)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "qux2",
				Password: "123456",
				Email:    "qux2@example.com",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u2,
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET usage_notices_sent = $1 WHERE id = $2;`,
				user.USAGE_NOTICES_NO_WARNING, u2.ID)
			Expect(err).To(BeNil())

			u3 = &user.User{
				Username: "qux3",
				Password: "123456",
				Email:    "qux3@example.com",
			}
			Expect(test.WrapInTransaction(db, u3.Insert)).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u3,
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET usage_notices_sent = $1 WHERE id = $2;`,
				user.USAGE_NOTICES_NO_WARNING, u3.ID)
			Expect(err).To(BeNil())

			u4 = &user.User{
				Username: "qux4",
				Password: "123456",
				Email:    "qux4@example.com",
			}
			Expect(test.WrapInTransaction(db, u4.Insert)).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u4,
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET usage_notices_sent = $1 WHERE id = $2;`,
				user.USAGE_NOTICES_FIRST_WARNING, u4.ID)
			Expect(err).To(BeNil())

			u5 = &user.User{
				Username: "qux5",
				Password: "123456",
				Email:    "qux5@example.com",
			}
			Expect(test.WrapInTransaction(db, u5.Insert)).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u5,
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET usage_notices_sent = $1 WHERE id = $2;`,
				user.USAGE_NOTICES_FIRST_WARNING, u5.ID)
			Expect(err).To(BeNil())

			// User with terminated host - should not be returned.
			u6 = &user.User{
				Username: "qux6",
				Password: "123456",
				Email:    "qux6@example.com",
			}
			Expect(test.WrapInTransaction(db, u6.Insert)).To(BeNil())

			hs6, err := test.CreateHostStack(db, &test.HostStack{
				U: u6,
				H: &host.Host{Free: true},
			})
			Expect(err).To(BeNil())

			err = hs6.H.Delete(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET usage_notices_sent = $1 WHERE id = $2;`,
				user.USAGE_NOTICES_FIRST_WARNING, u6.ID)
			Expect(err).To(BeNil())
		})

		It("returns users that has never receive notice email", func() {
			users, err := user.FindAllByNoticeSent(db, user.USAGE_NOTICES_NO_WARNING)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(3))
			Expect(users[0].Email).To(Equal("qux1@example.com"))
			Expect(users[1].Email).To(Equal("qux2@example.com"))
			Expect(users[2].Email).To(Equal("qux3@example.com"))
		})

		It("returns users that has received one notice email", func() {
			users, err := user.FindAllByNoticeSent(db, user.USAGE_NOTICES_FIRST_WARNING)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(2))
			Expect(users[0].Email).To(Equal("qux4@example.com"))
			Expect(users[1].Email).To(Equal("qux5@example.com"))
		})

		It("returns no user when user has received more than two notice emails", func() {
			users, err := user.FindAllByNoticeSent(db, user.USAGE_NOTICES_SECOND_WARNING)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(0))
		})
	})

	Describe("ConfirmEmail", func() {
		var (
			u          *user.User
			f          *test.FakeMailChimp
			fakeStripe *test.FakeStripe
		)
		curTime := time.Now()

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			f = test.NewFakeMailChimp()
			user.MailChimp = f

			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)

			envvars.MailChimpIDNewsletter = "newsletters"
			envvars.MailChimpIDAnnouncements = "announcements"
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
			Expect(u.ConfirmedAt.Time).To(BeTemporally(">=", now))
			Expect(u.ConfirmationSentAt.Valid).To(BeFalse())
			Expect(u.Email).To(Equal("qux@example.com"))
		})

		It("should update user email on Stripe", func() {
			_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'buddy@hotmail.com', confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE billing_profiles SET stripe_id = 'fakestripeid'
				WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			Eventually(func() int {
				return len(fakeStripe.ReceivedRequests)
			}).Should(Equal(1))

			Expect(fakeStripe.ReceivedRequests[0]).To(Equal(map[string]string{
				"method": "POST",
				"path":   "/customers/fakestripeid",
				"card":   "",
				"email":  "buddy@hotmail.com",
				"desc":   "",
			}))
		})

		It("should subscribe to the MailChimp newsletters", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			Eventually(func() int {
				return f.SubscribeCalled
			}).Should(Equal(2))

			Expect(f.UnsubscribeCalled).To(BeZero())
			Expect(f.SubscribeListIDs).To(Equal([]string{"newsletters", "announcements"}))
			opt := &mailchimp.SubscribeOptions{
				Email: "qux@example.com",
				MergeFields: map[string]interface{}{
					"USERNAME": "qux",
					"FNAME":    "quxboy",
				},
			}
			Expect(f.SubscribeOpts).To(Equal([]*mailchimp.SubscribeOptions{opt, opt}))
		})

		It("should subscribe to MailChimp even if one fails", func() {
			f.SubscribeErrs = []error{errors.New("Sorry!")}
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			Eventually(func() int {
				return f.SubscribeCalled
			}).Should(Equal(2))
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
			Expect(u.ConfirmationSentAt.Valid).To(BeTrue())
		})

		It("should move the user's unconfirmed email into email", func() {
			_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'buddy@hotmail.com', confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u.Email).To(Equal("buddy@hotmail.com"))
			Expect(u.UnconfirmedEmail.String).To(BeEmpty())
		})

		It("should unsubscribe the user when the email changes", func() {
			_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'buddy@hotmail.com', confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			Eventually(func() int {
				return f.UnsubscribeCalled
			}).Should(Equal(2))

			Expect(f.SubscribeCalled).To(Equal(2))
			Expect(f.UnsubscribeListIDs).To(Equal([]string{"newsletters", "announcements"}))
			Expect(f.UnsubscribeEmails).To(Equal([]string{"qux@example.com", "qux@example.com"}))
		})

		It("should subscribe the user to their new email when the email changes", func() {
			_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'buddy@hotmail.com', confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			Eventually(func() int {
				return f.UnsubscribeCalled
			}).Should(Equal(2))

			Expect(f.SubscribeCalled).To(Equal(2))
			Expect(f.SubscribeListIDs).To(Equal([]string{"newsletters", "announcements"}))
			opt := &mailchimp.SubscribeOptions{
				Email: "buddy@hotmail.com",
				MergeFields: map[string]interface{}{
					"USERNAME": "qux",
					"FNAME":    "quxboy",
				},
			}
			Expect(f.SubscribeOpts).To(Equal([]*mailchimp.SubscribeOptions{opt, opt}))
		})

		It("should update all outstanding container collabs to user IDs", func() {
			owner := &user.User{
				Username: "owner",
				Email:    "owner@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, owner.Insert)
			Expect(err).To(BeNil())

			hs, err := test.CreateHostStack(db, &test.HostStack{
				U: owner,
			})
			Expect(err).To(BeNil())

			cnr := &container.Container{
				CID:    "cafebabe",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
				Name:   sql.NullString{"Java", true},
			}
			err = cnr.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET confirmation_sent_at = $1 WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
			Expect(err).To(BeNil())

			_, err = containercollab.Create(db, cnr.ID, u.Email)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			err = u.ConfirmEmail(db)
			Expect(err).To(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
		})
	})

	Describe("SendUsageNoticeEmail", func() {
		var (
			hs              *test.HostStack
			u               *user.User
			fakeMailer      *test.FakeMailer
			origMailer      mailer.Mailer
			quotaRestoredAt time.Time
		)

		BeforeEach(func() {
			origMailer = user.Mailer
			fakeMailer = &test.FakeMailer{}
			user.Mailer = fakeMailer

			u = &user.User{
				Username:   "qux",
				Name:       "quxboy",
				Password:   "123456",
				Email:      "qux@example.com",
				UsageQuota: sql.NullInt64{180000, true},
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			hs, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE hosts SET free = true WHERE id = $1`, hs.H.ID)
			Expect(err).To(BeNil())

			quotaRestoredAt = time.Date(2014, 1, 1, 0, 0, 0, 0, time.Local)
			_, err = db.Exec(`UPDATE users SET usage_quota_restored_at = $1 ;`, quotaRestoredAt)
			Expect(err).To(BeNil())

			envvars.WebHostname = "nitrous.example"
		})

		AfterEach(func() {
			user.Mailer = origMailer
		})

		It("sends usage notice email", func() {
			// add usage of 44.8 hours and so there'll be 5.2 hours remaining
			// we will show 5 hours remaining in the email
			startTime := quotaRestoredAt.Add(2 * time.Hour)
			endTime := quotaRestoredAt.Add(168480 * time.Second) //46.8 hours
			_, err := db.Exec(`
				INSERT INTO host_transitions (
					host_id, previous_state, state, created_at, ended_at
				) values ($1, $2, $3, $4, $5);
			`, hs.H.ID, "starting", "running", startTime, endTime)
			Expect(err).To(BeNil())

			err = u.SendUsageNoticeEmail(db)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"quxboy\" <qux@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Subject).To(Equal("Your free Nitrous workstation: 5 hours remaining this month"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello quxboy!"))
			Expect(fakeMailer.HTML).To(ContainSubstring("<h3>Hello quxboy!</h3>"))
			Expect(fakeMailer.HTML).To(ContainSubstring("<p>You've used 45 out of the free 50 hours of usage"))
		})
	})

	Describe("SendConfirmationEmail", func() {
		var (
			u          *user.User
			fakeMailer *test.FakeMailer
			origMailer mailer.Mailer
		)

		BeforeEach(func() {
			origMailer = user.Mailer
			fakeMailer = &test.FakeMailer{}
			user.Mailer = fakeMailer

			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			user.Mailer = origMailer
		})

		It("should generate a confirmation token", func() {
			err := u.SendConfirmationEmail(db, false)
			Expect(err).To(BeNil())

			u, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			expectedLength := 20 + len(strconv.FormatInt(u.ID, 10))
			Expect(u.ConfirmationToken.String).ToNot(BeEmpty())
			Expect(u.ConfirmationToken.String).To(HaveLen(expectedLength))
			Expect(u.ConfirmationToken.String).To(HaveSuffix(strconv.FormatInt(u.ID, 10)))
		})

		It("should send the email", func() {
			envvars.WebHostname = "nitrous.example"
			err := u.SendConfirmationEmail(db, false)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"quxboy\" <qux@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Subject).To(Equal("Confirm your Nitrous email"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello quxboy!"))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app/#/confirm-email?token=%s", u.ConfirmationToken.String)))
			Expect(fakeMailer.HTML).To(ContainSubstring("<h3>Hello quxboy!</h3>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("<a href=\"https://nitrous.example/app/#/confirm-email?token=%s\" target=\"_blank\"", u.ConfirmationToken.String)))
		})

		It("should send the new user email if the user is new ", func() {
			envvars.WebHostname = "nitrous.example"
			err := u.SendConfirmationEmail(db, true)
			Expect(err).To(BeNil())

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"quxboy\" <qux@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Subject).To(Equal("Confirm your Nitrous email"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello quxboy!"))
			Expect(fakeMailer.Body).To(ContainSubstring("Thanks for signing up for Nitrous!"))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app/#/confirm-email?token=%s", u.ConfirmationToken.String)))
			Expect(fakeMailer.HTML).To(ContainSubstring("Thanks for signing up for Nitrous!"))
			Expect(fakeMailer.HTML).To(ContainSubstring("<h3>Hello quxboy!</h3>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("<a href=\"https://nitrous.example/app/#/confirm-email?token=%s\" target=\"_blank\"", u.ConfirmationToken.String)))
		})

		It("should send the email to UnconfirmedEmail if available", func() {
			_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'buddy@nitrous.io' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = u.SendConfirmationEmail(db, false)
			Expect(err).To(BeNil())

			Expect(fakeMailer.Tos).To(Equal([]string{"\"quxboy\" <buddy@nitrous.io>"}))
		})
	})

	Describe("ActivateBillingProfile", func() {
		var (
			u          *user.User
			fakeStripe *test.FakeStripe
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)
		})

		It("sends a request to Stripe to create a customer", func() {
			u.ActivateBillingProfile(db, "token")

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(fakeStripe.ReceivedRequests[0]).To(Equal(map[string]string{
				"method":             "POST",
				"path":               "/customers",
				"card":               "token",
				"email":              u.Email,
				"desc":               "Name: quxboy, Username: qux",
				"metadata[username]": "qux",
				"metadata[name]":     "quxboy",
			}))
		})

		It("should send a request if a token is not provided", func() {
			u.ActivateBillingProfile(db, "")

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(fakeStripe.ReceivedRequests[0]).To(Equal(map[string]string{
				"method":             "POST",
				"path":               "/customers",
				"card":               "",
				"email":              u.Email,
				"desc":               "Name: quxboy, Username: qux",
				"metadata[username]": "qux",
				"metadata[name]":     "quxboy",
			}))
		})

		Context("when Stripe returns the created customer", func() {
			It("updates and persists the record", func() {
				fakeStripe.Responses = []string{test.StripeCustomerResponse}

				b, err := u.ActivateBillingProfile(db, "token")
				Expect(err).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

				Expect(b.ID).NotTo(BeNil())
				Expect(b.UserID).To(Equal(u.ID))
				Expect(b.StripeID.String).To(Equal("sc_01"))
				Expect(b.LastFour.String).To(Equal("4242"))
				Expect(b.CardType.String).To(Equal("Visa"))
				Expect(b.ExpMonth.Int64).To(Equal(int64(10)))
				Expect(b.ExpYear.Int64).To(Equal(int64(2018)))
				Expect(b.PendingActivation).To(BeFalse())
			})

			It("should create a record if no card is provided", func() {
				resp := `{
					"id": "sc_01",
					"email": "user@example.com",
					"cards": null,
					"default_card": "card_14wQPX2CU2PklWtHi34KFonT"
				}`
				fakeStripe.Responses = []string{resp}

				b, err := u.ActivateBillingProfile(db, "token")
				Expect(err).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

				Expect(b.ID).NotTo(BeNil())
				Expect(b.UserID).To(Equal(u.ID))
				Expect(b.StripeID.String).To(Equal("sc_01"))
				Expect(b.LastFour.String).To(BeEmpty())
				Expect(b.CardType.String).To(BeEmpty())
				Expect(b.ExpMonth.Int64).To(BeZero())
				Expect(b.ExpYear.Int64).To(BeZero())
				Expect(b.State).To(Equal("no_card"))
				Expect(b.PendingActivation).To(BeFalse())

			})

			It("should not allow a banned card", func() {
				b := &user.User{
					Username: "banner",
					Password: "123456",
					Email:    "banner@example.com",
				}
				err := test.WrapInTransaction(db, b.Insert)
				Expect(err).To(BeNil())

				u2 := &user.User{
					Username: "bad",
					Name:     "quxboy",
					Password: "123456",
					Email:    "bad@example.com",
				}
				err = test.WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET deleted_at = now(), banned_at = now(), banned_by_id = $1 WHERE id = $2;`, b.ID, u2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE billing_profiles SET stripe_fingerprint = 'H0SJieNrXmTdq3qs' WHERE user_id = $1;`, u2.ID)
				Expect(err).To(BeNil())

				fakeStripe.Responses = []string{test.StripeCustomerResponse}

				_, err = u.ActivateBillingProfile(db, "token")
				Expect(err).To(Equal(user.ErrCardBanned))

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
				delReq := fakeStripe.ReceivedRequests[1]
				Expect(delReq["method"]).To(Equal("DELETE"))
				Expect(delReq["path"]).To(Equal("/customers/sc_01"))
			})
		})

		Context("when Stripe returns a card declined error", func() {
			It("returns the error, without updating the record", func() {
				fakeStripe.Errors = []*stripe.Error{test.StripeCardDeclined}

				_, err := u.ActivateBillingProfile(db, "token")
				Expect(err).To(Equal(billingprofile.ErrCardDeclined))

				b, err := billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
				Expect(b.State).To(Equal("no_card"))
			})
		})

		Context("when Stripe returns an error", func() {
			It("returns the error, without updating the record", func() {
				fakeStripe.Errors = []*stripe.Error{
					&stripe.Error{
						Type: stripe.ErrorType(stripe.APIErr),
						Msg:  "some error occurred",
						Code: stripe.ErrorCode(stripe.ProcessingErr),
					},
				}

				_, err := u.ActivateBillingProfile(db, "token")
				Expect(err).NotTo(BeNil())

				b, err := billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
				Expect(b.State).To(Equal("no_card"))
			})
		})
	})
})

var _ = Describe("models/user no db", func() {
	var u *user.User

	BeforeEach(func() {
		u = &user.User{}
	})

	Context("CheckSuspicious()", func() {
		It("finds suspicious on e-mail", func() {
			u.Email = "hackerviros@gmail.com"
			u.CheckSuspicious()
			Expect(u.Suspicious.Valid).To(BeTrue())
			Expect(u.Suspicious.String).To(Equal("Signature match email: viros"))
		})

		It("appends to existing suspicious field", func() {
			u.Suspicious = sql.NullString{"bad name", true}
			u.Username = "lucasaoqui"
			u.CheckSuspicious()
			Expect(u.Suspicious.Valid).To(BeTrue())
			Expect(u.Suspicious.String).To(Equal("bad name; Signature match username: aoqui"))
		})

		It("is no-op if user is not suspicious", func() {
			Expect(u.Suspicious.Valid).To(BeFalse())
			u.CheckSuspicious()
			Expect(u.Suspicious.Valid).To(BeFalse())
		})
	})
})
