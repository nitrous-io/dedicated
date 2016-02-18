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

	Describe("IdeSettingsMap()", func() {
		It("should only unpack user set values by default", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())

			settings.IdeSettings = `{"abc": true}`
			m, err := settings.IdeSettingsMap()
			Expect(err).To(BeNil())
			Expect(m).To(HaveKey("abc"))
			Expect(m["abc"]).To(BeTrue())
		})
	})

	Describe("IdeSettingsMapWithDefaults()", func() {
		It("should mix the values with the defaults", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())

			settings.IdeSettings = `{"wrap_lines": false, "ruler_column": null}`
			m, err := settings.IdeSettingsMapWithDefaults()
			Expect(err).To(BeNil())

			for k, v := range user.DefaultSettings {
				if k == "wrap_lines" || k == "ruler_column" {
					continue
				}
				Expect(m[k]).To(Equal(v))
			}

			Expect(m["wrap_lines"]).To(BeFalse())
			Expect(m["ruler_column"]).To(Equal(0))
		})
	})

	Describe("Update()", func() {
		It("updates the user settings record", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())

			Expect(settings.IdeSettings).To(Equal("{}"))

			settings.IdeSettings = `{"abc": true}`

			err = settings.Update(db)
			Expect(err).To(BeNil())

			Expect(settings.IdeSettings).To(Equal(`{"abc": true}`))
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
			Expect(settings.IdeSettings).To(Equal("{}"))
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
