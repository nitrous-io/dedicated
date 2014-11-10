package publickey_test

import (
	"database/sql"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey/ckey"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func TestContainerKey(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/publickey")
}

var _ = Describe("models/publickey", func() {
	var (
		db    *db.Database
		hs    *HostStack
		c, c2 *container.Container
	)

	validRSA := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6YIJRY7GTMUOHjoRDP4IXa5LdmKVYpW0ORccuRztsbBVcZhkm2GVrvOPwnUc53k0zUe+q95im+Yb3d5pATX27KG0VheDx7mz2l2xUb44VzdT6aMpYEsQREw5RrUnpzFQgPh295ZbVPTIGzq0+7LALlDMP5YpY8oYrY1gAtTEmivcffD/fQMmcExlRbWRBvDc/nfSoN5wSpG+sW32gQ/l5IWtmPlnhb4ze/D32YDL6OVK75tlFNeMk2pmPWejnw9GFOBXkXEpYVfmd5AHpJqc1WskOdv1ahXwM2n8pghkjw3eh9YDmpiLkxeJEpA+pNJA69NdF4fYJhpm2Nz2dIvwn"

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		c = &container.Container{
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())

		c2 = &container.Container{
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container-2",
		}
		err = c2.Insert(db)
		Expect(err).To(BeNil())
	})

	Describe("RawInsert()", func() {
		var pk *publickey.PublicKey

		BeforeEach(func() {
			pk = &publickey.PublicKey{
				UserID:      hs.U.ID,
				ContainerID: sql.NullInt64{c.ID, true},
				Key:         validRSA,
			}
		})

		Context("when fields are valid", func() {
			It("creates a container public key", func() {
				err := pk.RawInsert(db)
				Expect(err).To(BeNil())

				keys, err := ckey.FindAll(db, hs.U.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))
				Expect(keys[0].UserID).To(Equal(hs.U.ID))
				Expect(keys[0].ContainerID.Int64).To(Equal(c.ID))
				Expect(keys[0].ID).NotTo(BeZero())
			})
		})

		Context("when key field is invalid", func() {
			It("returns error", func() {
				pk.Key = "ssh-rsa Not a key!!!"
				err := pk.RawInsert(db)
				Expect(err).To(Equal(publickey.ErrInvalidKey))
			})
		})
	})
})
