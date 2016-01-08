package containercollab_test

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/containercollab")
}

var _ = Describe("models/containercollab", func() {
	var (
		db  *ndb.Database
		cnr *container.Container
		h   *host.Host
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err := test.CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		h = hs.H

		cnr = &container.Container{
			CID:    "cafebabe",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err = cnr.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Create", func() {
		It("should associate a container collaborator between an existing user and a container", func() {
			u := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			sc, err := containercollab.Create(db, cnr.ID, strconv.FormatInt(u.ID, 10))
			Expect(err).To(BeNil())
			Expect(sc).ToNot(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
		})

		It("should associate a container collaborator between a pending collab and a container", func() {
			sc, err := containercollab.Create(db, cnr.ID, "bob@collab.com")
			Expect(err).To(BeNil())
			Expect(sc).ToNot(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE invite_email = 'bob@collab.com' AND container_id = $1 AND invite_code = $2;`, cnr.ID, sc.InviteCode.String)
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
		})
	})

	Describe("Delete", func() {
		It("should delete a container collaborator", func() {
			u := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			cc, err := containercollab.Create(db, cnr.ID, strconv.FormatInt(u.ID, 10))
			Expect(err).To(BeNil())

			err = containercollab.Delete(db, cc.ID, cnr.ID)
			Expect(err).To(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeFalse())
		})

		It("should not delete a container collaborator if the container ID does not match", func() {
			u := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			cc, err := containercollab.Create(db, cnr.ID, strconv.FormatInt(u.ID, 10))
			Expect(err).To(BeNil())

			err = containercollab.Delete(db, cc.ID, -2)
			Expect(err).To(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
		})
	})

	Describe("FindAll", func() {
		It("should return all container collaborators for a container", func() {
			u1 := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 := &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			u3 := &user.User{
				Username: "dell",
				Email:    "dell@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u3.Insert)
			Expect(err).To(BeNil())

			_, err = containercollab.Create(db, cnr.ID, strconv.FormatInt(u1.ID, 10))
			Expect(err).To(BeNil())

			_, err = containercollab.Create(db, cnr.ID, strconv.FormatInt(u2.ID, 10))
			Expect(err).To(BeNil())

			_, err = containercollab.Create(db, cnr.ID, strconv.FormatInt(u3.ID, 10))
			Expect(err).To(BeNil())

			_, err = containercollab.Create(db, cnr.ID, "abc@example.com")
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u3.ID)
			Expect(err).To(BeNil())

			items, err := containercollab.FindAll(db, cnr.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(3))
			Expect(items[0].UserID.Valid).To(BeFalse())
			Expect(items[0].Name.Valid).To(BeFalse())
			Expect(items[0].Email.Valid).To(BeFalse())
			Expect(items[0].Username.Valid).To(BeFalse())
			Expect(items[0].InviteEmail.String).To(Equal("abc@example.com"))

			Expect([]int64{items[1].UserID.Int64, items[2].UserID.Int64}).To(ConsistOf([]int64{u1.ID, u2.ID}))
			Expect([]string{items[1].Username.String, items[2].Username.String}).To(ConsistOf([]string{"foo", "bar"}))
		})
	})

	Describe("FindByInviteCode()", func() {
		var (
			cc  *containercollab.ContainerCollab
			err error
		)
		BeforeEach(func() {
			cc, err = containercollab.Create(db, cnr.ID, "bob@pendingcollab.com")
			Expect(err).To(BeNil())
		})

		Context("when pending member does not exist", func() {
			It("returns err", func() {
				_, err := containercollab.FindByInviteCode(db, cc.InviteCode.String+"9000")
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when pending member exists", func() {
			It("returns member", func() {
				m, err := containercollab.FindByInviteCode(db, cc.InviteCode.String)
				Expect(err).To(BeNil())

				Expect(m.UserID).To(BeZero())
				Expect(m.InviteEmail.String).To(Equal("bob@pendingcollab.com"))
				Expect(m.ContainerID).To(Equal(cnr.ID))
			})
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				db.Close()
				_, err := containercollab.FindByInviteCode(db, cc.InviteCode.String)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("GetCollaboratorIDs", func() {
		It("returns all collaborator ids for given container id", func() {
			u1 := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 := &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u1.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u2.ID)
			Expect(err).To(BeNil())

			ids, err := containercollab.GetCollaboratorIDs(db, cnr.ID)
			Expect(err).To(BeNil())
			Expect(ids).To(ConsistOf(u1.ID, u2.ID))
		})
	})

	// max_collaborators on plans has a default value of 1. Where org=true, max_collaborators is set to NULL.
	Describe("CanAddCollaborator", func() {
		var (
			u1 *user.User
			u2 *user.User
		)
		BeforeEach(func() {
			u1 = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())
		})

		Context("when max_collaborators set", func() {
			It("does not return error if within limit", func() {
				err := containercollab.CanAddCollaborator(db, cnr.ID)
				Expect(err).To(BeNil())
			})

			It("returns error when collab limit is reached", func() {
				_, err := db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u1.ID)
				Expect(err).To(BeNil())

				err = containercollab.CanAddCollaborator(db, cnr.ID)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(Equal("collaborators limit reached"))
			})
		})

		Context("when max_collaborators not set", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE plans SET max_collaborators=NULL WHERE id=$1;`, h.PlanID)
				Expect(err).To(BeNil())
			})

			It("does not return error", func() {
				_, err := db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u1.ID)
				Expect(err).To(BeNil())

				err = containercollab.CanAddCollaborator(db, cnr.ID)
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("PromotePendingMemberToUser()", func() {
		It("returns error if container collab does not exist", func() {
			_, err := containercollab.PromotePendingMemberToUser(db, "bob@doesnotexist.com", "12345678912345678912", cnr.UserID)
			Expect(err).To(MatchError(containercollab.ErrDoesNotExist))
		})

		It("updates container collabs with user data and removes pending invite data", func() {
			cc, err := containercollab.Create(db, cnr.ID, "bob@collab.com")
			Expect(err).To(BeNil())
			Expect(cc).ToNot(BeNil())

			cc1, err := containercollab.PromotePendingMemberToUser(db, "bob@collab.com", cc.InviteCode.String, cnr.UserID)
			Expect(err).To(BeNil())
			Expect(cc1.UserID.Valid).To(BeTrue())
			Expect(cc1.UserID.Int64).To(Equal(cnr.UserID))
			Expect(cc1.InviteEmail.Valid).To(BeFalse())
			Expect(cc1.InviteCode.Valid).To(BeFalse())
		})

		Context("when some other error occurs", func() {
			It("returns error", func() {
				cc, err := containercollab.Create(db, cnr.ID, "bob@collab.com")
				Expect(err).To(BeNil())

				db.Close()
				_, err = containercollab.PromotePendingMemberToUser(db, "bob@collab.com", cc.InviteCode.String, cnr.UserID)
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
