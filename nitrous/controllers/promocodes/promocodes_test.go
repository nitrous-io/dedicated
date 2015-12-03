package promocodes_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/promocodes")
}

var _ = Describe("controllers/promocodes", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /subscriptions/promo_codes/{slug}", func() {
		var (
			pcID1 int64
			pcID2 int64
		)

		BeforeEach(func() {
			err := db.Get(&pcID1, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('just10', 10, 900, now()+interval '2 days') RETURNING id;")
			Expect(err).To(BeNil())
			err = db.Get(&pcID2, "INSERT INTO promo_codes(slug, quantity, expires_at) VALUES ('just20', 20, now()) RETURNING id;")
			Expect(err).To(BeNil())
		})

		doRequest := func(code string) {
			var err error
			req, err := test.NewRequest("GET", fmt.Sprintf("/api/v0/subscriptions/promo_codes/%v", code), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return 404 if the promocode does not exist", func() {
			doRequest("jest10")
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("should return the promo code if it exists", func() {
			doRequest("just10")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				PromoCode *serializers.PromoCode `json:"promo_code"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.PromoCode).ToNot(BeNil())
			Expect(j.PromoCode.Slug).To(Equal("just10"))
			Expect(j.PromoCode.TrialSeconds).To(Equal(int64(900)))
		})
	})

	Describe("GET /user/promo_codes", func() {
		var (
			u *user.User
			t *token.Token

			pcID1, pcID2 int64
			p1, p2       *plan.Plan
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

			t = &token.Token{
				UserID: u.ID,
				Token:  "valid-token",
			}
			Expect(t.Insert(db)).To(BeNil())

			p1 = &plan.Plan{
				Slug:          "promoPlan",
				Name:          "promoPlan",
				MaxContainers: 1,
				Price:         500,
				BasePrice:     500,
			}
			Expect(p1.Insert(db)).To(BeNil())

			p2 = &plan.Plan{
				Slug:          "promo2Plan",
				Name:          "promo2Plan",
				MaxContainers: 1,
				Price:         500,
				BasePrice:     500,
			}
			Expect(p2.Insert(db)).To(BeNil())

			err := db.Get(&pcID1, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('just10', 10, 900, now()+interval '2 days') RETURNING id;")
			Expect(err).To(BeNil())
			err = db.Get(&pcID2, "INSERT INTO promo_codes(slug, quantity) VALUES ('just20', 20) RETURNING id;")
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO promo_code_plans(promo_code_id, plan_id) SELECT id, $1 FROM promo_codes WHERE id IN ($2, $3)", p1.ID, pcID1, pcID2)
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO promo_code_plans(promo_code_id, plan_id) VALUES ($1, $2);", pcID1, p2.ID)
			Expect(err).To(BeNil())

			_, err = promocode.Claim(db, "just10", u.ID, p1.ID)
			Expect(err).To(BeNil())

			_, err = promocode.Claim(db, "just20", u.ID, p1.ID)
			Expect(err).To(BeNil())
		})

		doRequest := func() {
			var err error
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/user/promo_codes", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return an empty list if none are available", func() {
			_, err := db.Exec("DELETE FROM promo_code_uses;")
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				PromoCodes []*serializers.PromoCode `json:"promo_codes"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.PromoCodes).To(HaveLen(0))
		})

		It("should list all promo codes for user", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				PromoCodes []*serializers.PromoCode `json:"promo_codes"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.PromoCodes).To(HaveLen(2))

			Expect(j.PromoCodes[0].Slug).To(Equal("just10"))
			Expect(j.PromoCodes[0].PlanIDs).To(ConsistOf(p1.ID, p2.ID))

			Expect(j.PromoCodes[1].Slug).To(Equal("just20"))
			Expect(j.PromoCodes[1].PlanIDs).To(Equal([]int64{p1.ID}))
		})
	})

	Describe("GET /promo_codes", func() {
		BeforeEach(func() {
			c1 := &credittype.CreditType{
				Name:         "10BONUS",
				Active:       true,
				DefaultValue: int64(1000),
				Stock:        sql.NullInt64{int64(300), true},
				Used:         sql.NullInt64{int64(0), true},
			}
			c2 := &credittype.CreditType{
				Name:         "15BONUS",
				Active:       false,
				DefaultValue: int64(1500),
				Stock:        sql.NullInt64{int64(300), true},
				Used:         sql.NullInt64{int64(0), true},
			}
			c3 := &credittype.CreditType{
				Name:         "20BONUS",
				Active:       true,
				DefaultValue: int64(2000),
				Stock:        sql.NullInt64{int64(300), true},
				Used:         sql.NullInt64{int64(300), true},
			}
			var now time.Time = time.Now()
			c4 := &credittype.CreditType{
				Name:         "25BONUS",
				Active:       true,
				DefaultValue: int64(2500),
				Stock:        sql.NullInt64{int64(300), true},
				Used:         sql.NullInt64{int64(0), true},
				ExpiresAt:    pq.NullTime{now, true},
			}
			err := c1.Insert(db)
			Expect(err).To(BeNil())
			err = c2.Insert(db)
			Expect(err).To(BeNil())
			err = c3.Insert(db)
			Expect(err).To(BeNil())
			err = c4.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(code string) {
			var err error
			req, err = NewRequest("GET", fmt.Sprintf("/api/v0/promo_codes/%s", code), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when a valid promo code with a given name is found", func() {
			It("responds with 200 OK and promo code info", func() {
				doRequest("10BONUS")
				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					CreditAmount int64      `json:"credit_amount"`
					ExpiresAt    *time.Time `json:"expires_at"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.CreditAmount).To(Equal(int64(1000)))
			})
		})

		Context("when no valid promo code with a given name could be found", func() {
			It("responds with 404 not found", func() {
				doRequest("15BONUS")
				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(ContainSubstring("could not find"))

				res = httptest.NewRecorder()
				doRequest("20BONUS")
				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc = ErrorSlug(res.Body)
				Expect(errDesc).To(ContainSubstring("could not find"))
			})
		})
	})
})
