package user_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

var _ = Describe("model/user email blacklist", func() {
	Describe("IsBlacklisted", func() {
		It("returns true if email domain is in blacklist", func() {
			Expect(user.IsBlacklisted("hello@yopmail.com")).To(BeTrue())
			Expect(user.IsBlacklisted("hello@dispo.in")).To(BeTrue())
		})

		It("returns false if email domain is not in blacklist", func() {
			Expect(user.IsBlacklisted("hello@gmail.com")).To(BeFalse())
			Expect(user.IsBlacklisted("hello@action.io")).To(BeFalse())
			Expect(user.IsBlacklisted("hello@zumpul.com")).To(BeFalse())
		})

		It("returns true if email does not contain @", func() {
			Expect(user.IsBlacklisted("gmail.com")).To(BeTrue())
		})
	})
})
