package httperror

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "httperror")
}

var _ = Describe("httperror", func() {
	Describe("New", func() {
		It("accepts types, description and hash", func() {
			testCases := [][]string{
				{"type"},
				{"type", "description"},
				{"type", "description", "hash"},
			}
			for _, tc := range testCases {
				err := New(404, tc...).(*Error)
				for i, arg := range tc {
					switch i {
					case 0:
						Expect(err.Type).To(Equal(arg), "Invalid Type")
					case 1:
						Expect(err.Description).To(Equal(arg), "Invalid Description")
					case 2:
						Expect(err.Hash).To(Equal(arg), "Invalid Hash")
					}
				}
			}
		})
	})
})
