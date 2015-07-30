package orgmembership_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/orgmembership")
}

var _ = Describe("models/orgmembership", func() {
	var (
		db        *db.Database
		o, o2     *org.Org
		u, u2, u3 *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		o = &org.Org{
			Slug:  "Foobar",
			Name:  "Foo Bar, Inc.",
			Email: "eng@foobar.com",
		}
		err = o.Insert(db)
		Expect(err).To(BeNil())

		o2 = &org.Org{
			Name:  "Macrosoft Corp.",
			Slug:  "macrosoft",
			Email: "bill@macrosoft.com",
		}
		err = o2.Insert(db)
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
			Name:     "Mr. Foo",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		u2 = &user.User{
			Username: "bar",
			Email:    "bar@example.com",
			Password: "123456",
			Name:     "Ms. Bar",
		}
		err = test.WrapInTransaction(db, u2.Insert)
		Expect(err).To(BeNil())

		u3 = &user.User{
			Username: "baz",
			Email:    "baz@example.com",
			Password: "123456",
			Name:     "Mrs. Baz",
		}
		err = test.WrapInTransaction(db, u3.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAllByOrgID()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "owner")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o2.ID, u2.ID, "owner")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u3.ID, "admin")
			Expect(err).To(BeNil())
		})

		It("returns all members for a given org", func() {
			m, err := orgmembership.FindAllByOrgID(db, o.ID)
			Expect(err).To(BeNil())
			Expect(m).To(HaveLen(2))
			Expect(m[0].Username).To(Equal("foo"))
			Expect(m[0].Email).To(Equal("foo@example.com"))
			Expect(m[0].Name).To(Equal("Mr. Foo"))
			Expect(m[0].Role).To(Equal("owner"))
			Expect(m[1].Username).To(Equal("baz"))
			Expect(m[1].Email).To(Equal("baz@example.com"))
			Expect(m[1].Name).To(Equal("Mrs. Baz"))
			Expect(m[1].Role).To(Equal("admin"))
		})

		Context("when an error occurs", func() {
			It("returns error", func() {
				db.Close()
				_, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByUsernameAndOrgSlug()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "owner")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o2.ID, u2.ID, "admin")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u3.ID, "member")
			Expect(err).To(BeNil())
		})

		It("returns an org membership", func() {
			m, err := orgmembership.FindByUsernameAndOrgSlug(db, u.Username, o.Slug)
			Expect(err).To(BeNil())
			Expect(m.OrgID).To(Equal(o.ID))
			Expect(m.UserID).To(Equal(u.ID))
			Expect(m.Role).To(Equal("owner"))

			m, err = orgmembership.FindByUsernameAndOrgSlug(db, u2.Username, o2.Slug)
			Expect(err).To(BeNil())
			Expect(m.OrgID).To(Equal(o2.ID))
			Expect(m.UserID).To(Equal(u2.ID))
			Expect(m.Role).To(Equal("admin"))

			m, err = orgmembership.FindByUsernameAndOrgSlug(db, u3.Username, o.Slug)
			Expect(err).To(BeNil())
			Expect(m.OrgID).To(Equal(o.ID))
			Expect(m.UserID).To(Equal(u3.ID))
			Expect(m.Role).To(Equal("member"))
		})

		Context("when there is no match", func() {
			It("returns error", func() {
				_, err := orgmembership.FindByUsernameAndOrgSlug(db, u.Username, o2.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				_, err = orgmembership.FindByUsernameAndOrgSlug(db, u2.Username, o.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				_, err = orgmembership.FindByUsernameAndOrgSlug(db, u3.Username, o2.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				db.Close()
				_, err := orgmembership.FindByUsernameAndOrgSlug(db, u.Username, o.Slug)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("AddMember()", func() {
		Context("when an invalid role is given", func() {
			It("returns error", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "destroyer")
				Expect(err).To(MatchError("invalid role: destroyer"))
			})
		})

		Context("when a non-existent org id is given", func() {
			It("returns error", func() {
				err := orgmembership.AddMember(db, o.ID+9000, u.ID, "member")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))
			})
		})

		Context("when a non-existent user id is given", func() {
			It("returns error", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID+9000, "member")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))
			})
		})

		Context("when a user with a given id exists", func() {
			It("adds user as an org member", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o2.ID, u2.ID, "admin")
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o.ID, u3.ID, "member")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(2))
				Expect(m[0].Username).To(Equal("foo"))
				Expect(m[0].Email).To(Equal("foo@example.com"))
				Expect(m[0].Name).To(Equal("Mr. Foo"))
				Expect(m[0].Role).To(Equal("owner"))
				Expect(m[1].Username).To(Equal("baz"))
				Expect(m[1].Email).To(Equal("baz@example.com"))
				Expect(m[1].Name).To(Equal("Mrs. Baz"))
				Expect(m[1].Role).To(Equal("member"))
			})

			Context("when user is already a member", func() {
				It("returns error", func() {
					err := orgmembership.AddMember(db, o.ID, u.ID, "owner")
					Expect(err).To(BeNil())
					err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
					Expect(err).NotTo(BeNil())
				})
			})
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				db.Close()
				err := orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("UpdateMember()", func() {
		Context("when an invalid role is given", func() {
			It("returns error", func() {
				err := orgmembership.UpdateMember(db, o.ID, u.ID, "destroyer")
				Expect(err).To(MatchError("invalid role: destroyer"))
			})
		})

		Context("when a non-existent org id is given", func() {
			It("returns error", func() {
				err := orgmembership.UpdateMember(db, o.ID+9000, u.ID, "member")
				Expect(err).NotTo(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a non-existent user id is given", func() {
			It("returns error", func() {
				err := orgmembership.UpdateMember(db, o.ID, u.ID+9000, "member")
				Expect(err).NotTo(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a user with a given id exists", func() {
			Context("when user is not already a member", func() {
				It("returns error", func() {
					err := orgmembership.UpdateMember(db, o.ID, u.ID, "admin")
					Expect(err).NotTo(BeNil())
					err = orgmembership.UpdateMember(db, o2.ID, u2.ID, "member")
					Expect(err).NotTo(BeNil())
					err = orgmembership.UpdateMember(db, o.ID, u3.ID, "owner")
					Expect(err).NotTo(BeNil())
				})
			})

			It("updates user's role", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o2.ID, u2.ID, "admin")
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o.ID, u3.ID, "member")
				Expect(err).To(BeNil())

				err = orgmembership.UpdateMember(db, o.ID, u.ID, "admin")
				Expect(err).To(BeNil())
				err = orgmembership.UpdateMember(db, o2.ID, u2.ID, "member")
				Expect(err).To(BeNil())
				err = orgmembership.UpdateMember(db, o.ID, u3.ID, "owner")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(2))
				Expect(m[0].Username).To(Equal(u.Username))
				Expect(m[0].Email).To(Equal(u.Email))
				Expect(m[0].Name).To(Equal(u.Name))
				Expect(m[0].Role).To(Equal("admin"))
				Expect(m[1].Username).To(Equal(u3.Username))
				Expect(m[1].Email).To(Equal(u3.Email))
				Expect(m[1].Name).To(Equal(u3.Name))
				Expect(m[1].Role).To(Equal("owner"))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(1))
				Expect(m[0].Username).To(Equal(u2.Username))
				Expect(m[0].Email).To(Equal(u2.Email))
				Expect(m[0].Name).To(Equal(u2.Name))
				Expect(m[0].Role).To(Equal("member"))
			})
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "owner")
				Expect(err).To(BeNil())
				db.Close()
				err = orgmembership.UpdateMember(db, o.ID, u.ID, "owner")
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
