package providertest

import (
	"github.com/nitrous-io/freighter/provider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider", func() {
	Describe("IsValidName()", func() {
		Context("when an invalid name is given", func() {
			It("returns false", func() {
				Expect(provider.IsValidName("")).To(BeFalse())           // empty
				Expect(provider.IsValidName(" ")).To(BeFalse())          // empty
				Expect(provider.IsValidName("f")).To(BeFalse())          // too short
				Expect(provider.IsValidName("foo bar")).To(BeFalse())    // spaces
				Expect(provider.IsValidName("foo\nbar")).To(BeFalse())   // endline
				Expect(provider.IsValidName("foo_bar")).To(BeFalse())    // underscore
				Expect(provider.IsValidName("123foo")).To(BeFalse())     // begins with a digit
				Expect(provider.IsValidName("-foobar")).To(BeFalse())    // begins with a hyphen
				Expect(provider.IsValidName("foobar-")).To(BeFalse())    // ends with a hyphen
				Expect(provider.IsValidName("foo!@$%bar")).To(BeFalse()) // invalid special characters
				Expect(provider.IsValidName("Foobar")).To(BeFalse())     // uppercase
				Expect(provider.IsValidName("유니코드")).To(BeFalse())       // no unicode
			})
		})

		Context("when a valid name is given", func() {
			It("returns true", func() {
				Expect(provider.IsValidName("fo")).To(BeTrue())
				Expect(provider.IsValidName("foobar")).To(BeTrue())
				Expect(provider.IsValidName("foo123")).To(BeTrue())
				Expect(provider.IsValidName("foo-bar")).To(BeTrue())
			})
		})
	})
})
