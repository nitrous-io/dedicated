package containercollaborator_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollaborator"
	"github.com/nitrous-io/dedicated/nitrous/models/user"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/containercollaborator")
}

var _ = Describe("models/containercollaborator", func() {
	var (
		db  *ndb.Database
		cnr *container.Container
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err := test.CreateHostStack(db, nil)
		Expect(err).To(BeNil())

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
		It("should associate a container collaborator between a user and a container", func() {
			u := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			sc, err := containercollaborator.Create(db, cnr.ID, u.ID)
			Expect(err).To(BeNil())
			Expect(sc).ToNot(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collaborators WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
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

			var scID int64
			err = db.QueryRow(`INSERT INTO container_collaborators (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u.ID).Scan(&scID)
			Expect(err).To(BeNil())

			err = containercollaborator.Delete(db, scID)
			Expect(err).To(BeNil())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collaborators WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeFalse())
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

			_, err = db.Exec(`INSERT INTO container_collaborators (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u1.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collaborators (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u2.ID)
			Expect(err).To(BeNil())

			items, err := containercollaborator.FindAll(db, cnr.ID)
			Expect(err).To(BeNil())
			Expect(items).To(HaveLen(2))
			Expect([]int64{items[0].UserID, items[1].UserID}).To(Equal([]int64{u1.ID, u2.ID}))
			Expect([]string{items[0].Username, items[1].Username}).To(Equal([]string{"foo", "bar"}))
		})
	})
})
