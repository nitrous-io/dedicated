package org_test

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
	RunSpecs(t, "models/org")
}

var _ = Describe("models/org", func() {
	var db *db.Database

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO orgs (name, email, slug) values ($1, $2, $3);", "Foo Bar, Inc.", "eng@foobar.com", "foobar")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO orgs (name, email, slug) values ($1, $2, $3);", "Watermelon Corp.", "admin@watermelon.co", "watermelon")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO orgs (name, email, slug, deleted_at) values ($1, $2, $3, now());", "Action Enterprises", "dev@actionent.io", "action-enterprises")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindBySlug()", func() {
		var foobarID, watermelonID int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM orgs WHERE slug = $1", "foobar").Scan(&foobarID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM orgs WHERE slug = $1", "watermelon").Scan(&watermelonID)
			Expect(err).To(BeNil())
		})

		Context("when a record with a given slug exists", func() {
			It("returns the record", func() {
				o, err := org.FindBySlug(db, "foobar")
				Expect(err).To(BeNil())
				Expect(o).NotTo(BeNil())
				Expect(o.ID).To(Equal(foobarID))
				Expect(o.Name).To(Equal("Foo Bar, Inc."))
				Expect(o.Slug).To(Equal("foobar"))
				Expect(o.Email).To(Equal("eng@foobar.com"))

				o2, err := org.FindBySlug(db, "watermelon")
				Expect(err).To(BeNil())
				Expect(o2).NotTo(BeNil())
				Expect(o2).NotTo(Equal(o))
				Expect(o2.ID).To(Equal(watermelonID))
				Expect(o2.Name).To(Equal("Watermelon Corp."))
				Expect(o2.Slug).To(Equal("watermelon"))
				Expect(o2.Email).To(Equal("admin@watermelon.co"))
			})
		})

		Context("when a record with a given slug does not exist", func() {
			It("returns error", func() {
				o, err := org.FindBySlug(db, "acme")
				Expect(o).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a record with a given slug is deleted", func() {
			It("returns error", func() {
				o, err := org.FindBySlug(db, "action-enterprise")
				Expect(o).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				o, err := org.FindBySlug(db, "foobar")
				Expect(o).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var o *org.Org

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
		})

		Context("when no error happens", func() {
			It("persists the org in the database", func() {
				err := o.Insert(db)
				Expect(err).To(BeNil())

				o2, err := org.FindBySlug(db, "macrosoft")
				Expect(err).To(BeNil())
				Expect(o2).NotTo(BeNil())
				Expect(o2.Name).To(Equal("Macrosoft Corp."))
				Expect(o2.Slug).To(Equal("macrosoft"))
				Expect(o2.Email).To(Equal("bill@macrosoft.com"))
				Expect(o2.CreatedAt).NotTo(BeNil())
				Expect(o2.UpdatedAt).NotTo(BeNil())
			})
		})

		Context("when slug is taken", func() {
			It("returns error", func() {
				o.Slug = "foobar"
				err := o.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})

		Context("when some error occurs", func() {
			It("returns error", func() {
				db.Close()
				err := o.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Validate()", func() {
		var o *org.Org

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			Expect(o.Validate()).To(BeNil())
		})

		It("validates email", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", org.ErrEmailBlank},
				{"foo", org.ErrEmailInvalid},
				{"foo@gmail.com", nil},
				{"foo+bar@gmail.com", nil},
				{"foo+bar@hello.com.sg", nil},
			}

			for _, t := range tests {
				o.Email = t.input
				Expect(o.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates email with email blacklist", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"foo@yopmail.com", org.ErrEmailBlacklisted},
				{"foo@yopmail.fr", org.ErrEmailBlacklisted},
				{"foo@gmail.com", nil},
			}

			for _, t := range tests {
				o.Email = t.input
				Expect(o.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates slug", func() {
			buf := make([]byte, 64)
			for i := range buf {
				buf[i] = 'f'
			}
			s := string(buf) // "f" * 64.

			tests := []struct {
				input string
				err   error
			}{
				{"", org.ErrSlugBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", org.ErrSlugTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, org.ErrSlugTooLong},
				{s[:63], nil},
				{"f00", nil},
				{"f-o-0", nil},
				{"f-o-0-", org.ErrSlugInvalid},
				{"-foo", org.ErrSlugInvalid},
				{"foo_bar", org.ErrSlugInvalid},
				{"FOO", org.ErrSlugInvalid},
			}

			for _, t := range tests {
				o.Slug = t.input
				Expect(o.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})

	Describe("AddMember()", func() {
		var (
			o, o2     *org.Org
			u, u2, u3 *user.User
		)

		BeforeEach(func() {
			var err error
			o, err = org.FindBySlug(db, "foobar")
			Expect(err).To(BeNil())
			o2, err = org.FindBySlug(db, "watermelon")
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

		Context("when an invalid role is given", func() {
			It("returns error", func() {
				err := o.AddMember(db, u.ID, "destroyer")
				Expect(err).To(MatchError("invalid role: destroyer"))
			})
		})

		Context("when a non-existent user id is given", func() {
			It("returns error", func() {
				err := o.AddMember(db, u.ID+9000, "member")
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))
			})
		})

		Context("when a user with a given id exists", func() {
			It("adds user as an org member", func() {
				err := o.AddMember(db, u.ID, "owner")
				Expect(err).To(BeNil())
				err = o2.AddMember(db, u2.ID, "admin")
				Expect(err).To(BeNil())
				err = o.AddMember(db, u3.ID, "member")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgId(db, o.ID)
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
		})
	})
})
