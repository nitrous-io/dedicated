package subscription_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/subscription")
}

var _ = Describe("models/subscription", func() {
	var (
		db *db.Database
		u  *user.User
		p  *plan.Plan
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "qux",
			Name:     "quxboy",
			Password: "123456",
			Email:    "qux@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		p = &plan.Plan{
			Slug:          "512mb",
			Name:          "Nano",
			MaxContainers: 2,
			Price:         995,
			CPU:           "1",
			Memory:        "512 MB",
			Storage:       "20 GB",
		}
		err = p.Insert(db)
		Expect(err).To(BeNil())

	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Insert()", func() {
		var s *subscription.Subscription

		It("persists the subscription in the database", func() {
			var err error
			s = &subscription.Subscription{
				UserID:   u.ID,
				PlanID:   p.ID,
				StripeID: "sub_001",
			}
			err = s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())
			Expect(s.CreatedAt).NotTo(BeNil())
			Expect(s.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Update()", func() {
		var s *subscription.Subscription

		It("updates the subscription in the database", func() {
			var err error
			s = &subscription.Subscription{
				UserID:   u.ID,
				PlanID:   p.ID,
				StripeID: "sub_001",
			}
			err = s.Insert(db)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeZero())

			oUpdatedAt := s.UpdatedAt
			s.HostID = 15
			err = s.Update(db)
			Expect(err).To(BeNil())

			Expect(s.HostID).To(Equal(int64(15)))
			Expect(s.UpdatedAt).NotTo(Equal(oUpdatedAt))
		})
	})

	Describe("Find()", func() {
		var h *host.Host

		BeforeEach(func() {
			hs := &HostStack{}
			hs, err := CreateHostStack(db, hs)
			Expect(err).To(BeNil())
			h = hs.H

			s := &subscription.Subscription{
				UserID:   u.ID,
				PlanID:   p.ID,
				StripeID: "sub_001",
				HostID:   h.ID,
			}
			err = s.Insert(db)
			Expect(err).To(BeNil())
		})

		It("returns the subscription for the given host ID", func() {
			s, err := subscription.Find(db, h.ID)
			Expect(err).To(BeNil())
			Expect(s.ID).NotTo(BeNil())
			Expect(s.HostID).To(Equal(h.ID))
		})

		Context("when there's no subscription for the given host id", func() {
			It("returns nil", func() {
				s, err := subscription.Find(db, 9999)
				Expect(s).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				s, err := subscription.Find(db, h.ID)
				Expect(s).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})
})
