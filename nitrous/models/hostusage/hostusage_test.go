package hostusage_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostusage"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/hostusage")
}

var _ = Describe("models/hostusage", func() {
	var (
		db *db.Database
		u  *user.User
		h  *host.Host
		r  *region.Region
		p  *plan.Plan
		pt *providertemplate.ProviderTemplate
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		r = &region.Region{Slug: "apse1", Name: "Southeast Asia"}
		err = r.Insert(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{Slug: "512mb", Name: "micro", MaxContainers: 1, Price: 500}
		err = p.Insert(db)
		Expect(err).To(BeNil())

		pt = &providertemplate.ProviderTemplate{
			RegionID: r.ID,
			PlanID:   p.ID,
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
		err = pt.Insert(db)
		Expect(err).To(BeNil())

		h = &host.Host{
			UserID:             u.ID,
			Slug:               "my-host-1",
			RegionID:           r.ID,
			PlanID:             p.ID,
			ProviderTemplateID: pt.ID,
		}
		err = h.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Insert()", func() {
		It("should persist in the database", func() {
			t := time.Now()
			hU := &hostusage.HostUsage{
				HostID: h.ID,
				UserID: u.ID,
				Hours:  19,
				Day:    &t,
			}
			err := hU.Insert(db)
			Expect(err).To(BeNil())
			Expect(hU.ID).ToNot(Equal(0))

			// Timestamps are set.
			Expect(hU.CreatedAt).NotTo(BeNil())
			Expect(hU.UpdatedAt).NotTo(BeNil())

			h2 := &hostusage.HostUsage{}
			err = db.QueryRowx("SELECT * FROM host_usages WHERE id = $1", hU.ID).StructScan(h2)
			Expect(err).To(BeNil())
			Expect(hU).To(Equal(h2))
		})
	})

	Describe("Update()", func() {
		var hU *hostusage.HostUsage
		var t time.Time
		BeforeEach(func() {
			t = time.Now()
			hU = &hostusage.HostUsage{
				HostID: h.ID,
				UserID: u.ID,
				Hours:  19,
				Day:    &t,
			}
			Expect(hU.Insert(db)).To(BeNil())
		})

		It("should use user ID, host ID and the day to update", func() {
			target := &hostusage.HostUsage{
				HostID: h.ID,
				UserID: u.ID,
				Day:    &t,
				Hours:  16,
			}

			err := target.Update(db)
			Expect(err).To(BeNil())

			h2 := &hostusage.HostUsage{}
			err = db.QueryRowx("SELECT * FROM host_usages WHERE id = $1", hU.ID).StructScan(h2)
			Expect(err).To(BeNil())
			Expect(h2.Hours).To(Equal(16))
		})
	})

})
