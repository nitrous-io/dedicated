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
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
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
			u  *user.User
			t  *token.Token
			bp *billingprofile.BillingProfile

			pcID1 int64
			pcID2 int64
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

			var err error
			bp, err = u.BillingProfile(db)
			Expect(err).To(BeNil())

			err = db.Get(&pcID1, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('just10', 10, 900, now()+interval '2 days') RETURNING id;")
			Expect(err).To(BeNil())
			err = db.Get(&pcID2, "INSERT INTO promo_codes(slug, quantity, expires_at) VALUES ('just20', 20, now()) RETURNING id;")
			Expect(err).To(BeNil())
		})

		doRequest := func(code string) {
			var err error
			req, err := test.NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/subscriptions/promo_codes/%v", code), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return 404 if the promocode does not exist", func() {
			doRequest("jest10")
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("should return 404 if the promocode has expired", func() {
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
