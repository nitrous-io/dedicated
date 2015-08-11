package cidrblacklist_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/pkg/cidrblacklist"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/cidrblacklist")
}

var _ = Describe("CIDRBlacklisted", func() {
	It("returns true if ip is in blacklist", func() {
		// these are actual ips used by banned users
		Expect(cidrblacklist.Contains("95.7.70.144")).To(MatchError("blocked by CIDR 95.0.0.0/12"))
	})

	It("ignores commented out cidrs", func() {
		// assumes the line "# 8.8.4.0/24" is present in the test blacklist file
		Expect(cidrblacklist.Contains("8.8.4.4")).To(BeNil())
	})

	It("returns false if ip is not in blacklist", func() {
		Expect(cidrblacklist.Contains("8.8.8.8")).To(BeNil())
	})
})
