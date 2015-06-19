package billingprofile_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

var _ = Describe("CIDRBlacklisted", func() {
	It("returns true if ip is in blacklist", func() {
		// these are actual ips used by banned users
		Expect(billingprofile.CIDRBlacklisted("95.7.70.144")).To(MatchError("blocked by CIDR 95.0.0.0/12"))
	})

	It("ignores commented out cidrs", func() {
		// assumes the line "# 8.8.4.0/24" is present in the test blacklist file
		Expect(billingprofile.CIDRBlacklisted("8.8.4.4")).To(BeNil())
	})

	It("returns false if ip is not in blacklist", func() {
		Expect(billingprofile.CIDRBlacklisted("8.8.8.8")).To(BeNil())
	})
})
