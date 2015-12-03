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
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/controllers/subscriptions"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
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

	setupForOrg := func(s *subscription.Subscription, role string) {
		u2 := &user.User{
			Username: "user2org",
			Password: "123456",
			Email:    "user2org@example.com",
		}
		err := test.WrapInTransaction(db, u2.Insert)
		Expect(err).To(BeNil())

		o := &org.Org{
			Name:  "Foo Bar, Inc.",
			Slug:  "foobar",
			Email: "eng@foobar.com",
		}
		err = o.Insert(db)
		Expect(err).To(BeNil())
		err = db.WithTransaction(func(t *sqlx.Tx) error {
			Expect(billingprofile.Create(t, u2.ID, sql.NullInt64{o.ID, true})).To(BeNil())
			return nil
		})
		Expect(err).To(BeNil())
		err = orgmembership.AddMember(db, o.ID, u.ID, role)
		Expect(err).To(BeNil())

		bp, err := billingprofile.FindForOrg(db, o.ID)
		Expect(err).To(BeNil())

		_, err = db.Exec("UPDATE subscriptions SET multi_host = TRUE, billing_profile_id = $1 WHERE id = $2;", bp.ID, s.ID)
		Expect(err).To(BeNil())

		// join the user via a host if needed
		_, err = subscription.FindBySlug(db, s.Slug, u.ID)
		if err == nil {
			return
		}

		Expect(err).To(Equal(sql.ErrNoRows), "Unexpected error from join check")
		hs, err := test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-joining-org-host",
				HourlyPrice: 2000,
			},
			U: u,
		})
		Expect(err).To(BeNil())

		_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s.ID, hs.H.ID)
		Expect(err).To(BeNil())
	}

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
				Hidden:        true,
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
				Slug:             "SLUG",
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
			Expect(subs[0].Slug).To(Equal(s.Slug))
			Expect(subs[0].PlanID).To(Equal(s.PlanID))
			Expect(subs[0].State).To(Equal(s.State))
			Expect(subs[0].BillingProfileID).To(Equal(s.BillingProfileID))
			Expect(subs[0].CreatedAt).NotTo(BeNil())
			Expect(subs[0].UpdatedAt).NotTo(BeNil())
			Expect(subs[0].Plan).ToNot(BeNil())
			Expect(subs[0].Plan.Slug).To(Equal("plan-a"))
		})
	})

	Describe("GET /api/v0/orgs/{org_slug}/subscriptions", func() {
		var (
			s  *subscription.Subscription
			bp *billingprofile.BillingProfile
			o  *org.Org
		)

		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/orgs/foobar/subscriptions", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())
			err = db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())
			err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
			Expect(err).To(BeNil())

			// Insert plan
			p := &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
				Hidden:        true,
			}
			p.Insert(db)

			// Insert user subscriptions
			bp, err = u.BillingProfile(db)
			Expect(err).To(BeNil())

			uS := &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"sub_2xx", true},
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, uS.Insert)
			Expect(err).To(BeNil())

			// Insert org subscriptions
			bp, err = billingprofile.FindForOrg(db, o.ID)
			Expect(err).To(BeNil())

			s = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"sub_1x22", true},
				Slug:             "orgSub",
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
			Expect(subs[0].Slug).To(Equal(s.Slug))
			Expect(subs[0].PlanID).To(Equal(s.PlanID))
			Expect(subs[0].State).To(Equal(s.State))
			Expect(subs[0].BillingProfileID).To(Equal(bp.ID))
			Expect(subs[0].CreatedAt).NotTo(BeNil())
			Expect(subs[0].UpdatedAt).NotTo(BeNil())
			Expect(subs[0].Plan).ToNot(BeNil())
			Expect(subs[0].Plan.Slug).To(Equal("plan-a"))
		})

		It("should return forbidden if the user is not the BP owner", func() {
			_, err := db.Exec("UPDATE org_memberships SET role = $1 WHERE user_id = $2;", orgmembership.RoleAdmin, u.ID)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("should return forbidden if the org does not exist", func() {
			_, err := db.Exec("UPDATE orgs SET slug = 'not_here' WHERE id = $1;", o.ID)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("POST /api/v0/orgs/{org_slug}/subscriptions", func() {
		var (
			o    *org.Org
			p    *plan.Plan
			pc   *promocode.PromoCode
			fsub *test.FakeSubscriptionProvider
		)

		doRequest := func() *httptest.ResponseRecorder {
			u := &url.Values{}
			if p != nil {
				u.Set("plan", p.Slug)
			}
			if pc != nil {
				u.Set("promo_code", pc.Slug)
			}
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/orgs/foobar/subscriptions", u)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			fsub = &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())
			err = db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())
			err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
			Expect(err).To(BeNil())

			// Insert plan
			p = &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
				Hidden:        true,
			}
			Expect(p.Insert(db)).To(BeNil())
		})

		It("should return an error if a plan slug is not provided", func() {
			p = nil

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("plan_not_provided"))
			Expect(errDesc).To(Equal("`plan` must be provided"))

			Expect(fsub.SubscribeCalled).To(BeFalse())
		})

		It("should return an error if the plan slug does not exist", func() {
			p = &plan.Plan{Slug: "not-here-too-bad"}

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("plan_not_found"))
			Expect(errDesc).To(Equal("the specified `plan` does not exist"))

			Expect(fsub.SubscribeCalled).To(BeFalse())
		})

		It("should not allow access if user is not an owner", func() {
			_, err := db.Exec("UPDATE org_memberships SET role = 'admin' WHERE user_id = $1", u.ID)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("incorrect_role"))
			Expect(errDesc).To(Equal("You don't have permission to access this subscription"))

			Expect(fsub.SubscribeCalled).To(BeFalse())
		})

		It("should return an error if a subscriptions.ErrExistingMultiHostSubscription is returned", func() {
			fsub.SubscribeError = sub.ErrExistingMultiHostSubscription

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("upgrade_plan"))
			Expect(errDesc).To(Equal("You already have a subscription, please upgrade."))

			Expect(fsub.SubscribeCalled).To(BeTrue())
		})

		It("should return a serialized subscription on success", func() {
			sub := &subscription.Subscription{
				Slug:   "new-thing",
				PlanID: p.ID,
				State:  "active",
			}
			fsub.SubscribeReturn = sub

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var respData struct {
				Subscription *serializers.Subscription `json:"subscription"`
			}

			err := json.NewDecoder(res.Body).Decode(&respData)
			Expect(err).To(BeNil())
			s := respData.Subscription
			Expect(s.Slug).To(Equal(s.Slug))
			Expect(s.PlanID).To(Equal(s.PlanID))
			Expect(s.State).To(Equal(s.State))
			Expect(s.Plan).ToNot(BeNil())
			Expect(s.Plan.Slug).To(Equal(p.Slug))
			Expect(s.HostSlug).To(BeEmpty())
		})

		It("should silently ignore unquantity promocodes", func() {
			pc = &promocode.PromoCode{Slug: "cant_find_me"}

			sub := &subscription.Subscription{
				Slug:   "new-thing",
				PlanID: p.ID,
				State:  "active",
			}
			fsub.SubscribeReturn = sub

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.SubscribeCalled).To(BeTrue())
			Expect(fsub.SubscribeOpts.PromoCode).To(BeNil())
		})

		It("should pass in a quantity promocode", func() {
			err := db.Get(pc, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('just10', 10, 900, now()+interval '2 days') RETURNING *;")
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{
				Slug:   "new-thing",
				PlanID: p.ID,
				State:  "active",
			}
			fsub.SubscribeReturn = sub

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.SubscribeCalled).To(BeTrue())
			Expect(fsub.SubscribeOpts.PromoCode).ToNot(BeNil())
			Expect(fsub.SubscribeOpts.PromoCode.ID).To(Equal(pc.ID))
		})
	})

	Describe("GET /api/v0/subscriptions/:slug/upgrade", func() {
		var (
			s          *subscription.Subscription
			bp         *billingprofile.BillingProfile
			trgtPlan   *plan.Plan
			fakeStripe *test.FakeStripe
		)

		doRequest := func(slug string, planID int64) *httptest.ResponseRecorder {
			uri := fmt.Sprintf("/api/v0/subscriptions/%s/upgrade", slug)
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
				Slug:             "SLUG",
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
			res := doRequest(s.Slug, -1)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(fakeStripe.ReceivedRequests).To(BeEmpty())
		})

		It("should return an estimate from Stripe", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				stripeUpgradeResponse,
			}

			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))
			var str struct {
				Total int `json:"total"`
			}

			err := json.NewDecoder(res.Body).Decode(&str)
			Expect(err).To(BeNil())
			Expect(str.Total).To(Equal(2001))

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
				stripeUpgradeResponse,
			}

			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))
			var str struct {
				Total int `json:"total"`
			}

			err = json.NewDecoder(res.Body).Decode(&str)
			Expect(err).To(BeNil())
			Expect(str.Total).To(Equal(2001))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r := fakeStripe.ReceivedRequests[0]
			Expect(r["path"]).To(Equal("/invoices/upcoming"))
			Expect(r["customer"]).To(Equal(bp.StripeID.String))
			Expect(r["subscription"]).To(Equal(s.StripeID.String))
			Expect(r["subscription_plan"]).To(Equal("upgrade-to-exempt"))
		})

		It("should return forbidden if the subscription is MultiHost not owned by the user", func() {
			setupForOrg(s, "member")
			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("should be allowed if the subscription is MultiHost and is owned by the user", func() {
			setupForOrg(s, "owner")
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				stripeUpgradeResponse,
			}

			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("POST /api/v0/subscriptions/:slug/upgrade", func() {
		var (
			s        *subscription.Subscription
			trgtPlan *plan.Plan
			fsub     *test.FakeSubscriptionProvider
		)

		doRequest := func(slug string, planID int64) *httptest.ResponseRecorder {
			uri := fmt.Sprintf("/api/v0/subscriptions/%s/upgrade", slug)
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
				Slug:             "SLUG",
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
			res := doRequest(s.Slug, -1)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(fsub.UpgradeCalled).To(BeFalse())
		})

		It("should return an error if the plan cannot be upgraded to", func() {
			fsub.UpgradeError = sub.ErrCannotUpgrade
			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			Expect(fsub.UpgradeCalled).To(BeTrue())
			Expect(fsub.UpgradeSubscription.ID).To(Equal(s.ID))
			Expect(fsub.UpgradePlan.ID).To(Equal(trgtPlan.ID))
			Expect(fsub.UpgradeUser.ID).To(Equal(u.ID))
		})

		It("should perform the upgrade", func() {
			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.UpgradeCalled).To(BeTrue())
			Expect(fsub.UpgradeSubscription.ID).To(Equal(s.ID))
			Expect(fsub.UpgradePlan.ID).To(Equal(trgtPlan.ID))
			Expect(fsub.UpgradeUser.ID).To(Equal(u.ID))
		})

		It("should return forbidden if the subscription is MultiHost not owned by the user", func() {
			setupForOrg(s, "member")
			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			Expect(fsub.UpgradeCalled).To(BeFalse())
		})

		It("should be allowed if the subscription is MultiHost and is owned by the user", func() {
			setupForOrg(s, "owner")
			res := doRequest(s.Slug, trgtPlan.ID)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.UpgradeCalled).To(BeTrue())
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
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())
		})

		doRequest := func(slug string) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/subscriptions/%s/cancel", slug)
			req, err := test.NewAuthedRequest(t, "DELETE", url, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should request to cancel the subscription", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.CancelCalled).To(BeTrue())
			Expect(fsub.CancelSubscription.ID).To(Equal(s.ID))
		})

		It("should return a bad request if the subscription has been canceled", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			_, err := db.Exec("UPDATE subscriptions SET state = 'canceled' WHERE id = $1;", s.ID)
			Expect(err).To(BeNil())

			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("you cannot cancel a subscription that has been canceled"))

			Expect(fsub.CancelCalled).To(BeFalse())
		})

		It("should return a bad request if the subscription is pending cancellation", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			_, err := db.Exec("UPDATE subscriptions SET cancel_pending = true WHERE id = $1;", s.ID)
			Expect(err).To(BeNil())

			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("you cannot cancel a subscription that has been canceled"))

			Expect(fsub.CancelCalled).To(BeFalse())
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
				Slug:             "SLUG2",
			}
			err = test.WrapInTransaction(db, s2.Insert)
			Expect(err).To(BeNil())

			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			res := doRequest(s2.Slug)
			Expect(res.Code).To(Equal(http.StatusNotFound))

			Expect(fsub.CancelCalled).To(BeFalse())
			Expect(fsub.CancelSubscription).To(BeNil())
		})

		It("should return forbidden if the subscription is MultiHost not owned by the user", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			setupForOrg(s, "member")
			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			Expect(fsub.CancelCalled).To(BeFalse())
		})

		It("should be allowed if the subscription is MultiHost and is owned by the user", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			setupForOrg(s, "owner")
			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.CancelCalled).To(BeTrue())
			Expect(fsub.CancelSubscription.ID).To(Equal(s.ID))
		})
	})

	Describe("POST /api/v0/subscriptions/{id}/reactivate", func() {
		var (
			s  *subscription.Subscription
			p  *plan.Plan
			hs *test.HostStack
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
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())

			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s.ID, hs.H.ID)
			Expect(err).To(BeNil())
		})

		doRequest := func(slug string) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/subscriptions/%s/reactivate", slug)
			req, err := test.NewAuthedRequest(t, "POST", url, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should request to reactivate the subscription", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			_, err := db.Exec("UPDATE subscriptions SET cancel_pending = true WHERE id = $1;", s.ID)
			Expect(err).To(BeNil())

			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.ReactivateCalled).To(BeTrue())
			Expect(fsub.ReactivateSubscription.ID).To(Equal(s.ID))
		})

		It("should return a bad request if cancel_pending = false", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("you cannot reactivate a subscription that has not been cancelled"))

			Expect(fsub.ReactivateCalled).To(BeFalse())
		})

		It("should return a not found if host is terminated", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			_, err := db.Exec("UPDATE subscriptions SET cancel_pending = true WHERE id = $1;", s.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET state='terminated', terminated_at = NOW() WHERE subscription_id = $1;", s.ID)
			Expect(err).To(BeNil())

			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusNotFound))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("subscription_not_found"))
			Expect(errDesc).To(Equal("subscription does not exist"))

			Expect(fsub.ReactivateCalled).To(BeFalse())
		})

		It("should return a not found if the subscription does not belong to the user", func() {
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
				Slug:             "SLUG2",
			}
			err = test.WrapInTransaction(db, s2.Insert)
			Expect(err).To(BeNil())

			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			res := doRequest(s2.Slug)
			Expect(res.Code).To(Equal(http.StatusNotFound))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("subscription_not_found"))
			Expect(errDesc).To(Equal("subscription does not exist"))

			Expect(fsub.ReactivateCalled).To(BeFalse())
			Expect(fsub.ReactivateSubscription).To(BeNil())
		})

		It("should return forbidden if the subscription is MultiHost not owned by the user", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			setupForOrg(s, "member")
			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			Expect(fsub.ReactivateCalled).To(BeFalse())
		})

		It("should be allowed if the subscription is MultiHost and is owned by the user", func() {
			fsub := &test.FakeSubscriptionProvider{}
			subscriptions.SetSubscriptionProvider(fsub)

			_, err := db.Exec("UPDATE subscriptions SET cancel_pending = true WHERE id = $1;", s.ID)
			Expect(err).To(BeNil())

			setupForOrg(s, "owner")
			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(fsub.ReactivateCalled).To(BeTrue())
			Expect(fsub.ReactivateSubscription.ID).To(Equal(s.ID))
		})
	})

	Describe("GET /api/v0/subscriptions/{slug}/hosts", func() {
		var (
			s   *subscription.Subscription
			p   *plan.Plan
			hs  *test.HostStack
			hs2 *test.HostStack
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
				Slug:             "SLUG",
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())

			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s.ID, hs.H.ID)
			Expect(err).To(BeNil())

			hs2, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s.ID, hs2.H.ID)
			Expect(err).To(BeNil())

			hs3, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-3",
					HourlyPrice: 2000,
					State:       "terminated",
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s.ID, hs3.H.ID)
			Expect(err).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-4",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		doRequest := func(slug string) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/subscriptions/%s/hosts", slug)
			req, err := test.NewAuthedRequest(t, "GET", url, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("should return a not found if the subscription does not belong to the user", func() {
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
				Slug:             "SLUG2",
			}
			err = test.WrapInTransaction(db, s2.Insert)
			Expect(err).To(BeNil())

			res := doRequest(s2.Slug)
			Expect(res.Code).To(Equal(http.StatusNotFound))
			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("subscription_not_found"))
			Expect(errDesc).To(Equal("subscription does not exist"))
		})

		It("should return forbidden if the subscription is MultiHost not owned by the user", func() {
			setupForOrg(s, "member")
			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("should be allowed if the subscription is MultiHost and is owned by the user", func() {
			_, err := db.Exec("UPDATE subscriptions SET cancel_pending = true WHERE id = $1;", s.ID)
			Expect(err).To(BeNil())

			setupForOrg(s, "owner")
			res := doRequest(s.Slug)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Hosts []*serializers.HostWithUser `json:"hosts"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Hosts).To(HaveLen(2))
			Expect([]int64{j.Hosts[0].Host.ID, j.Hosts[1].Host.ID}).To(ConsistOf(hs.H.ID, hs2.H.ID))
			Expect([]string{j.Hosts[0].Username, j.Hosts[1].Username}).To(ConsistOf(hs.U.Username, hs2.U.Username))
		})
	})
})

var stripeUpgradeResponse = `
{
  "date": 1439597220,
  "period_start": 1436918820,
  "period_end": 1439597220,
  "lines": {
    "object": "list",
    "total_count": 3,
    "has_more": false,
    "url": "/v1/invoices/upcoming/lines?customer=cus_6bqrAO65m89tk1\u0026subscription=sub_6br883mCMXa2PN\u0026subscription_plan=nitro\u0026subscription_prorate=true",
    "data": [
      {
        "id": "ii_16Oe36AOjoYioo6eJIEJgEhm",
        "object": "line_item",
        "type": "invoiceitem",
        "livemode": false,
        "amount": -2499,
        "currency": "usd",
        "proration": true,
        "period": {
          "start": 1436921220,
          "end": 1439597220
        },
        "subscription": "sub_6br883mCMXa2PN",
        "quantity": 1,
        "plan": {
          "interval": "month",
          "name": "Turbo",
          "created": 1436913820,
          "amount": 4500,
          "currency": "usd",
          "id": "turbo",
          "object": "plan",
          "livemode": false,
          "interval_count": 1,
          "trial_period_days": null,
          "metadata": {},
          "statement_descriptor": null
        },
        "description": "Unused time on Turbo after 15 Jul 2015",
        "discountable": false,
        "metadata": {}
      },
      {
        "id": "ii_16Oe36AOjoYioo6ejrSL0TZw",
        "object": "line_item",
        "type": "invoiceitem",
        "livemode": false,
        "amount": 4500,
        "currency": "usd",
        "proration": true,
        "period": {
          "start": 1436921220,
          "end": 1439597220
        },
        "subscription": "sub_6br883mCMXa2PN",
        "quantity": 1,
        "plan": {
          "interval": "month",
          "name": "Nitro",
          "created": 1436913755,
          "amount": 8000,
          "currency": "usd",
          "id": "nitro",
          "object": "plan",
          "livemode": false,
          "interval_count": 1,
          "trial_period_days": null,
          "metadata": {},
          "statement_descriptor": null
        },
        "description": "Remaining time on Nitro after 15 Jul 2015",
        "discountable": false,
        "metadata": {}
      },
      {
        "id": "ii_16OdwhAOjoYioo6e6w4YBPGR",
        "object": "line_item",
        "type": "invoiceitem",
        "livemode": false,
        "amount": 4500,
        "currency": "usd",
        "proration": false,
        "period": {
          "start": 1436920823,
          "end": 1439597220
        },
        "subscription": "sub_6br883mCMXa2PN",
        "quantity": 1,
        "plan": {
          "interval": "month",
          "name": "Shift",
          "created": 1436918692,
          "amount": 2500,
          "currency": "usd",
          "id": "shift",
          "object": "plan",
          "livemode": false,
          "interval_count": 1,
          "trial_period_days": null,
          "metadata": {},
          "statement_descriptor": null
        },
        "description": "Unused time on Shift after 15 Jul 2015",
        "discountable": false,
        "metadata": {}
      }
    ]
  },
  "subtotal": 13496,
  "total": 13496,
  "customer": "cus_6bqrAO65m89tk1",
  "object": "invoice",
  "attempted": false,
  "closed": false,
  "forgiven": false,
  "paid": false,
  "livemode": false,
  "attempt_count": 0,
  "amount_due": 13496,
  "currency": "usd",
  "starting_balance": 0,
  "ending_balance": null,
  "next_payment_attempt": 1439600820,
  "webhooks_delivered_at": null,
  "charge": null,
  "discount": null,
  "application_fee": null,
  "subscription": "sub_6br883mCMXa2PN",
  "tax_percent": null,
  "tax": null,
  "metadata": {},
  "statement_descriptor": null,
  "description": null,
  "receipt_number": null,
  "subscription_proration_date": 1436921220
}
`
