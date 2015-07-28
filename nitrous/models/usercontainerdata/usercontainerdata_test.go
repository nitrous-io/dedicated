package usercontainerdata_test

import (
	"database/sql"
	"testing"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/usercontainerdata"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/usercontainerdata")
}

var _ = Describe("models/usercontainerdata", func() {
	var (
		u  *user.User
		hs *test.HostStack
		c  *container.Container
		d  *db.Database
	)

	insert := func(d db.QueryReceiver, uc *usercontainerdata.UserContainerData) error {
		nstmt, err := d.PrepareNamed(`INSERT INTO user_container_data
		(user_id, container_id, metadata)
		VALUES (:user_id, :container_id, :metadata) RETURNING *;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRowx(uc).StructScan(uc)
	}

	BeforeEach(func() {
		var err error
		d, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		Expect(test.WrapInTransaction(d, u.Insert)).To(BeNil())

		hs, err = test.CreateHostStack(d, &test.HostStack{
			H: &host.Host{
				Slug: "ok-host",
			},
			U: u,
		})
		Expect(err).To(BeNil())

		c = &container.Container{
			CID:    "deadbeef",
			UserID: u.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "my-1st-container",
		}
		Expect(c.Insert(d)).To(BeNil())
	})

	AfterEach(func() {
		if d != nil {
			d.Close()
		}
	})

	Describe(".Find()", func() {
		It("should find the data by user ID and container ID", func() {
			uc := &usercontainerdata.UserContainerData{
				UserID:      u.ID,
				ContainerID: c.ID,
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"Original": {"Val", true},
					},
				},
			}
			err := insert(d, uc)
			Expect(err).To(BeNil())

			uc2, err := usercontainerdata.Find(d, u.ID, c.ID)
			Expect(err).To(BeNil())
			Expect(uc2).To(Equal(uc))
		})

		It("should return an error if it cannot be found", func() {
			_, err := usercontainerdata.Find(d, u.ID, 1)
			Expect(err).To(Equal(sql.ErrNoRows))
		})
	})

	Describe(".SetData()", func() {
		It("should insert new metadata if the record does not exist", func() {
			Expect(usercontainerdata.SetData(d, u.ID, c.ID, "new", "value")).To(BeNil())

			uc, err := usercontainerdata.Find(d, u.ID, c.ID)
			Expect(err).To(BeNil())
			Expect(uc.Metadata.Map["new"].String).To(Equal("value"))
		})

		It("should update metadata if the record does exist", func() {
			record := &usercontainerdata.UserContainerData{
				UserID:      u.ID,
				ContainerID: c.ID,
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"some": sql.NullString{"value", true},
					},
				},
			}
			Expect(insert(d, record)).To(BeNil())

			Expect(usercontainerdata.SetData(d, u.ID, c.ID, "new", "value")).To(BeNil())

			uc, err := usercontainerdata.Find(d, u.ID, c.ID)
			Expect(err).To(BeNil())
			Expect(uc.Metadata.Map["some"].String).To(Equal("value"))
			Expect(uc.Metadata.Map["new"].String).To(Equal("value"))
		})
	})
})
