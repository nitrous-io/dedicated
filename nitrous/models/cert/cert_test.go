package cert_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func mkcert() *cert.Cert {
	return &cert.Cert{
		ServerCert: "servercert",
		ServerKey:  "serverkey",
	}
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/cert")
}

var _ = Describe("models/cert", func() {
	var db *db.Database

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
	})

	Describe(".Insert", func() {
		It("creates certificates for a host", func() {
			cert1 := mkcert()

			tx, _ := db.Beginx()
			err := cert1.Insert(tx, 1)
			Expect(err).To(BeNil(), "failed to create new cert")
			tx.Commit()

			cert2, err := cert.Find(db, 1)
			Expect(err).To(BeNil(), "failed to find an existing cert")

			Expect(cert2).To(Equal(cert1), "created cert is not the same as stored cert")
		})
	})

	Describe(".Find", func() {
		It("fails to get non-existing cert", func() {
			_, err := cert.Find(db, 1)
			Expect(err).NotTo(BeNil(), "should fail to find invalid cert")
		})
	})

	Describe(".FindAllByHostIDs", func() {
		It("should return an empty set when the hostIDs are empty", func() {
			var hostIDs []int64
			certs, err := cert.FindAllByHostIDs(db, hostIDs)
			Expect(err).To(BeNil())
			Expect(certs).To(HaveLen(0))
		})

		It("fails to get non-existing cert", func() {
			cert1 := mkcert()
			cert2 := mkcert()

			tx, _ := db.Beginx()
			err := cert1.Insert(tx, 1)
			Expect(err).To(BeNil())
			err = cert2.Insert(tx, 2)
			Expect(err).To(BeNil())
			tx.Commit()

			hostIDs := []int64{1, 2}
			certs, err := cert.FindAllByHostIDs(db, hostIDs)
			Expect(err).To(BeNil())
			Expect(cert1).To(Equal(certs[0]))
			Expect(cert2).To(Equal(certs[1]))
		})
	})

	Describe(".Delete", func() {
		It("removes the host certificates", func() {
			db, _ := TestDatabase("nitrous_test")
			crt := mkcert()

			tx, _ := db.Beginx()
			crt.Insert(tx, 1)
			tx.Commit()

			tx, _ = db.Beginx()
			err := cert.Delete(tx, 1)
			Expect(err).To(BeNil(), "failed to delete the host's certs")
			tx.Commit()

			_, err = cert.Find(db, 1)
			Expect(err).ToNot(BeNil(), "should not find deleted certs")

			var terminated *time.Time
			err = db.QueryRow("SELECT terminated_at FROM certs WHERE id = $1", crt.ID).
				Scan(&terminated)
			Expect(err).To(BeNil())
			Expect(terminated).ToNot(BeNil(), "failed to update terminated time")
		})
	})
})
