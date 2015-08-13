package user_test

import (
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("models/user settings", func() {
	var (
		db *ndb.Database
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Update()", func() {
		It("updates the user settings record", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())

			Expect(settings.AutoCloseBrackets).To(BeTrue())
			Expect(settings.Keybindings).To(Equal("default"))
			Expect(settings.RulerColumn).To(Equal(int64(0)))
			Expect(settings.Theme).To(Equal("solarized dark"))
			Expect(settings.RemoveTrailingWhitespaceOnSave).To(BeTrue())
			origUpdatedAt := settings.UpdatedAt

			settings.AutoCloseBrackets = false
			settings.Keybindings = "vim"
			settings.RulerColumn = 120
			settings.Theme = "vibrant-ink"
			settings.RemoveTrailingWhitespaceOnSave = false

			err = settings.Update(db)
			Expect(err).To(BeNil())

			Expect(settings.AutoCloseBrackets).To(BeFalse())
			Expect(settings.Keybindings).To(Equal("vim"))
			Expect(settings.RulerColumn).To(Equal(int64(120)))
			Expect(settings.Theme).To(Equal("vibrant-ink"))
			Expect(settings.UpdatedAt.Time).To(BeTemporally(">", origUpdatedAt.Time))
			Expect(settings.RemoveTrailingWhitespaceOnSave).To(BeFalse())
		})
	})

	Describe("User.CreateSettings()", func() {
		It("creates an associated user settings record with default values", func() {
			// Creating a user also creates an associated user settings record,
			// so we delete it first.
			_, err := db.Exec(`DELETE FROM user_settings WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			err = u.CreateSettings(db)
			Expect(err).To(BeNil())

			settings := &user.Settings{}
			err = db.Get(settings, `SELECT * FROM user_settings WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())

			Expect(settings.UserID).To(Equal(u.ID))
			Expect(settings.AutoCloseBrackets).To(BeTrue())
			Expect(settings.RulerColumn).To(Equal(int64(0)))
			Expect(settings.RemoveTrailingWhitespaceOnSave).To(BeTrue())
			Expect(settings.CreatedAt).NotTo(BeNil())
			Expect(settings.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("User.Settings()", func() {
		It("returns the user's settings", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())

			Expect(settings.UserID).To(Equal(u.ID))
		})
	})
})
