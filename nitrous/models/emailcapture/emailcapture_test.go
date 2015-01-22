package emailcapture_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/models/emailcapture"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/emailcapture")
}

var _ = Describe("models/emailcapture", func() {
	Describe(".Insert", func() {
		It("creates the emailcapture", func() {
			db, err := TestDatabase("nitrous_test")
			Expect(err).To(BeNil())

			e := &emailcapture.EmailCapture{
				Email: "me@callumj.com",
			}

			Expect(e.Insert(db)).To(BeNil())

			fE := &emailcapture.EmailCapture{}
			err = db.Get(fE, `SELECT * FROM email_captures WHERE id = $1;`, e.ID)
			Expect(err).To(BeNil())
			Expect(fE).To(Equal(e))
		})
	})
})
