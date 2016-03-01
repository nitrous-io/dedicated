package oauthtoken_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthtoken"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/oauthtoken")
}

var _ = Describe("models/oauthtoken", func() {
	var (
		db *db.Database
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "demoman",
			Password: "123456",
			Email:    "demoman@mann.co",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Upsert()", func() {
		It("should insert new record if no existing record", func() {
			t := &oauthtoken.OAuthToken{
				UserID:       u.ID,
				Service:      "tweater",
				NitrousToken: sql.NullString{"nitroustoken", true},
				RequestToken: sql.NullString{"oauthtoken", true},
			}
			Expect(t.Upsert(db)).To(BeNil())
			Expect(t.UserID).To(Equal(u.ID))
			Expect(t.Service).To(Equal("tweater"))
			Expect(t.NitrousToken.String).To(Equal("nitroustoken"))
			Expect(t.RequestToken.String).To(Equal("oauthtoken"))
		})

		It("update existing record", func() {
			t := &oauthtoken.OAuthToken{
				UserID:       u.ID,
				Service:      "tweater",
				NitrousToken: sql.NullString{"nitroustoken", true},
				RequestToken: sql.NullString{"oauthtoken", true},
			}
			Expect(t.Upsert(db)).To(BeNil())

			t1 := &oauthtoken.OAuthToken{
				UserID:       u.ID,
				Service:      "tweater",
				NitrousToken: sql.NullString{"nitroustoken", true},
				RequestToken: sql.NullString{"oauthtoken", true},
			}
			Expect(t1.Upsert(db)).To(BeNil())

			Expect(t1.UserID).To(Equal(u.ID))
			Expect(t1.Service).To(Equal("tweater"))
			Expect(t1.NitrousToken.String).To(Equal("nitroustoken"))
			Expect(t1.RequestToken.String).To(Equal("oauthtoken"))
		})

		Describe("Update()", func() {
			It("updates existing record", func() {
				t := &oauthtoken.OAuthToken{
					UserID:       u.ID,
					Service:      "tweater",
					NitrousToken: sql.NullString{"nitroustoken", true},
					RequestToken: sql.NullString{"oauthtoken", true},
				}
				Expect(t.Upsert(db)).To(BeNil())
				Expect(t.UserID).To(Equal(u.ID))
				Expect(t.Service).To(Equal("tweater"))
				Expect(t.NitrousToken.String).To(Equal("nitroustoken"))
				Expect(t.RequestToken.String).To(Equal("oauthtoken"))

				t.Secret = sql.NullString{"newsecret", true}
				t.AccessToken = sql.NullString{"newaccesstoken", true}
				Expect(t.Update(db)).To(BeNil())

				Expect(t.UserID).To(Equal(u.ID))
				Expect(t.Service).To(Equal("tweater"))
				Expect(t.NitrousToken.String).To(Equal("nitroustoken"))
				Expect(t.RequestToken.String).To(Equal("oauthtoken"))
				Expect(t.AccessToken.String).To(Equal("newaccesstoken"))
				Expect(t.Secret.String).To(Equal("newsecret"))
			})
		})

		Describe("FindByTokenAndService()", func() {
			It("returns oauth token record", func() {
				t := &oauthtoken.OAuthToken{
					UserID:       u.ID,
					Service:      "tweater",
					NitrousToken: sql.NullString{"nitroustoken", true},
					RequestToken: sql.NullString{"oauthtoken", true},
				}
				Expect(t.Upsert(db)).To(BeNil())
				Expect(t.UserID).To(Equal(u.ID))
				Expect(t.Service).To(Equal("tweater"))
				Expect(t.NitrousToken.String).To(Equal("nitroustoken"))
				Expect(t.RequestToken.String).To(Equal("oauthtoken"))

				t1, err := oauthtoken.FindByTokenAndService(db, "oauthtoken", "tweater")
				Expect(err).To(BeNil())

				Expect(t1.UserID).To(Equal(u.ID))
				Expect(t1.Service).To(Equal("tweater"))
				Expect(t1.NitrousToken.String).To(Equal("nitroustoken"))
				Expect(t1.RequestToken.String).To(Equal("oauthtoken"))
			})
		})

	})
})
