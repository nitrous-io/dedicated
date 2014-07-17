package keypairs

import (
	"testing"
	"time"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/keypairs")
}

var _ = Describe("models/keypairs", func() {
	Describe(".Create", func() {
		It("creates a keypair for the specified host", func() {
			db, _ := TestDatabase("nitrous_test")
			priv, _ := GenerateKey()
			kp1, err := Create(db, priv, "aes_key", 1)
			Expect(err).To(BeNil(), "should be able to create new keypair")

			kp2, err := Get(db, 1)
			Expect(err).To(BeNil(), "should be able to get an existing keypair")

			Expect(kp1).To(Equal(kp2), "should be the same keypair")
		})
	})

	Describe(".Get", func() {
		It("fails to get non-existing keypair", func() {
			db, _ := TestDatabase("nitrous_test")
			_, err := Get(db, 1)
			Expect(err).NotTo(BeNil(), "should fail to find invalid keypair")
		})
	})

	Describe(".Delete", func() {
		It("deletes the keypair when its host is deleted", func() {
			db, _ := TestDatabase("nitrous_test")
			priv, _ := GenerateKey()
			kp, _ := Create(db, priv, "aes_key", 1)

			err := Delete(db, 1)
			Expect(err).To(BeNil(), "should delete the host's keypairs")

			_, err = Get(db, 1)
			Expect(err).ToNot(BeNil(), "should not find deleted keypairs")

			var terminated time.Time
			db.QueryRow("SELECT terminated_at, created_at FROM key_pairs WHERE id = ?", kp.ID).
				Scan(&terminated)
			Expect(terminated).ToNot(BeNil(), "should update terminated time")
		})
	})
})
