package subscriptions_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/subscriptions"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	test "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/subscriptions")
}

var _ = Describe("controllers/subscriptions", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router
		u       *user.User
		t       *token.Token
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil)
		handler = svr.Router()

		u = &user.User{
			Username: "user",
			Password: "123456",
			Email:    "user@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "foobarbazqux",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/subscriptions", func() {
		var (
			s *subscription.Subscription
		)

		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/subscriptions", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			// Insert plan
			p := &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
			}
			p.Insert(db)

			// Insert subscriptions
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			s = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())
		})

		It("should return subscriptions", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("responds with subscriptions", func() {
			res := doRequest()
			var subscriptions struct {
				Subscriptions []*serializers.Subscription `json:"subscriptions"`
			}

			err := json.NewDecoder(res.Body).Decode(&subscriptions)
			Expect(err).To(BeNil())
			subs := subscriptions.Subscriptions
			Expect(subs).To(HaveLen(1))
			Expect(subs[0].ID).To(Equal(s.ID))
			Expect(subs[0].PlanID).To(Equal(s.PlanID))
			Expect(subs[0].State).To(Equal(s.State))
			Expect(subs[0].BillingProfileID).To(Equal(s.BillingProfileID))
			Expect(subs[0].CreatedAt).NotTo(BeNil())
			Expect(subs[0].UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("DELETE /api/v0/subscriptions/{id}/cancel", func() {
		var (
			s *subscription.Subscription
			p *plan.Plan
		)

		BeforeEach(func() {
			// Insert plan
			p = &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
			}
			p.Insert(db)

			// Insert subscriptions
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			s = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())
		})

		doRequest := func(subscriptionID int64) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/subscriptions/%d/cancel", subscriptionID)
			req, err := test.NewAuthedRequest(t, "DELETE", url, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should request to cancel the subscription", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			res := doRequest(s.ID)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.CancelCalled).To(BeTrue())
			Expect(fsub.CancelSubscription.ID).To(Equal(s.ID))
		})

		It("should return not found if the subscription does not belong to the user", func() {
			u2 := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			bp, err := u2.BillingProfile(db)
			Expect(err).To(BeNil())

			s2 := &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"2", true},
			}
			err = test.WrapInTransaction(db, s2.Insert)
			Expect(err).To(BeNil())

			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			res := doRequest(s2.ID)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(fsub.CancelCalled).To(BeFalse())
			Expect(fsub.CancelSubscription).To(BeNil())
		})
	})
})
