package subscriptions_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
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
			s1 *subscription.Subscription
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
			bp1, err := u.BillingProfile(db)
			s1 = &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp1.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
			}
			err = test.WrapInTransaction(db, s1.Insert)
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
			s := subscriptions.Subscriptions
			Expect(s).To(HaveLen(1))
			Expect(s[0].ID).To(Equal(s1.ID))
			Expect(s[0].PlanID).To(Equal(s1.PlanID))
			Expect(s[0].State).To(Equal(s1.State))
			Expect(s[0].BillingProfileID).To(Equal(s1.BillingProfileID))
			Expect(s[0].CreatedAt).NotTo(BeNil())
			Expect(s[0].UpdatedAt).NotTo(BeNil())
		})
	})
})
