package invitecode_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invitecode"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/invitecode")
}

var _ = Describe("models/invitecode", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO invite_codes (code, used) values ($1, $2);", "code1", false)
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO invite_codes (code, used) values ($1, $2);", "code2", true)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByCode()", func() {
		It("should find a code", func() {
			found, err := invitecode.FindByCode(db, "code1")
			Expect(err).To(BeNil())
			Expect(found).ToNot(BeNil())
			Expect(found.Used).To(BeFalse())
			Expect(found.Code).To(Equal("code1"))
		})

		It("should find used codes", func() {
			found, err := invitecode.FindByCode(db, "code2")
			Expect(err).To(BeNil())
			Expect(found).ToNot(BeNil())
			Expect(found.Used).To(BeTrue())
			Expect(found.Code).To(Equal("code2"))
		})

		It("should return error when there is no code matching", func() {
			found, err := invitecode.FindByCode(db, "codey1")
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(found).To(BeNil())
		})
	})

	Describe("MarkAsUsed", func() {
		It("should allow for a code to be marked as used", func() {
			err := invitecode.MarkAsUsed(db, "code1")
			Expect(err).To(BeNil())
			found, err := invitecode.FindByCode(db, "code1")
			Expect(err).To(BeNil())
			Expect(found.Used).To(BeTrue())
		})

		It("should allow a used code to be marked as used", func() {
			err := invitecode.MarkAsUsed(db, "code2")
			Expect(err).To(BeNil())
			found, err := invitecode.FindByCode(db, "code2")
			Expect(err).To(BeNil())
			Expect(found.Used).To(BeTrue())
		})

		It("should be silent on failure", func() {
			err := invitecode.MarkAsUsed(db, "codey2")
			Expect(err).To(BeNil())
		})
	})

	Describe("GenerateInvite()", func() {
		It("should return the InviteCode", func() {
			code, err := invitecode.GenerateInvite(db)
			Expect(err).To(BeNil())
			Expect(code).ToNot(BeNil())
			Expect(code.Used).To(BeFalse())
			Expect(code.Code).To(HaveLen(32))
		})
	})
})
