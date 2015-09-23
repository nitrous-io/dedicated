package userprofile_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/userprofile")
}

var _ = Describe("models/userprofile", func() {
	var (
		db *db.Database
		u  *user.User
	)

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

	Describe("UserProfile", func() {
		It("created as part of user creation", func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			p := &userprofile.UserProfile{}
			err = db.Get(p, "SELECT * FROM user_profiles WHERE user_id = $1;", u.ID)
			Expect(err).To(BeNil())
			Expect(p.ID).NotTo(BeNil())
			Expect(p.UserID).To(Equal(u.ID))
			Expect(p.MaxHosts).To(Equal(int64(10)))
			Expect(p.Advanced).To(Equal(false))
			Expect(p.CreatedAt).NotTo(BeNil())
			Expect(p.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Find()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		Context("when a valid user id is given", func() {
			It("returns the user profile record", func() {
				p, err := userprofile.Find(db, u.ID)
				Expect(err).To(BeNil())
				Expect(p).NotTo(BeNil())
				Expect(p.UserID).To(Equal(u.ID))
			})
		})

		Context("when a user profile for given user id does not exist", func() {
			It("returns an error", func() {
				p, err := userprofile.Find(db, 9001)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(p).To(BeNil())
			})
		})
	})

	Describe("Update()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates the user profile record", func() {
			p, err := userprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(p).NotTo(BeNil())

			oldUpdatedAt := p.UpdatedAt
			p.Advanced = true

			err = p.Update(db)
			Expect(err).To(BeNil())

			Expect(p.Advanced).To(BeTrue())
			Expect(p.UpdatedAt).NotTo(Equal(oldUpdatedAt))
		})

		It("does not allow to update the max_hosts field", func() {
			p, err := userprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(p).NotTo(BeNil())

			oldMaxHosts := p.MaxHosts
			p.MaxHosts = 9999

			err = p.Update(db)
			Expect(err).To(BeNil())

			Expect(p.MaxHosts).NotTo(Equal(9999))
			Expect(p.MaxHosts).To(Equal(oldMaxHosts))
		})
	})

	Describe("UpdateGithubInfo", func() {
		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates github info", func() {
			p, err := userprofile.Find(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(p).NotTo(BeNil())

			p.GitHubUserName = sql.NullString{"foo-github", true}
			p.GitHubOAuthTokenJson = sql.NullString{`{"access_token": "token" }`, true}
			p.GitHubUserScopesJson = sql.NullString{`["repo","admin:public_key"]`, true}

			err = p.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())

			Expect(p.GitHubUserName.String).To(Equal("foo-github"))
			Expect(p.GitHubOAuthTokenJson.String).To(Equal(`{"access_token": "token" }`))
			Expect(p.GitHubUserScopesJson.String).To(Equal(`["repo","admin:public_key"]`))
		})
	})
})
