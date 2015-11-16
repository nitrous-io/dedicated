package userkey_test

import (
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userkey"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestUserKey(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/userkey")
}

var _ = Describe("models/userkey", func() {
	var (
		db *db.Database
		hs *HostStack
		c  *container.Container
		c2 *container.Container
	)

	validRSA := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6YIJRY7GTMUOHjoRDP4IXa5LdmKVYpW0ORccuRztsbBVcZhkm2GVrvOPwnUc53k0zUe+q95im+Yb3d5pATX27KG0VheDx7mz2l2xUb44VzdT6aMpYEsQREw5RrUnpzFQgPh295ZbVPTIGzq0+7LALlDMP5YpY8oYrY1gAtTEmivcffD/fQMmcExlRbWRBvDc/nfSoN5wSpG+sW32gQ/l5IWtmPlnhb4ze/D32YDL6OVK75tlFNeMk2pmPWejnw9GFOBXkXEpYVfmd5AHpJqc1WskOdv1ahXwM2n8pghkjw3eh9YDmpiLkxeJEpA+pNJA69NdF4fYJhpm2Nz2dIvwn"
	validECDSA := "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAEJfzWE4kBhqfyJV5PGUhCyaJ9GB0RRrc3RsCweBLgLmbagOy4IwRtd79/AgxlmoZ/7e8eTNfAhE4vV6MXJoltIkAFz+0+NsAopvQEWWDHfADilCuTltegQn6r1f6iA5XgSoLJQvnJVaneo4UzClCHIVDZWQL1XvNlKKtxRHU0q0niElA=="

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

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAll()", func() {
		BeforeEach(func() {
			_, err := db.Exec("INSERT INTO public_keys (user_id, container_id, key, fingerprint) VALUES ($1, $2, $3, $4);", hs.U.ID, c.ID, validRSA, "fingerprint")
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO public_keys (user_id, container_id, key, fingerprint) VALUES ($1, $2, $3, $4);", hs.U.ID, c2.ID, validECDSA, "fingerprint")
			Expect(err).To(BeNil())
		})

		Context("when public_key records with a given user_id exists", func() {
			It("finds public_keys by a given user", func() {
				keys, err := userkey.FindAll(db, hs.U.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))

				Expect(keys[0].UserID).To(Equal(hs.U.ID))
				Expect(keys[0].Key).To(Equal(validRSA))
			})
		})

		Context("when no public_key records with a given user_id and container_id exist", func() {
			It("returns empty slice", func() {
				keys, err := userkey.FindAll(db, hs.U.ID, 999)
				Expect(err).To(BeNil())
				Expect(keys).To(BeEmpty())

				keys, err = userkey.FindAll(db, 999, c.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(BeEmpty())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u := user.User{}
				c := container.Container{}
				keys, err := userkey.FindAll(db, u.ID, c.ID)
				Expect(keys).To(BeEmpty())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var pk *userkey.UserKey

		BeforeEach(func() {
			pk = &userkey.UserKey{
				UserID:      hs.U.ID,
				ContainerID: c.ID,
				Key:         validRSA,
			}
		})

		Context("when given params are valid", func() {
			It("creates a public key", func() {
				err := pk.Insert(db)
				Expect(err).To(BeNil())

				keys, err := userkey.FindAll(db, hs.U.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))
				Expect(keys[0].UserID).To(Equal(hs.U.ID))
				Expect(keys[0].ContainerID).To(Equal(c.ID))
				Expect(keys[0].ID).NotTo(BeZero())
			})
		})

		Context("when given key is invalid", func() {
			It("returns error", func() {
				pk.Key = "ssh-rsa Not a key!!!"
				err := pk.Insert(db)
				Expect(err).To(Equal(userkey.ErrInvalidKey))
			})
		})

		Context("when insert same key", func() {
			It("does not duplicate", func() {
				err := pk.Insert(db)
				Expect(err).To(BeNil())

				pk2 := &userkey.UserKey{
					UserID:      hs.U.ID,
					ContainerID: c.ID,
					Key:         validRSA,
				}

				err = pk2.Insert(db)
				Expect(err).To(BeNil())

				keys, err := userkey.FindAll(db, hs.U.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))
			})
		})
	})

	Describe("Find", func() {
		var (
			u, u2 *user.User
			// rsa1 and rsa2 belong to u, ecdsa belongs to u2
			rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa              *userkey.UserKey
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

			rsa1C1 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c.ID,
				Key:         validRSA,
			}
			rsa1C1print, err = rsa1C1.Md5Fingerprint()
			Expect(err).To(BeNil())

			rsa2C1 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c.ID,
				Key:         "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			rsa2C1print, err = rsa2C1.Md5Fingerprint()
			Expect(err).To(BeNil())

			rsa1C2 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c2.ID,
				Key:         validRSA,
			}
			rsa1C2print, err = rsa1C2.Md5Fingerprint()
			Expect(err).To(BeNil())

			rsa2C2 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c2.ID,
				Key:         "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			rsa2C2print, err = rsa2C2.Md5Fingerprint()
			Expect(err).To(BeNil())

			ecdsa = &userkey.UserKey{
				UserID:      u2.ID,
				ContainerID: c.ID,
				Key:         validECDSA,
			}
			for _, k := range []*userkey.UserKey{rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa} {
				err := k.Insert(db)
				Expect(err).To(BeNil())
			}
		})

		Context("when finding with ID", func() {
			It("should return the right publickey object if found", func() {
				pk, err := userkey.Find(db, u.ID, c.ID, strconv.FormatInt(rsa1C1.ID, 10))
				Expect(err).To(BeNil())
				Expect(pk).To(Equal(rsa1C1))
			})
		})

		Context("when finding with fingerprint", func() {
			It("should return the right publickey record if found", func() {
				pk, err := userkey.Find(db, u.ID, c.ID, rsa1C1print)
				Expect(err).To(BeNil())
				Expect(pk).To(Equal(rsa1C1))
			})
		})

		It("should return nil if it can't find a key with the given identifier", func() {
			pk, err := userkey.Find(db, u.ID, c.ID, "invalid fingerprint")
			Expect(err).To(Equal(userkey.ErrNotFound))
			Expect(pk).To(BeNil())
		})
	})

	Describe("Destroy methods", func() {
		var (
			u, u2 *user.User
			// rsa1 and rsa2 belong to u, ecdsa belongs to u2
			rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa *userkey.UserKey
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

			rsa1C1 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c.ID,
				Key:         validRSA,
			}
			rsa2C1 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c.ID,
				Key:         "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			rsa1C2 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c2.ID,
				Key:         validRSA,
			}
			rsa2C2 = &userkey.UserKey{
				UserID:      u.ID,
				ContainerID: c2.ID,
				Key:         "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			ecdsa = &userkey.UserKey{
				UserID:      u2.ID,
				ContainerID: c.ID,
				Key:         validECDSA,
			}
			for _, k := range []*userkey.UserKey{rsa1C1, rsa2C1, rsa1C2, rsa2C2, ecdsa} {
				err := k.Insert(db)
				Expect(err).To(BeNil())
			}
		})

		Describe("DestroyAll", func() {
			It("removes a user's and keys from a given container", func() {
				pks, err := userkey.FindAll(db, u.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))

				err = userkey.DestroyAll(db, u.ID, c.ID)
				Expect(err).To(BeNil())

				pks, err = userkey.FindAll(db, u.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(0))
			})

			It("does not remove other users' or containers' keys", func() {
				pks, err := userkey.FindAll(db, u.ID, c2.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))

				err = userkey.DestroyAll(db, u.ID, c.ID)
				Expect(err).To(BeNil())

				pks, err = userkey.FindAll(db, u.ID, c2.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))
			})
		})

		Describe("Destroy", func() {
			It("removes a key with the right fingerprint", func() {
				err := userkey.Destroy(db, u.ID, c.ID, rsa1C1.ID)
				Expect(err).To(BeNil())

				pks, err := userkey.FindAll(db, u.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(1))
				Expect(pks[0].Key).To(Equal(rsa2C1.Key))
			})
		})
	})

	Describe("Validate()", func() {
		var pk *userkey.UserKey

		BeforeEach(func() {
			u := &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			pk = &userkey.UserKey{
				UserID: u.ID,
				Key:    validRSA,
			}
			Expect(pk.Validate()).To(BeNil())
		})

		It("validates key", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", userkey.ErrKeyBlank},
			}
			for _, t := range tests {
				pk.Key = t.input
				Expect(pk.Validate()).To(Equal(t.err))
			}
		})
	})
})
