package orgmembership_test

import (
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

	Describe("FindAllByOrgId()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "owner")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o2.ID, u2.ID, "owner")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u3.ID, "admin")
			Expect(err).To(BeNil())
		})

		It("returns all members for a given org", func() {
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
			Expect(m[1].Role).To(Equal("admin"))
		})
	})
})
