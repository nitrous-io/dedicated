package freighterclient

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("freighterclient/host", func() {
	Describe("NewHost()", func() {
		It("sets port to 65343 by default", func() {
			h := NewHost("127.0.0.1", 0, "slug.nitrouspro.com", nil, nil, nil)
			Expect(h.Port).To(Equal(65343))
		})
	})

	Describe("BaseURL()", func() {
		It("returns a https URL based on its IP, port", func() {
			h := &Host{
				Host: "127.0.0.1",
				Port: 65333,
			}
			Expect(h.BaseURL()).To(Equal("https://127.0.0.1:65333"))
		})
	})
})
