package keypairs

import (
	"testing"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/keypairs")
}

var _ = Describe("models/keypairs", func() {
	It("creates and fetches a keypair for the specified owner", func() {
		db, _ := TestDatabase("nitrous_test")

		// prepare the host
		hs, _ := CreateHostStack(db, nil)
		priv, _ := GenerateKey()

		kp1, err := Create(db, priv, "aes_key", hs.H.ID)
		Expect(err).To(BeNil(), "should be able to create new keypair")

		kp2, err := Get(db, hs.H.ID)
		Expect(err).To(BeNil(), "should be able to get an existing keypair")

		Expect(kp1).To(Equal(kp2), "should be the same keypair")
	})

	It("fails to get non-existing keypair", func() {
		db, _ := TestDatabase("nitrous_test")
		_, err := Get(db, 1)
		Expect(err).NotTo(BeNil(), "should fail to find invalid keypair")
	})

	It("deletes the keypair when its host is deleted", func() {
		db, _ := TestDatabase("nitrous_test")

		// prepare the host
		hs, _ := CreateHostStack(db, nil)
		priv, _ := GenerateKey()

		kp, _ := Create(db, priv, "aes_key", hs.H.ID)
		Expect(kp.HostID).To(Equal(hs.H.ID))

		err := hs.H.Delete(db)
		Expect(err).To(BeNil(), "should succesfully delete the host")

		_, err = Get(db, hs.H.ID)
		Expect(err).ToNot(BeNil(), "should delete the host's keypairs")
	})
})
