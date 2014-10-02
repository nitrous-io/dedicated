package bcrypt_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/bcrypt")
}

var _ = Describe("pkg/bcrypt", func() {
	var (
		hashed  string
		hashed2 string
		hashed3 string
		err     error
	)

	BeforeEach(func() {
		hashed, err = bcrypt.Hash("foobarbaz")
		Expect(err).To(BeNil())
		hashed2, err = bcrypt.Hash("foobarbaz")
		Expect(err).To(BeNil())
		hashed3, err = bcrypt.Hash("barbarbar")
		Expect(err).To(BeNil())
	})

	Describe("Hash()", func() {
		It("encrypts a given password", func() {
			Expect(hashed).NotTo(Equal("foobarbaz"))
			Expect(hashed2).NotTo(Equal("foobarbaz"))
			Expect(hashed3).NotTo(Equal("barbarbar"))
			Expect(hashed).NotTo(Equal(hashed2))
			Expect(hashed).NotTo(Equal(hashed3))
			Expect(hashed2).NotTo(Equal(hashed3))
		})
	})

	Describe("Verify()", func() {
		It("verifies a given password by hash", func() {
			Expect(bcrypt.Verify(hashed, "foobarbaz")).To(BeTrue())
			Expect(bcrypt.Verify(hashed2, "foobarbaz")).To(BeTrue())
			Expect(bcrypt.Verify(hashed3, "barbarbar")).To(BeTrue())
			Expect(bcrypt.Verify(hashed, "foobarba1")).To(BeFalse())
			Expect(bcrypt.Verify(hashed2, "foobarbar")).To(BeFalse())
			Expect(bcrypt.Verify(hashed3, "borborbor")).To(BeFalse())
		})
	})
})
