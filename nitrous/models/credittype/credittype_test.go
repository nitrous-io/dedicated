package credittype_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/credittype")
}

var _ = Describe("models/credittype", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	Describe("Insert()", func() {
		It("should able to insert a record", func() {
			c := &credittype.CreditType{
				Name:        "Signup",
				Description: "Desc",
				Active:      true,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			r := &credittype.CreditType{}
			err = db.QueryRowx("SELECT * FROM credit_types WHERE id = $1", c.ID).StructScan(r)
			Expect(err).To(BeNil())
			Expect(c).To(Equal(r))
		})
	})
})
