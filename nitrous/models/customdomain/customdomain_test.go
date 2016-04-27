package customdomain_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/customdomain"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/customdomain")
}

var _ = Describe("models/customdomain", func() {
	var (
		db *db.Database
		c  *container.Container
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err := test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:          "foo",
				IPAddress:     sql.NullString{"127.0.0.1", true},
				MaxContainers: sql.NullInt64{1, true},
			},
		})
		Expect(err).To(BeNil())

		c = &container.Container{
			CID:    "container-id-id-id",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "rails-test-container",
		}
		Expect(c.Insert(db)).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Insert()", func() {
		It("persists the custom domains in the database", func() {
			cd1 := &customdomain.CustomDomain{
				ContainerID:   c.ID,
				Domain:        "www.gomama.com",
				Port:          80,
				ContainerPort: 4000,
			}
			Expect(cd1.Insert(db)).To(BeNil())

			cd2 := &customdomain.CustomDomain{}
			Expect(db.Get(cd2, `SELECT * FROM custom_domains LIMIT 1`)).To(BeNil())
			Expect(cd2).NotTo(BeNil())
			Expect(cd2.ContainerID).To(Equal(c.ID))
			Expect(cd2.Port).To(Equal(int64(80)))
			Expect(cd2.ContainerPort).To(Equal(int64(4000)))
			Expect(cd2.CreatedAt).NotTo(BeNil())
			Expect(cd2.UpdatedAt).NotTo(BeNil())
		})

		It("returns error when some error occurs", func() {
			// Container ID not exist
			cd := &customdomain.CustomDomain{
				ContainerID:   1123123,
				Domain:        "www.error.com",
				Port:          80,
				ContainerPort: 4000,
			}
			Expect(cd.Insert(db)).NotTo(BeNil())
		})
	})
})
