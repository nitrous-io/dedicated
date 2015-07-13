package subscriptions_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/subscriptions"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	sub "github.com/nitrous-io/dedicated/nitrous/subscriptions"
	test "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	stripe "github.com/stripe/stripe-go"
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
		svr = server.NewServer("", db, nil, nil)
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

	Describe("GET /api/v0/subscriptions/:id/upgrade", func() {
		var (
			s          *subscription.Subscription
			bp         *billingprofile.BillingProfile
			trgtPlan   *plan.Plan
			fakeStripe *test.FakeStripe
		)

		doRequest := func(id int64, planID int64) *httptest.ResponseRecorder {
			uri := fmt.Sprintf("/api/v0/subscriptions/%d/upgrade", id)
			params := &url.Values{
				"plan_id": []string{strconv.FormatInt(planID, 10)},
			}
			req, err := test.NewAuthedRequest(t, "GET", uri, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)

			// Insert plan
			p := &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
			}
			Expect(p.Insert(db)).To(BeNil())

			// Insert subscriptions
			var err error
			bp, err = u.BillingProfile(db)
			Expect(err).To(BeNil())

			s = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())

			// plan to upgrade to
			trgtPlan = &plan.Plan{
				Slug:          "plan-b",
				Name:          "better-plan",
				MaxContainers: 20,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
				StripePlanID:  sql.NullString{"upgrade-to", true},
			}
			Expect(trgtPlan.Insert(db)).To(BeNil())
		})

		It("should return an error if the plan_id is incorrect", func() {
			res := doRequest(s.ID, -1)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(fakeStripe.ReceivedRequests).To(BeEmpty())
		})

		It("should return an estimate from Stripe", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{"object": "invoice", "id": "ABC", "total": 230}`,
			}

			res := doRequest(s.ID, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))
			var str struct {
				Total int `json:"total"`
			}

			err := json.NewDecoder(res.Body).Decode(&str)
			Expect(err).To(BeNil())
			Expect(str.Total).To(Equal(230))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["path"]).To(Equal("/invoices/upcoming"))
			Expect(r["customer"]).To(Equal(bp.StripeID.String))
			Expect(r["subscription"]).To(Equal(s.StripeID.String))
			Expect(r["subscription_plan"]).To(Equal("upgrade-to"))
		})

		It("should support exempt profiles", func() {
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StateExempt, u.ID)
			Expect(err).To(BeNil())

			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{"object": "invoice", "id": "ABC", "total": 230}`,
			}

			res := doRequest(s.ID, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))
			var str struct {
				Total int `json:"total"`
			}

			err = json.NewDecoder(res.Body).Decode(&str)
			Expect(err).To(BeNil())
			Expect(str.Total).To(Equal(230))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["path"]).To(Equal("/invoices/upcoming"))
			Expect(r["customer"]).To(Equal(bp.StripeID.String))
			Expect(r["subscription"]).To(Equal(s.StripeID.String))
			Expect(r["subscription_plan"]).To(Equal("upgrade-to-exempt"))
		})
	})

	Describe("POST /api/v0/subscriptions/:id/upgrade", func() {
		var (
			s        *subscription.Subscription
			trgtPlan *plan.Plan
			fsub     *test.FakeSubscriptionProvider
		)

		doRequest := func(id int64, planID int64) *httptest.ResponseRecorder {
			uri := fmt.Sprintf("/api/v0/subscriptions/%d/upgrade", id)
			params := &url.Values{
				"plan_id": []string{strconv.FormatInt(planID, 10)},
			}
			req, err := test.NewAuthedRequest(t, "POST", uri, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			fsub = &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			// Insert plan
			p := &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
			}
			Expect(p.Insert(db)).To(BeNil())

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

			// plan to upgrade to
			trgtPlan = &plan.Plan{
				Slug:          "plan-b",
				Name:          "better-plan",
				MaxContainers: 20,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
			}
			Expect(trgtPlan.Insert(db)).To(BeNil())
		})

		It("should return an error if the plan_id is incorrect", func() {
			res := doRequest(s.ID, -1)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(fsub.UpgradeCalled).To(BeFalse())
		})

		It("should return an error if the plan cannot be upgraded to", func() {
			fsub.UpgradeError = sub.ErrCannotUpgrade
			res := doRequest(s.ID, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			Expect(fsub.UpgradeCalled).To(BeTrue())
			Expect(fsub.UpgradeSubscription.ID).To(Equal(s.ID))
			Expect(fsub.UpgradePlan.ID).To(Equal(trgtPlan.ID))
			Expect(fsub.UpgradeUser.ID).To(Equal(u.ID))
		})

		It("should perform the upgrade", func() {
			res := doRequest(s.ID, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.UpgradeCalled).To(BeTrue())
			Expect(fsub.UpgradeSubscription.ID).To(Equal(s.ID))
			Expect(fsub.UpgradePlan.ID).To(Equal(trgtPlan.ID))
			Expect(fsub.UpgradeUser.ID).To(Equal(u.ID))
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
