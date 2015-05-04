package engagement_test

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/engagement"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "engagement")
}

var _ = Describe("engagement", func() {
	var (
		db         *db.Database
		u          *user.User
		fakeMailer *FakeMailer
		origMailer mailer.Mailer
	)

	createUser := func(username string) *user.User {
		user := &user.User{
			Username: username,
			Password: "123456",
			Email:    fmt.Sprintf("%s@example.com", username),
		}
		err := WrapInTransaction(db, user.Insert)
		Expect(err).To(BeNil())
		Expect(user.ID).NotTo(BeZero())
		return user
	}

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = createUser("foo")

		origMailer = user.Mailer
		fakeMailer = &FakeMailer{}
		engagement.Mailer = fakeMailer

		engagement.ActivationEmailInterval = 30 * time.Minute
		engagement.GetStartedEmailIntervalFirst = 1 * time.Hour
		engagement.GetStartedEmailIntervalSecond = 4 * time.Hour
		engagement.GetStartedEmailIntervalThird = 6 * time.Hour
	})

	AfterEach(func() {
		engagement.Mailer = origMailer
		if db != nil {
			db.Close()
		}
	})

	Describe("ActivationEmailCandidates", func() {
		var u3 *user.User

		BeforeEach(func() {
			u2 := createUser("foo2")
			u3 = createUser("foo3")
			u4 := createUser("foo4")
			u5 := createUser("foo5")

			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '30 minutes' WHERE id = $1 OR id = $2 OR id = $3;`, u.ID, u3.ID, u4.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE users SET created_at = now() - interval '30 minutes', confirmation_token = 'ABC' WHERE id = $1;`, u2.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2 OR user_id = $3 OR user_id = $4 OR user_Id = $5`, billingprofile.StateNoCard, u.ID, u2.ID, u3.ID, u5.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2`, billingprofile.StatePaid, u4.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET activation_email_sent_at = NULL WHERE user_id = $1 OR user_id = $2 OR user_id = $3 OR user_Id = $4`, u2.ID, u3.ID, u4.ID, u5.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE user_profiles SET activation_email_sent_at = now() WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())
		})

		It("should include only 12hr+ old confirmed users with a no card billing profile", func() {
			users, err := engagement.ActivationEmailCandidates(db)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(1))
			Expect(users[0].ID).To(Equal(u3.ID))
		})

		It("should not include deleted users", func() {
			_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1`, u3.ID)
			Expect(err).To(BeNil())

			users, err := engagement.ActivationEmailCandidates(db)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(0))
		})
	})

	Describe("SendActivationEmail", func() {
		It("should send the email", func() {
			envvars.WebHostname = "nitrous.example"
			err := engagement.SendActivationEmail(db, *u)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"foo\" <foo@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Subject).To(Equal("Activate your Nitrous account"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.Body).To(ContainSubstring("Thanks for confirming your email address."))
			Expect(fakeMailer.Body).To(ContainSubstring("https://nitrous.example/app#billing"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Thanks for confirming your email address."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app#billing" target="_blank">Activate account</a>`))
		})

		It("should mark the user profile as activation email sent", func() {
			envvars.WebHostname = "nitrous.example"
			err := engagement.SendActivationEmail(db, *u)
			Expect(err).To(BeNil())

			up, err := userprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(up).ToNot(BeNil())
			Expect(up.ActivationEmailSentAt).ToNot(BeNil())
		})

		It("should fail if the email has already been sent", func() {
			_, err := db.Exec(`UPDATE user_profiles SET activation_email_sent_at = now() WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			envvars.WebHostname = "nitrous.example"
			err = engagement.SendActivationEmail(db, *u)
			Expect(err).To(Equal(engagement.ErrActivationEmailAlreadySent))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should fail if the user has been deleted", func() {
			_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			envvars.WebHostname = "nitrous.example"
			err = engagement.SendActivationEmail(db, *u)
			Expect(err).ToNot(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should rollback the changes if the mailer fails", func() {
			mErr := errors.New("Internet unavailable")
			fakeMailer.Error = mErr
			defer func() { fakeMailer.Error = nil }()

			err := engagement.SendActivationEmail(db, *u)
			Expect(err).To(Equal(mErr))

			up, err := userprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(up).ToNot(BeNil())
			Expect(up.ActivationEmailSentAt).To(BeNil())
		})
	})

	Describe("GetStartedEmailCandidates", func() {
		var u2, u6 *user.User

		BeforeEach(func() {
			u2 = createUser("foo2")
			u3 := createUser("foo3")
			u4 := createUser("foo4")
			u5 := createUser("foo5")
			u6 = createUser("foo6")
			u7 := createUser("foo7")

			// u5 is not a candidate
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1 OR id = $2 OR id = $3 OR id = $4 OR id = $5 OR id = $6;`, u.ID, u2.ID, u3.ID, u4.ID, u6.ID, u7.ID)
			Expect(err).To(BeNil())

			// u4 is not a candidate
			_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2 OR user_id = $3 OR user_id = $4 OR user_id = $5 OR user_id = $6`, billingprofile.StateNoCard, u.ID, u2.ID, u3.ID, u5.ID, u6.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2`, billingprofile.StatePaid, u4.ID)
			Expect(err).To(BeNil())

			// only u2 & u6 are candidates
			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_last_sent_at = now(), starter_credit_email_send_count = 1 WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_last_sent_at = now() - interval '4 hours', starter_credit_email_send_count = 1 WHERE user_id = $1`, u2.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_last_sent_at = now() - interval '5 hours', starter_credit_email_send_count = 2 WHERE user_id = $1`, u3.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_last_sent_at = now() - interval '6 hours', starter_credit_email_send_count = 2 WHERE user_id = $1`, u6.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE user_profiles SET activation_email_sent_at = now() - interval '20 minutes' WHERE user_id = $1`, u7.ID)
			Expect(err).To(BeNil())
		})

		It("should only include users without billing profiles and users who haven't received email in the 3 thresholds", func() {
			users, err := engagement.GetStartedEmailCandidates(db)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(2))
			Expect([]string{users[0].Username, users[1].Username}).To(ConsistOf(u2.Username, u6.Username))
		})

		It("should not include deleted users", func() {
			_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1`, u2.ID)
			Expect(err).To(BeNil())

			users, err := engagement.GetStartedEmailCandidates(db)
			Expect(err).To(BeNil())
			Expect(users).To(HaveLen(1))
			Expect(users[0].Username).To(Equal(u6.Username))
		})
	})

	Describe("SendGetStartedEmail", func() {
		verifySendCount := func(count int, datePresent bool) {
			var up struct {
				SendCount int        `db:"starter_credit_email_send_count"`
				SentAt    *time.Time `db:"starter_credit_email_last_sent_at"`
			}
			err := db.Get(&up, "SELECT starter_credit_email_send_count, starter_credit_email_last_sent_at FROM user_profiles WHERE user_id = $1", u.ID)
			Expect(err).To(BeNil())
			Expect(up.SendCount).To(Equal(count))
			if datePresent {
				Expect(up.SentAt).ToNot(BeNil())
			} else {
				Expect(up.SentAt).To(BeNil())
			}
		}

		It("should return an error if the account is less than 1hr old", func() {
			err := engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(Equal(engagement.ErrGetStartedEmailNotApplicable))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should return an error if the account is less than 4 hrs old but not at a send_count of 1", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_send_count = 1, starter_credit_email_last_sent_at = now() - interval '3 hours' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(Equal(engagement.ErrGetStartedEmailNotApplicable))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should return an error if the account is less than 6 hrs old but not at a send_count of 2", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_send_count = 2, starter_credit_email_last_sent_at = now() - interval '5 hours' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(Equal(engagement.ErrGetStartedEmailNotApplicable))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should not send an email if the user has been deleted", func() {
			_, err := db.Exec(`UPDATE users SET deleted_at = now(), created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(Equal(engagement.ErrGetStartedEmailNotApplicable))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should send an email including a confirmation token link if needed", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			envvars.WebHostname = "nitrous.example"
			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(BeNil())

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"foo\" <foo@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Subject).To(Equal("Nitrous: $5 Credit to Get Started"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.Body).To(ContainSubstring("To help you get started developing your first"))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app/#/confirm-email?token=%s", u2.ConfirmationToken.String)))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.HTML).To(ContainSubstring("To help you get started developing your first"))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf(`<a href="https://nitrous.example/app/#/confirm-email?token=%s" target="_blank">Claim Credit</a>`, u2.ConfirmationToken.String)))

			verifySendCount(1, true)
		})

		It("should regenerate the confirmation token if it has expired", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			envvars.WebHostname = "nitrous.example"
			u.ConfirmationToken = sql.NullString{"ABCDEF", true}
			t := time.Now().Add(-73 * time.Hour)
			u.ConfirmationSentAt = pq.NullTime{t, true}
			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(BeNil())

			u2, err := user.FindByID(db, u.ID)
			Expect(u2.ConfirmationToken.Valid).To(BeTrue())
			Expect(u2.ConfirmationToken).ToNot(Equal("ABCDEF"))
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"foo\" <foo@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Subject).To(Equal("Nitrous: $5 Credit to Get Started"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.Body).To(ContainSubstring("To help you get started developing your first"))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app/#/confirm-email?token=%s", u2.ConfirmationToken.String)))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.HTML).To(ContainSubstring("To help you get started developing your first"))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf(`<a href="https://nitrous.example/app/#/confirm-email?token=%s" target="_blank">Claim Credit</a>`, u2.ConfirmationToken.String)))
		})

		It("should send an email without a confirmation token if already confirmed", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			envvars.WebHostname = "nitrous.example"
			t := time.Now()
			u.ConfirmedAt = pq.NullTime{t, true}
			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Tos).To(Equal([]string{"\"foo\" <foo@example.com>"}))
			Expect(fakeMailer.ReplyTo).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
			Expect(fakeMailer.Subject).To(Equal("Nitrous: $5 Credit to Get Started"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.Body).To(ContainSubstring("To help you get started developing your first"))
			Expect(fakeMailer.Body).To(ContainSubstring("https://nitrous.example/app"))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hello foo!"))
			Expect(fakeMailer.HTML).To(ContainSubstring("To help you get started developing your first"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app" target="_blank">Claim Credit</a>`))
		})

		It("should send its next email 2 days after", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_send_count = 1, starter_credit_email_last_sent_at = now() - interval '4 hours' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			verifySendCount(2, true)
		})

		It("should send the following email 5 days after", func() {
			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET starter_credit_email_send_count = 2, starter_credit_email_last_sent_at = now() - interval '6 hours' WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			verifySendCount(3, true)
		})

		It("should rollback the changes if the mailer fails", func() {
			mErr := errors.New("Internet unavailable")
			fakeMailer.Error = mErr
			defer func() { fakeMailer.Error = nil }()

			_, err := db.Exec(`UPDATE users SET created_at = now() - interval '1 hour' WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = engagement.SendGetStartedEmail(db, *u)
			Expect(err).To(Equal(mErr))

			verifySendCount(0, false)
		})
	})
})
