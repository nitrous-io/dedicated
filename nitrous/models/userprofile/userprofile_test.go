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

	Describe("UpdateGitConfig", func() {
		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates git config", func() {
			p, err := userprofile.Find(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(p).NotTo(BeNil())

			p.GitName = sql.NullString{"food", true}
			p.GitEmail = sql.NullString{"food@bad.com", true}

			err = p.UpdateGitConfig(db)
			Expect(err).NotTo(HaveOccurred())

			Expect(p.GitName.String).To(Equal("food"))
			Expect(p.GitEmail.String).To(Equal("food@bad.com"))
		})
	})

	Describe("FindByGitHubUserName", func() {
		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).NotTo(HaveOccurred())

			p, err := userprofile.Find(db, u.ID)
			Expect(err).NotTo(HaveOccurred())

			p.GitHubUserName = sql.NullString{"foo-github", true}
			err = p.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the user profile record", func() {
			p, err := userprofile.FindByGitHubUserName(db, "foo-github")
			Expect(err).NotTo(HaveOccurred())
			Expect(p).NotTo(BeNil())
			Expect(p.UserID).To(Equal(u.ID))
		})
	})

	Describe("ConnectedGithub", func() {
		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns true if github records exist", func() {
			p, err := userprofile.Find(db, u.ID)
			Expect(err).NotTo(HaveOccurred())

			p.GitHubUserName = sql.NullString{"foo-github", true}
			p.GitHubOAuthTokenJson = sql.NullString{`{"access_token":"746d66f0dfa22fbb73a994a2ec621ef6e52c05ef","token_type":"bearer","expiry":"0001-01-01T00:00:00Z"}`, true}
			p.GitHubUserScopesJson = sql.NullString{`["user:email","repo","admin:public_key"]`, true}
			err = p.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())

			connected, err := userprofile.ConnectedToGithub(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(connected).To(BeTrue())
		})

		It("returns false if none of the github records exist", func() {
			connected, err := userprofile.ConnectedToGithub(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(connected).To(BeFalse())
		})

		It("returns false if one of the required github record does not exist", func() {
			p, err := userprofile.Find(db, u.ID)
			Expect(err).NotTo(HaveOccurred())

			p.GitHubUserName = sql.NullString{"", false}
			p.GitHubOAuthTokenJson = sql.NullString{`{"access_token":"746d66f0dfa22fbb73a994a2ec621ef6e52c05ef","token_type":"bearer","expiry":"0001-01-01T00:00:00Z"}`, true}
			p.GitHubUserScopesJson = sql.NullString{`["user:email","repo","admin:public_key"]`, true}
			err = p.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())

			connected, err := userprofile.ConnectedToGithub(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(connected).To(BeFalse())

			p.GitHubUserName = sql.NullString{"foo-github", true}
			p.GitHubOAuthTokenJson = sql.NullString{"", false}
			p.GitHubUserScopesJson = sql.NullString{`["user:email","repo","admin:public_key"]`, true}
			err = p.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())

			connected, err = userprofile.ConnectedToGithub(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(connected).To(BeFalse())

			p.GitHubUserName = sql.NullString{"foo-github", true}
			p.GitHubOAuthTokenJson = sql.NullString{`{"access_token":"746d66f0dfa22fbb73a994a2ec621ef6e52c05ef","token_type":"bearer","expiry":"0001-01-01T00:00:00Z"}`, true}
			p.GitHubUserScopesJson = sql.NullString{"", false}
			err = p.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())

			connected, err = userprofile.ConnectedToGithub(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(connected).To(BeFalse())
		})
	})

	Describe("CanDeleteGitHubPublicKeys", func() {
		var up *userprofile.UserProfile

		BeforeEach(func() {
			up = &userprofile.UserProfile{}
		})

		It("returns false when github scopes is nil", func() {
			Expect(up.CanDeleteGitHubPublicKeys()).To(Equal(false))
		})

		It("returns false when github scopes is empty", func() {
			up.GitHubUserScopesJson = sql.NullString{"", true}
			Expect(up.CanDeleteGitHubPublicKeys()).To(Equal(false))
		})

		It("returns false when github scopes does not contain `admin:public_key`", func() {
			up.GitHubUserScopesJson = sql.NullString{`["repo", "write:public_key"]`, true}
			Expect(up.CanDeleteGitHubPublicKeys()).To(Equal(false))
		})

		It("returns true when github scopes contains `admin:public_key`", func() {
			up.GitHubUserScopesJson = sql.NullString{`["repo", "admin:public_key"]`, true}
			Expect(up.CanDeleteGitHubPublicKeys()).To(Equal(true))
		})
	})

	Describe("AuthorizedGitHubUserName", func() {
		var up *userprofile.UserProfile

		BeforeEach(func() {
			up = &userprofile.UserProfile{}
		})

		It("returns empty string when username is empty", func() {
			Expect(up.AuthorizedGitHubUserName()).To(Equal(""))
		})

		It("returns empty string when token is not empty", func() {
			up.GitHubUserName = sql.NullString{"spongebob", true}
			Expect(up.AuthorizedGitHubUserName()).To(Equal(""))
		})

		It("returns username when github username and token are valid", func() {
			up.GitHubOAuthTokenJson = sql.NullString{`{"access_token":"746d66f0dfa22fbb73a994a2ec621ef6e52c05ef","token_type":"bearer","expiry":"0001-01-01T00:00:00Z"}`, true}
			up.GitHubUserName = sql.NullString{"spongebob", true}
			Expect(up.AuthorizedGitHubUserName()).To(Equal("spongebob"))
		})
	})

	Describe("GetTokenIfValidFromUserProfile", func() {
		var up *userprofile.UserProfile

		BeforeEach(func() {
			up = &userprofile.UserProfile{}
		})

		It("returns error when token is empty", func() {
			_, err := up.GetTokenIfValidFromUserProfile()
			Expect(err.Error()).To(Equal("token is empty"))
		})

		It("returns error when token not valid", func() {
			up.GitHubOAuthTokenJson = sql.NullString{`{"access_token":"","token_type":"bearer","expiry":"0001-01-01T00:00:00Z"}`, true}
			_, err := up.GetTokenIfValidFromUserProfile()
			Expect(err.Error()).To(Equal("token not valid"))
		})

		It("returns token", func() {
			up.GitHubOAuthTokenJson = sql.NullString{`{"access_token":"123","token_type":"bearer","expiry":"0001-01-01T00:00:00Z"}`, true}
			t, err := up.GetTokenIfValidFromUserProfile()
			Expect(err).To(BeNil())
			Expect(t.AccessToken).To(Equal("123"))
			Expect(t.TokenType).To(Equal("bearer"))
		})
	})

	Describe("GetTokenIfValid", func() {
		var up *userprofile.UserProfile

		BeforeEach(func() {
			up = &userprofile.UserProfile{}
		})

		It("returns error when no user profile", func() {
			_, err := userprofile.GetTokenIfValid(db, u.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("return error when token is empty", func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}

			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = userprofile.GetTokenIfValid(db, u.ID)
			Expect(err.Error()).To(Equal("token is empty"))
		})
	})

})
