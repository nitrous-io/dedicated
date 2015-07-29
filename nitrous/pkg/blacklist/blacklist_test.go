package blacklist_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nitrous-io/dedicated/nitrous/pkg/blacklist"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/blacklist")
}

var _ = Describe("pkg/blacklist", func() {
	Describe("Check", func() {
		It("returns true if email domain is in blacklist", func() {
			Expect(blacklist.Check("hello@yopmail.com")).To(BeTrue())
			Expect(blacklist.Check("hello@dispo.in")).To(BeTrue())
		})

		It("returns false if email domain is not in blacklist", func() {
			Expect(blacklist.Check("hello@gmail.com")).To(BeFalse())
			Expect(blacklist.Check("hello@action.io")).To(BeFalse())
			Expect(blacklist.Check("hello@zumpul.com")).To(BeFalse())
		})

		It("returns true if email does not contain @", func() {
			Expect(blacklist.Check("gmail.com")).To(BeTrue())
		})
	})
})
