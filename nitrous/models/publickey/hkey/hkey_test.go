package hkey_test

import (
	"database/sql"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey/hkey"
	"github.com/nitrous-io/dedicated/nitrous/models/user"

	"github.com/nitrous-io/nitrous-common/sshkey"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestHostKey(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/publickey/hkey")
}

var _ = Describe("models/publickey/hkey", func() {
	var (
		db      *db.Database
		hs, hs2 *HostStack
	)

	validRSA := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6YIJRY7GTMUOHjoRDP4IXa5LdmKVYpW0ORccuRztsbBVcZhkm2GVrvOPwnUc53k0zUe+q95im+Yb3d5pATX27KG0VheDx7mz2l2xUb44VzdT6aMpYEsQREw5RrUnpzFQgPh295ZbVPTIGzq0+7LALlDMP5YpY8oYrY1gAtTEmivcffD/fQMmcExlRbWRBvDc/nfSoN5wSpG+sW32gQ/l5IWtmPlnhb4ze/D32YDL6OVK75tlFNeMk2pmPWejnw9GFOBXkXEpYVfmd5AHpJqc1WskOdv1ahXwM2n8pghkjw3eh9YDmpiLkxeJEpA+pNJA69NdF4fYJhpm2Nz2dIvwn"
	validECDSA := "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAEJfzWE4kBhqfyJV5PGUhCyaJ9GB0RRrc3RsCweBLgLmbagOy4IwRtd79/AgxlmoZ/7e8eTNfAhE4vV6MXJoltIkAFz+0+NsAopvQEWWDHfADilCuTltegQn6r1f6iA5XgSoLJQvnJVaneo4UzClCHIVDZWQL1XvNlKKtxRHU0q0niElA=="

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		hs2, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAll()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO public_keys (user_id, host_id, key, fingerprint) VALUES ($1, $2, $3, $4);", hs.U.ID, hs.H.ID, validRSA, "fingerprint")
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO public_keys (user_id, host_id, key, fingerprint) VALUES ($1, $2, $3, $4);", hs.U.ID, hs2.H.ID, validECDSA, "fingerprint")
			Expect(err).To(BeNil())
		})

		Context("when public_key records with a given user_id exists", func() {
			It("finds public_keys by a given user", func() {
				keys, err := hkey.FindAll(db, hs.U.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))

				Expect(keys[0].UserID).To(Equal(hs.U.ID))
				Expect(keys[0].Key).To(Equal(validRSA))
			})
		})

		Context("when no public_key records with a given user_id and host_id exist", func() {
			It("returns empty slice", func() {
				keys, err := hkey.FindAll(db, hs.U.ID, 999)
				Expect(err).To(BeNil())
				Expect(keys).To(BeEmpty())

				keys, err = hkey.FindAll(db, 999, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(BeEmpty())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u := user.User{}
				keys, err := hkey.FindAll(db, u.ID, hs.H.ID)
				Expect(keys).To(BeEmpty())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert", func() {
		var pk *publickey.PublicKey

		BeforeEach(func() {
			pk = &publickey.PublicKey{
				UserID: hs.U.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    validRSA,
			}
		})

		It("returns error if host id is not set", func() {
			pk.HostID.Valid = false
			err := hkey.Insert(db, pk)
			Expect(err).To(MatchError(publickey.ErrInvalidKey))

			pk.HostID.Valid = true
			pk.HostID.Int64 = 0
			err = hkey.Insert(db, pk)
			Expect(err).To(MatchError(publickey.ErrInvalidKey))
		})

		It("returns error if container id is set", func() {
			pk.ContainerID = sql.NullInt64{3, true}
			err := hkey.Insert(db, pk)
			Expect(err).To(MatchError(hkey.ErrWrongKeyType))
		})

		It("inserts into db given valid fields", func() {
			err := hkey.Insert(db, pk)
			Expect(err).To(BeNil())

			keys, err := hkey.FindAll(db, hs.U.ID, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(keys).To(HaveLen(1))
			Expect(keys[0].UserID).To(Equal(hs.U.ID))
			Expect(keys[0].HostID.Int64).To(Equal(hs.H.ID))
			Expect(keys[0].ID).NotTo(BeZero())
		})
	})

	Describe("Find", func() {
		var (
			u, u2 *user.User
			// rsa1 and rsa2 belong to u, ecdsa belongs to u2
			rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa              *publickey.PublicKey
			rsa1C1print, rsa2C1print, rsa1C2print, rsa2C2print string
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Password: "123456",
				Email:    "bar@example.com",
			}
			err = WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			rsa1C1 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    validRSA,
			}
			rsa1C1print, err = sshkey.Md5Fingerprint(rsa1C1.Key)
			Expect(err).To(BeNil())

			rsa2C1 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			rsa2C1print, err = sshkey.Md5Fingerprint(rsa2C1.Key)
			Expect(err).To(BeNil())

			rsa1C2 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs2.H.ID, true},
				Key:    validRSA,
			}
			rsa1C2print, err = sshkey.Md5Fingerprint(rsa1C2.Key)
			Expect(err).To(BeNil())

			rsa2C2 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs2.H.ID, true},
				Key:    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			rsa2C2print, err = sshkey.Md5Fingerprint(rsa2C2.Key)
			Expect(err).To(BeNil())

			ecdsa = &publickey.PublicKey{
				UserID: u2.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    validECDSA,
			}
			for _, k := range []*publickey.PublicKey{rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa} {
				err := hkey.Insert(db, k)
				Expect(err).To(BeNil())
			}
		})

		Context("when finding with ID", func() {
			It("should return the right publickey object if found", func() {
				pk, err := hkey.Find(db, u.ID, hs.H.ID, strconv.FormatInt(rsa1C1.ID, 10))
				Expect(err).To(BeNil())
				Expect(pk).To(Equal(rsa1C1))
			})
		})

		Context("when finding with fingerprint", func() {
			It("should return the right publickey record if found", func() {
				pk, err := hkey.Find(db, u.ID, hs.H.ID, rsa1C1print)
				Expect(err).To(BeNil())
				Expect(pk).To(Equal(rsa1C1))
			})
		})

		It("should return nil if it can't find a key with the given identifier", func() {
			pk, err := hkey.Find(db, u.ID, hs.H.ID, "invalid fingerprint")
			Expect(err).To(Equal(publickey.ErrNotFound))
			Expect(pk).To(BeNil())
		})
	})

	Describe("Destroy methods", func() {
		var (
			u, u2 *user.User
			// rsa1 and rsa2 belong to u, ecdsa belongs to u2
			rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa *publickey.PublicKey
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Password: "123456",
				Email:    "bar@example.com",
			}
			err = WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			rsa1C1 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    validRSA,
			}
			rsa2C1 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			rsa1C2 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs2.H.ID, true},
				Key:    validRSA,
			}
			rsa2C2 = &publickey.PublicKey{
				UserID: u.ID,
				HostID: sql.NullInt64{hs2.H.ID, true},
				Key:    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			ecdsa = &publickey.PublicKey{
				UserID: u2.ID,
				HostID: sql.NullInt64{hs.H.ID, true},
				Key:    validECDSA,
			}
			for _, k := range []*publickey.PublicKey{rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa} {
				err := hkey.Insert(db, k)
				Expect(err).To(BeNil())
			}
		})

		Describe("DestroyAll", func() {
			It("removes a user's and keys from a given container", func() {
				pks, err := hkey.FindAll(db, u.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))

				err = hkey.DestroyAll(db, u.ID, hs.H.ID)
				Expect(err).To(BeNil())

				pks, err = hkey.FindAll(db, u.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(0))
			})

			It("does not remove other users' or containers' keys", func() {
				pks, err := hkey.FindAll(db, u.ID, hs2.H.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))

				err = hkey.DestroyAll(db, u.ID, hs.H.ID)
				Expect(err).To(BeNil())

				pks, err = hkey.FindAll(db, u.ID, hs2.H.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))
			})
		})

		Describe("Destroy", func() {
			It("removes a key with the right fingerprint", func() {
				err := hkey.Destroy(db, u.ID, hs.H.ID, rsa1C1.ID)
				Expect(err).To(BeNil())

				pks, err := hkey.FindAll(db, u.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(1))
				Expect(pks[0].Key).To(Equal(rsa2C1.Key))
			})
		})
	})
})
