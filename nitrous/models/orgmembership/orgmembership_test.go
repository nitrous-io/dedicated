package orgmembership_test

import (
	"database/sql"
	"fmt"
	"strconv"
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

	Describe("IsAdmin()", func() {
		It("should be correct for the role", func() {
			roles := map[string]bool{
				"owner":  true,
				"admin":  true,
				"member": false,
			}

			o := &orgmembership.OrgMembership{}
			for role, val := range roles {
				o.Role = role
				Expect(o.IsAdmin()).To(Equal(val), fmt.Sprintf("Should be %v for %s", val, role))
			}
		})
	})

	Describe("SnapshotPublishAllowed()", func() {
		It("returns true for owners, admins and members", func() {
			testCases := map[string]bool{
				orgmembership.RoleOwner:   true,
				orgmembership.RoleAdmin:   true,
				orgmembership.RoleMember:  true,
				orgmembership.RoleLimited: false,
			}

			o := &orgmembership.OrgMembership{}
			for role, expected := range testCases {
				o.Role = role
				result := o.SnapshotPublishAllowed()
				Expect(result).To(Equal(expected), fmt.Sprintf("expected SnapshotPublishAllowed() for role %q to be %v, but got %v", role, expected, result))
			}
		})
	})

	Describe("SnapshotDeleteAllowed()", func() {
		It("returns true for owners and admins", func() {
			testCases := map[string]bool{
				orgmembership.RoleOwner:   true,
				orgmembership.RoleAdmin:   true,
				orgmembership.RoleMember:  false,
				orgmembership.RoleLimited: false,
			}

			o := &orgmembership.OrgMembership{}
			for role, expected := range testCases {
				o.Role = role

				result := o.SnapshotDeleteAllowed()
				Expect(result).To(Equal(expected), fmt.Sprintf("expected SnapshotDeleteAllowed() for role %q to be %v, but got %v", role, expected, result))
			}
		})
	})

	Describe("SnapshotReadAllowed()", func() {
		It("returns true for owners, admins and members", func() {
			testCases := map[string]bool{
				orgmembership.RoleOwner:   true,
				orgmembership.RoleAdmin:   true,
				orgmembership.RoleMember:  true,
				orgmembership.RoleLimited: false,
			}

			o := &orgmembership.OrgMembership{}
			for role, expected := range testCases {
				o.Role = role
				result := o.SnapshotReadAllowed()
				Expect(result).To(Equal(expected), fmt.Sprintf("expected SnapshotReadAllowed() for role %q to be %v, but got %v", role, expected, result))
			}
		})
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
			var omID, om2ID int64
			err := db.QueryRow("SELECT id FROM org_memberships WHERE org_id = $1 AND user_id = $2", o.ID, u.ID).Scan(&omID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM org_memberships WHERE org_id = $1 AND user_id = $2", o.ID, u3.ID).Scan(&om2ID)
			Expect(err).To(BeNil())

			m, err := orgmembership.FindAllByOrgID(db, o.ID)
			Expect(err).To(BeNil())
			Expect(m).To(HaveLen(2))
			Expect(m[0].ID).To(Equal(omID))
			Expect(m[0].OrgID).To(Equal(o.ID))
			Expect(m[0].OrgSlug).To(Equal(o.Slug))
			Expect(m[0].UserID.Int64).To(Equal(u.ID))
			Expect(m[0].Username.String).To(Equal("foo"))
			Expect(m[0].Email.String).To(Equal("foo@example.com"))
			Expect(m[0].Name.String).To(Equal("Mr. Foo"))
			Expect(m[0].Role).To(Equal("owner"))
			Expect(m[1].ID).To(Equal(om2ID))
			Expect(m[1].OrgID).To(Equal(o.ID))
			Expect(m[1].OrgSlug).To(Equal(o.Slug))
			Expect(m[1].UserID.Int64).To(Equal(u3.ID))
			Expect(m[1].Username.String).To(Equal("baz"))
			Expect(m[1].Email.String).To(Equal("baz@example.com"))
			Expect(m[1].Name.String).To(Equal("Mrs. Baz"))
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

	Describe("FindByIdentifierAndOrgSlug()", func() {
		Context("when user exists", func() {
			BeforeEach(func() {
				_, err := db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "owner")
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o2.ID, u2.ID, "admin")
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u3.ID, "member")
				Expect(err).To(BeNil())
			})

			Context("when username is provided", func() {
				It("returns an org membership", func() {
					m, err := orgmembership.FindByIdentifierAndOrgSlug(db, u.Username, o.Slug)
					Expect(err).To(BeNil())
					Expect(m.OrgID).To(Equal(o.ID))
					Expect(m.UserID.Int64).To(Equal(u.ID))
					Expect(m.Role).To(Equal("owner"))

					m, err = orgmembership.FindByIdentifierAndOrgSlug(db, u2.Username, o2.Slug)
					Expect(err).To(BeNil())
					Expect(m.OrgID).To(Equal(o2.ID))
					Expect(m.UserID.Int64).To(Equal(u2.ID))
					Expect(m.Role).To(Equal("admin"))

					m, err = orgmembership.FindByIdentifierAndOrgSlug(db, u3.Username, o.Slug)
					Expect(err).To(BeNil())
					Expect(m.OrgID).To(Equal(o.ID))
					Expect(m.UserID.Int64).To(Equal(u3.ID))
					Expect(m.Role).To(Equal("member"))
				})

				Context("when there is no match", func() {
					It("returns error", func() {
						_, err := orgmembership.FindByIdentifierAndOrgSlug(db, u.Username, o2.Slug)
						Expect(err).To(MatchError(sql.ErrNoRows))
						_, err = orgmembership.FindByIdentifierAndOrgSlug(db, u2.Username, o.Slug)
						Expect(err).To(MatchError(sql.ErrNoRows))
						_, err = orgmembership.FindByIdentifierAndOrgSlug(db, u3.Username, o2.Slug)
						Expect(err).To(MatchError(sql.ErrNoRows))
					})
				})

				Context("when some other error occurs", func() {
					It("returns error", func() {
						db.Close()
						_, err := orgmembership.FindByIdentifierAndOrgSlug(db, u.Username, o.Slug)
						Expect(err).NotTo(BeNil())
					})
				})
			})

			Context("when userid is provided", func() {
				It("returns an org membership", func() {
					m, err := orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u.ID, 10), o.Slug)
					Expect(err).To(BeNil())
					Expect(m.OrgID).To(Equal(o.ID))
					Expect(m.UserID.Int64).To(Equal(u.ID))
					Expect(m.Role).To(Equal("owner"))

					m, err = orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u2.ID, 10), o2.Slug)
					Expect(err).To(BeNil())
					Expect(m.OrgID).To(Equal(o2.ID))
					Expect(m.UserID.Int64).To(Equal(u2.ID))
					Expect(m.Role).To(Equal("admin"))

					m, err = orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u3.ID, 10), o.Slug)
					Expect(err).To(BeNil())
					Expect(m.OrgID).To(Equal(o.ID))
					Expect(m.UserID.Int64).To(Equal(u3.ID))
					Expect(m.Role).To(Equal("member"))
				})

				Context("when there is no match", func() {
					It("returns error", func() {
						_, err := orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u.ID, 10), o2.Slug)
						Expect(err).To(MatchError(sql.ErrNoRows))
						_, err = orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u2.ID, 10), o.Slug)
						Expect(err).To(MatchError(sql.ErrNoRows))
						_, err = orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u3.ID, 10), o2.Slug)
						Expect(err).To(MatchError(sql.ErrNoRows))
					})
				})

				Context("when some other error occurs", func() {
					It("returns error", func() {
						db.Close()
						_, err := orgmembership.FindByIdentifierAndOrgSlug(db, strconv.FormatInt(u.ID, 10), o.Slug)
						Expect(err).NotTo(BeNil())
					})
				})
			})

		})

		Context("when user is pending email invite", func() {
			BeforeEach(func() {
				_, err := db.Exec("INSERT INTO org_memberships (org_id, invite_email, role, invite_code, email_sent_at) values ($1, $2, $3, $4, now());", o.ID, "member@bob.com", "member", "123abc")
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO org_memberships (org_id, invite_email, role, invite_code, email_sent_at) values ($1, $2, $3, $4, now());", o.ID, "admin@bob.com", "admin", "123abc")
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO org_memberships (org_id, invite_email, role, invite_code, email_sent_at) values ($1, $2, $3, $4, now());", o.ID, "owner@bob.com", "owner", "123abc")
				Expect(err).To(BeNil())
			})

			It("returns an org membership", func() {
				m1, err := orgmembership.FindByIdentifierAndOrgSlug(db, "member@bob.com", o.Slug)
				Expect(err).To(BeNil())
				Expect(m1.OrgID).To(Equal(o.ID))
				Expect(m1.InviteEmail.String).To(Equal("member@bob.com"))
				Expect(m1.Role).To(Equal("member"))

				m2, err := orgmembership.FindByIdentifierAndOrgSlug(db, "admin@bob.com", o.Slug)
				Expect(err).To(BeNil())
				Expect(m2.OrgID).To(Equal(o.ID))
				Expect(m2.InviteEmail.String).To(Equal("admin@bob.com"))
				Expect(m2.Role).To(Equal("admin"))

				m3, err := orgmembership.FindByIdentifierAndOrgSlug(db, "owner@bob.com", o.Slug)
				Expect(err).To(BeNil())
				Expect(m3.OrgID).To(Equal(o.ID))
				Expect(m3.InviteEmail.String).To(Equal("owner@bob.com"))
				Expect(m3.Role).To(Equal("owner"))
			})

			Context("when there is no match", func() {
				It("returns error", func() {
					_, err := orgmembership.FindByIdentifierAndOrgSlug(db, "member@bob.com", o2.Slug)
					Expect(err).To(MatchError(sql.ErrNoRows))
				})
			})

			Context("when some other error occurs", func() {
				It("returns error", func() {
					db.Close()
					_, err := orgmembership.FindByIdentifierAndOrgSlug(db, "member@bob.com", o.Slug)
					Expect(err).NotTo(BeNil())
				})
			})
		})
	})

	Describe("FindByUserIDAndOrgID()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "owner")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o2.ID, u2.ID, "admin")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u3.ID, "member")
			Expect(err).To(BeNil())
		})

		It("returns an org membership", func() {
			m, err := orgmembership.FindByUserIDAndOrgID(db, u.ID, o.ID)
			Expect(err).To(BeNil())
			Expect(m.OrgID).To(Equal(o.ID))
			Expect(m.UserID.Int64).To(Equal(u.ID))
			Expect(m.Role).To(Equal("owner"))

			m, err = orgmembership.FindByUserIDAndOrgID(db, u2.ID, o2.ID)
			Expect(err).To(BeNil())
			Expect(m.OrgID).To(Equal(o2.ID))
			Expect(m.UserID.Int64).To(Equal(u2.ID))
			Expect(m.Role).To(Equal("admin"))

			m, err = orgmembership.FindByUserIDAndOrgID(db, u3.ID, o.ID)
			Expect(err).To(BeNil())
			Expect(m.OrgID).To(Equal(o.ID))
			Expect(m.UserID.Int64).To(Equal(u3.ID))
			Expect(m.Role).To(Equal("member"))
		})

		Context("when there is no match", func() {
			It("returns error", func() {
				_, err := orgmembership.FindByUserIDAndOrgID(db, u.ID, o2.ID)
				Expect(err).To(MatchError(sql.ErrNoRows))
				_, err = orgmembership.FindByUserIDAndOrgID(db, u2.ID, o.ID)
				Expect(err).To(MatchError(sql.ErrNoRows))
				_, err = orgmembership.FindByUserIDAndOrgID(db, u3.ID, o2.ID)
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})
	})

	Describe("AddMember()", func() {
		Context("when an invalid role is given", func() {
			It("returns error", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "destroyer")
				Expect(err).To(MatchError("invalid role: destroyer"))
			})
		})

		Context("when a non-existent org id is given", func() {
			It("returns error", func() {
				_, err := orgmembership.AddMember(db, o.ID+9000, strconv.FormatInt(u.ID, 10), "member")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))
			})
		})

		Context("when a non-existent user id is given", func() {
			It("returns error", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID+9000, 10), "member")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))
			})
		})

		Context("when a user with a given id exists", func() {
			It("adds user as an org member", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "admin")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u3.ID, 10), "member")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(2))
				Expect(m[0].Username.String).To(Equal("foo"))
				Expect(m[0].Email.String).To(Equal("foo@example.com"))
				Expect(m[0].Name.String).To(Equal("Mr. Foo"))
				Expect(m[0].Role).To(Equal("owner"))
				Expect(m[1].Username.String).To(Equal("baz"))
				Expect(m[1].Email.String).To(Equal("baz@example.com"))
				Expect(m[1].Name.String).To(Equal("Mrs. Baz"))
				Expect(m[1].Role).To(Equal("member"))
			})

			Context("when user is already a member", func() {
				It("returns error", func() {
					_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
					Expect(err).To(BeNil())
					_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
					Expect(err).NotTo(BeNil())
				})
			})
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				db.Close()
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
				Expect(err).NotTo(BeNil())
			})
		})

		Context("when pending email is given", func() {
			Context("when an invalid role is given", func() {
				It("returns error", func() {
					_, err := orgmembership.AddMember(db, o.ID, "member@bob.com", "destroyer")
					Expect(err).To(MatchError("invalid role: destroyer"))
				})
			})

			Context("when a non-existent org id is given", func() {
				It("returns error", func() {
					_, err := orgmembership.AddMember(db, o.ID+9000, "member@bob.com", "member")
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))
				})
			})

			Context("when email is provided", func() {
				It("adds pending user as an org member", func() {
					_, err := orgmembership.AddMember(db, o.ID, "owner@bob.com", "owner")
					Expect(err).To(BeNil())
					_, err = orgmembership.AddMember(db, o2.ID, "admin@bob.com", "admin")
					Expect(err).To(BeNil())
					_, err = orgmembership.AddMember(db, o.ID, "member@bob.com", "member")
					Expect(err).To(BeNil())

					m, err := orgmembership.FindAllByOrgID(db, o.ID)
					Expect(err).To(BeNil())
					Expect(m).To(HaveLen(2))
					Expect(m[0].InviteEmail.String).To(Equal("owner@bob.com"))
					Expect(m[0].Role).To(Equal("owner"))
					Expect(m[0].InviteCode.Valid).To(BeTrue())
					Expect(m[1].InviteEmail.String).To(Equal("member@bob.com"))
					Expect(m[1].Role).To(Equal("member"))
					Expect(m[1].InviteCode.Valid).To(BeTrue())
				})

				Context("when pending user is already a member", func() {
					It("returns error", func() {
						_, err := orgmembership.AddMember(db, o.ID, "owner@bob.com", "owner")
						Expect(err).To(BeNil())
						_, err = orgmembership.AddMember(db, o.ID, "owner@bob.com", "owner")
						Expect(err).NotTo(BeNil())
					})
				})

				Context("when some other error occurs", func() {
					It("returns error", func() {
						db.Close()
						_, err := orgmembership.AddMember(db, o.ID, "owner@bob.com", "owner")
						Expect(err).NotTo(BeNil())
					})
				})
			})
		})
	})

	Describe("UpdateMember()", func() {
		Context("when an invalid role is given", func() {
			It("returns error", func() {
				err := orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u.ID, 10), "destroyer")
				Expect(err).To(MatchError("invalid role: destroyer"))
			})
		})

		Context("when a non-existent org id is given", func() {
			It("returns error", func() {
				err := orgmembership.UpdateMember(db, o.ID+9000, strconv.FormatInt(u.ID, 10), "member")
				Expect(err).NotTo(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a non-existent user id is given", func() {
			It("returns error", func() {
				err := orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u.ID+9000, 10), "member")
				Expect(err).NotTo(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a user with a given id exists", func() {
			Context("when user is not already a member", func() {
				It("returns error", func() {
					err := orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u.ID, 10), "admin")
					Expect(err).NotTo(BeNil())
					err = orgmembership.UpdateMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "member")
					Expect(err).NotTo(BeNil())
					err = orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u3.ID, 10), "owner")
					Expect(err).NotTo(BeNil())
				})
			})

			It("updates user's role", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "admin")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u3.ID, 10), "member")
				Expect(err).To(BeNil())

				err = orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u.ID, 10), "admin")
				Expect(err).To(BeNil())
				err = orgmembership.UpdateMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "member")
				Expect(err).To(BeNil())
				err = orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u3.ID, 10), "owner")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(2))
				Expect(m[0].Username.String).To(Equal(u.Username))
				Expect(m[0].Email.String).To(Equal(u.Email))
				Expect(m[0].Name.String).To(Equal(u.Name))
				Expect(m[0].Role).To(Equal("admin"))
				Expect(m[1].Username.String).To(Equal(u3.Username))
				Expect(m[1].Email.String).To(Equal(u3.Email))
				Expect(m[1].Name.String).To(Equal(u3.Name))
				Expect(m[1].Role).To(Equal("owner"))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(1))
				Expect(m[0].Username.String).To(Equal(u2.Username))
				Expect(m[0].Email.String).To(Equal(u2.Email))
				Expect(m[0].Name.String).To(Equal(u2.Name))
				Expect(m[0].Role).To(Equal("member"))
			})
		})

		Context("when updating pending member", func() {
			Context("when pending user is not already a member", func() {
				It("returns error", func() {
					err := orgmembership.UpdateMember(db, o.ID, "bob@admin.com", "admin")
					Expect(err).NotTo(BeNil())
					err = orgmembership.UpdateMember(db, o2.ID, "bob@member.com", "member")
					Expect(err).NotTo(BeNil())
					err = orgmembership.UpdateMember(db, o.ID, "bob@owner.com", "owner")
					Expect(err).NotTo(BeNil())
				})
			})

			It("updates pending user's role", func() {
				_, err := orgmembership.AddMember(db, o.ID, "bob@owner.com", "owner")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o2.ID, "bob@member.com", "member")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, "bob@admin.com", "admin")
				Expect(err).To(BeNil())

				err = orgmembership.UpdateMember(db, o.ID, "bob@owner.com", "admin")
				Expect(err).To(BeNil())
				err = orgmembership.UpdateMember(db, o2.ID, "bob@member.com", "owner")
				Expect(err).To(BeNil())
				err = orgmembership.UpdateMember(db, o.ID, "bob@admin.com", "member")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(2))
				Expect(m[0].Username.Valid).To(BeFalse())
				Expect(m[0].Name.Valid).To(BeFalse())
				Expect(m[0].InviteEmail.String).To(Equal("bob@owner.com"))
				Expect(m[0].Role).To(Equal("admin"))
				Expect(m[1].Username.Valid).To(BeFalse())
				Expect(m[1].InviteEmail.String).To(Equal("bob@admin.com"))
				Expect(m[1].Name.Valid).To(BeFalse())
				Expect(m[1].Role).To(Equal("member"))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(1))
				Expect(m[0].Username.Valid).To(BeFalse())
				Expect(m[0].Name.Valid).To(BeFalse())
				Expect(m[0].InviteEmail.String).To(Equal("bob@member.com"))
				Expect(m[0].Role).To(Equal("owner"))
			})
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
				Expect(err).To(BeNil())
				db.Close()
				err = orgmembership.UpdateMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("RemoveMember()", func() {
		Context("when user exists", func() {
			Context("when a non-existent org id is given", func() {
				It("returns error", func() {
					err := orgmembership.RemoveMember(db, o.ID+9000, strconv.FormatInt(u.ID, 10))
					Expect(err).NotTo(BeNil())
					Expect(err).To(MatchError(sql.ErrNoRows))
				})
			})

			Context("when a non-existent user id is given", func() {
				It("returns error", func() {
					err := orgmembership.RemoveMember(db, o.ID, strconv.FormatInt(u.ID+9000, 10))
					Expect(err).NotTo(BeNil())
					Expect(err).To(MatchError(sql.ErrNoRows))
				})
			})

			Context("when a user with a given id exists", func() {
				Context("when user is not already a member", func() {
					It("returns error", func() {
						err := orgmembership.RemoveMember(db, o.ID, strconv.FormatInt(u.ID, 10))
						Expect(err).NotTo(BeNil())
						err = orgmembership.RemoveMember(db, o2.ID, strconv.FormatInt(u2.ID, 10))
						Expect(err).NotTo(BeNil())
						err = orgmembership.RemoveMember(db, o.ID, strconv.FormatInt(u3.ID, 10))
						Expect(err).NotTo(BeNil())
					})
				})

				It("removes user", func() {
					_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
					Expect(err).To(BeNil())
					_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "admin")
					Expect(err).To(BeNil())
					_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u3.ID, 10), "member")
					Expect(err).To(BeNil())

					err = orgmembership.RemoveMember(db, o.ID, strconv.FormatInt(u.ID, 10))
					Expect(err).To(BeNil())
					err = orgmembership.RemoveMember(db, o2.ID, strconv.FormatInt(u2.ID, 10))
					Expect(err).To(BeNil())

					m, err := orgmembership.FindAllByOrgID(db, o.ID)
					Expect(err).To(BeNil())
					Expect(m).To(HaveLen(1))
					Expect(m[0].Username.String).To(Equal(u3.Username))
					Expect(m[0].Email.String).To(Equal(u3.Email))
					Expect(m[0].Name.String).To(Equal(u3.Name))
					Expect(m[0].Role).To(Equal("member"))

					m, err = orgmembership.FindAllByOrgID(db, o2.ID)
					Expect(err).To(BeNil())
					Expect(m).To(HaveLen(0))
				})
			})

			Context("when some other error occurs", func() {
				It("returns error", func() {
					_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
					Expect(err).To(BeNil())
					db.Close()
					err = orgmembership.RemoveMember(db, o.ID, strconv.FormatInt(u.ID, 10))
					Expect(err).NotTo(BeNil())
				})
			})
		})

		Context("when user pending email invite", func() {
			Context("when a non-existent org id is given", func() {
				It("returns error", func() {
					err := orgmembership.RemoveMember(db, o.ID+9000, "member@bob.com")
					Expect(err).NotTo(BeNil())
					Expect(err).To(MatchError(sql.ErrNoRows))
				})
			})

			Context("when a non-existent invite email is given", func() {
				It("returns error", func() {
					err := orgmembership.RemoveMember(db, o.ID, "member@bob.com")
					Expect(err).NotTo(BeNil())
					Expect(err).To(MatchError(sql.ErrNoRows))
				})
			})

			Context("when the given invite email exists", func() {
				BeforeEach(func() {
					_, err := db.Exec("INSERT INTO org_memberships (org_id, invite_email, role, invite_code, email_sent_at) values ($1, $2, $3, $4, now());", o.ID, "member@bob.com", "member", "123abc")
					Expect(err).To(BeNil())
					_, err = db.Exec("INSERT INTO org_memberships (org_id, invite_email, role, invite_code, email_sent_at) values ($1, $2, $3, $4, now());", o.ID, "admin@bob.com", "admin", "123abc")
					Expect(err).To(BeNil())
					_, err = db.Exec("INSERT INTO org_memberships (org_id, invite_email, role, invite_code, email_sent_at) values ($1, $2, $3, $4, now());", o.ID, "owner@bob.com", "owner", "123abc")
					Expect(err).To(BeNil())
				})

				It("removes pending invite user", func() {
					err := orgmembership.RemoveMember(db, o.ID, "member@bob.com")
					Expect(err).To(BeNil())
					err = orgmembership.RemoveMember(db, o.ID, "admin@bob.com")
					Expect(err).To(BeNil())
					err = orgmembership.RemoveMember(db, o.ID, "owner@bob.com")
					Expect(err).To(BeNil())

					m, err := orgmembership.FindAllByOrgID(db, o.ID)
					Expect(err).To(BeNil())
					Expect(m).To(HaveLen(0))
				})
			})

			Context("when some other error occurs", func() {
				It("returns error", func() {
					db.Close()
					err := orgmembership.RemoveMember(db, o.ID, "member@bob.com")
					Expect(err).NotTo(BeNil())
				})
			})
		})
	})
})
