package cert

import (
	"testing"
	"time"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func mkcert() *Cert {
	return &Cert{
		CA:         "ca",
		ServerCert: "servercert",
		ServerKey:  "serverkey",
		ClientCert: "clientcert",
		ClientKey:  "clientkey",
	}
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/cert")
}

var _ = Describe("models/cert", func() {
	Describe(".Insert", func() {
		It("creates certificates for a host", func() {
			db, _ := TestDatabase("nitrous_test")
			cert := mkcert()

			tx, _ := db.Beginx()
			err := cert.Insert(tx, 1)
			Expect(err).To(BeNil(), "should be able to create new cert")
			tx.Commit()

			cert2, err := Get(db, 1)
			Expect(err).To(BeNil(), "should be able to get an existing cert")

			Expect(cert2).To(Equal(cert), "should be the same cert")
		})
	})

	Describe(".Get", func() {
		It("fails to get non-existing cert", func() {
			db, _ := TestDatabase("nitrous_test")
			_, err := Get(db, 1)
			Expect(err).NotTo(BeNil(), "should fail to find invalid cert")
		})
	})

	Describe(".Delete", func() {
		It("removes the host certificates", func() {
			db, _ := TestDatabase("nitrous_test")
			cert := mkcert()

			tx, _ := db.Beginx()
			cert.Insert(tx, 1)
			tx.Commit()

			tx, _ = db.Beginx()
			err := Delete(tx, 1)
			Expect(err).To(BeNil(), "should delete the host's certs")
			tx.Commit()

			_, err = Get(db, 1)
			Expect(err).ToNot(BeNil(), "should not find deleted certs")

			var terminated time.Time
			db.QueryRow("SELECT terminated_at FROM certs WHERE id = ?", cert.ID).
				Scan(&terminated)
			Expect(terminated).ToNot(BeNil(), "should update terminated time")
		})
	})
})
