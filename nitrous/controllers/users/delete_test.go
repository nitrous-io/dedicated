package users_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gorilla/mux"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stripe/stripe-go"
)

var _ = Describe("controllers/users", func() {
	var (
		u   *user.User
		hs1 *test.HostStack
		t   *token.Token

		db      *ndb.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
		ft      *test.FakeTracker
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, logger, ft, &dns.FakeProvider{})
		handler = svr.Router()
		res = httptest.NewRecorder()

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		hs1, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 2000,
			},
			U: u,
		})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	sharedCanCreateExamples := func(doRequest func()) {
		It("should return an error if running hosts are present", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("hosts_not_terminated"))
		})

		It("should return an error if the user has outstanding invoices", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", hs1.H.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'unpaid', now() - interval '2 hours', now());", u.ID)
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("outstanding_invoices"))
		})

		It("should return an error if the user has a deferred invoice that has not been covered", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", hs1.H.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'deferred', now() - interval '2 hours', now());", u.ID)
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("outstanding_invoices"))
		})

		It("should return an error if the user doesn't have enough credit", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 1);", u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", hs1.H.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'paid', now() - interval '2 hours', now());", u.ID)
			Expect(err).To(BeNil())

			startTime := time.Now().Add(-2 * time.Second)
			hs2, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 9000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h2 := hs2.H

			_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", h2.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "born", "running", startTime, startTime.Add(1*time.Second))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h2.ID, "running", "terminated", startTime.Add(1*time.Second))
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("outstanding_usage"))
		})

		It("should not return an error if the user has enough credit to cover the outstanding charge", func() {
			_, err := db.Exec("INSERT INTO credits (user_id, amount) values ($1, 9000);", u.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", hs1.H.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO invoices (user_id, state, period_start, period_end) values ($1, 'paid', now() - interval '2 hours', now());", u.ID)
			Expect(err).To(BeNil())

			startTime := time.Now().Add(-2 * time.Second)
			hs2, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())
			h2 := hs2.H

			_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", h2.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "born", "running", startTime, startTime.Add(1*time.Second))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h2.ID, "running", "terminated", startTime.Add(1*time.Second))
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	}

	Describe("GET /api/v0/user/can_delete", func() {
		doRequest := func() {
			var err error
			req, err = test.NewAuthedFormRequest(t, "GET", "/api/v0/user/can_delete", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		sharedCanCreateExamples(doRequest)

		It("should be OK if the account can be deleted", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", hs1.H.ID)
			Expect(err).To(BeNil())

			hs2, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 2000,
				},
				U: u,
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET state = 'orphan' WHERE id = $1", hs2.H.ID)
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("DELETE /api/v0/user/delete", func() {
		var (
			fakeStripe *test.FakeStripe
		)

		BeforeEach(func() {
			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)

			fakeStripe.Responses = []string{
				listCardResp,
				delCardResp,
			}
		})

		doRequest := func() {
			var err error
			req, err = test.NewAuthedFormRequest(t, "DELETE", "/api/v0/user", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		sharedCanCreateExamples(doRequest)

		Context("with terminated hosts", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", hs1.H.ID)
				Expect(err).To(BeNil())

				hs2, err := test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:        "my-host-2",
						HourlyPrice: 2000,
					},
					U: u,
				})
				Expect(err).To(BeNil())

				_, err = db.Exec("UPDATE hosts SET state = 'orphan' WHERE id = $1", hs2.H.ID)
				Expect(err).To(BeNil())
			})

			It("should return an error if there is a host creation in progress", func() {
				tx, _ := db.Beginx()
				ndb.TransactionLock(tx, ndb.HOST_CREATE_LOCK, int(u.ID))
				defer tx.Rollback()

				doRequest()
				Expect(res.Code).To(Equal(http.StatusConflict))
				errType, _ := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("host_creation_in_progress"))
			})

			It("should successfully delete all user tokens", func() {
				_, err := db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2), ($1, $3);", u.ID, "toke1", "toke2")
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM tokens WHERE user_id = $1;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("should successfully remove container collabs for user", func() {
				cnr := &container.Container{
					CID:    "cafebeans",
					UserID: hs1.U.ID,
					HostID: hs1.H.ID,
					Image:  "nitrous-base",
					Slug:   "java-test-container",
				}
				err := cnr.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, cnr.ID, u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM container_collabs WHERE user_id = $1;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("should soft delete the free queue requests for the user", func() {
				_, err := db.Exec(`INSERT INTO free_queue
					(user_id, container_template_slug, container_name, host_name, region_slug)
					VALUES ($1, 'slug1', 'name', 'host', 'reg');`, u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = user.FindByID(db, u.ID)
				Expect(err).To(Equal(sql.ErrNoRows))

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM free_queue WHERE user_id = $1 AND deleted_at IS NOT NULL;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("should delete all verifications", func() {
				_, err := db.Exec(`INSERT INTO verifications
					(user_id, phone, code)
					VALUES ($1, '123456789', '61');`, u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = user.FindByID(db, u.ID)
				Expect(err).To(Equal(sql.ErrNoRows))

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM verifications WHERE user_id = $1;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("should soft delete the user", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err := user.FindByID(db, u.ID)
				Expect(err).To(Equal(sql.ErrNoRows))

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = $1 AND deleted_at IS NOT NULL;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				Expect(ft.PropsCalled).To(HaveLen(1))
				pc := ft.PropsCalled[0]
				Expect(pc.UserID).To(Equal(u.ID))
				Expect(pc.Mode).To(Equal("set"))
				Expect(pc.PropsSet["deleted_date"]).To(BeTemporally("~", time.Now(), time.Minute))
			})

			It("should erase the billing profile details", func() {
				_, err := db.Exec(`UPDATE
					 billing_profiles SET state = $1,
					 last_four = 3443,
					 stripe_id = 'St17',
					 stripe_fingerprint = 'print',
					 card_type = 'visa',
					 exp_month = 9,
					 exp_year = 2018,
					 pending_activation = FALSE,
					 max_hours = 1902
					 WHERE user_id = $2;`, billingprofile.StatePaid, u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				bp, err := billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())

				Expect(bp.State).To(Equal(billingprofile.StateNoCard))
				Expect(bp.LastFour.Valid).To(BeFalse())
				Expect(bp.CardType.Valid).To(BeFalse())
				Expect(bp.ExpMonth.Valid).To(BeFalse())
				Expect(bp.ExpYear.Valid).To(BeFalse())
				Expect(bp.PendingActivation).To(BeTrue())
				Expect(bp.MaxHours.Valid).To(BeFalse())

				Expect(bp.StripeID).To(Equal(sql.NullString{"St17", true}))
				Expect(bp.StripeFingerprint).To(Equal(sql.NullString{"print", true}))
			})

			It("should send a delete request to Stripe", func() {
				_, err := db.Exec(`UPDATE
					billing_profiles SET state = $1,
					last_four = 3443,
					stripe_id = 'St17',
					stripe_fingerprint = 'print',
					card_type = 'visa',
					exp_month = 9,
					exp_year = 2018,
					pending_activation = FALSE,
					max_hours = 1902
					WHERE user_id = $2;`, billingprofile.StatePaid, u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
				Expect(fakeStripe.ReceivedRequests[0]).To(Equal(map[string]string{
					"method": "GET",
					"path":   "/customers/St17/cards",
					"card":   "",
					"email":  "",
					"desc":   "",
				}))

				Expect(fakeStripe.ReceivedRequests[1]).To(Equal(map[string]string{
					"method": "DELETE",
					"path":   "/customers/St17/cards/card_15wTKjD2OOBduWpzVWXmAwVx",
				}))
			})

			It("should abort the transaction if the Stripe comm fails", func() {
				_, err := db.Exec(`UPDATE
					billing_profiles SET state = $1,
					last_four = 3443,
					stripe_id = 'St17',
					stripe_fingerprint = 'print',
					card_type = 'visa',
					exp_month = 9,
					exp_year = 2018,
					pending_activation = FALSE,
					max_hours = 1902
					WHERE user_id = $2;`, billingprofile.StatePaid, u.ID)
				Expect(err).To(BeNil())

				fakeStripe.Error = &stripe.Error{
					Type: stripe.ErrorType(stripe.APIErr),
					Msg:  "rate limited!",
					Code: stripe.ErrorCode(stripe.RateLimit),
				}

				doRequest()
				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

				var count int
				err = db.QueryRow(`SELECT COUNT(*) FROM tokens WHERE user_id = $1;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				bp, err := billingprofile.Find(db, u.ID)
				Expect(err).To(BeNil())

				Expect(bp.State).To(Equal(billingprofile.StatePaid))
				Expect(bp.LastFour.Valid).To(BeTrue())
				Expect(bp.CardType.Valid).To(BeTrue())
				Expect(bp.ExpMonth.Valid).To(BeTrue())
				Expect(bp.ExpYear.Valid).To(BeTrue())
				Expect(bp.PendingActivation).To(BeFalse())
				Expect(bp.MaxHours.Valid).To(BeTrue())

				Expect(bp.StripeID).To(Equal(sql.NullString{"St17", true}))
				Expect(bp.StripeFingerprint).To(Equal(sql.NullString{"print", true}))

				err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = $1 AND deleted_at IS NOT NULL;`, u.ID).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(0))
			})
		})
	})
})

const listCardResp = `
{
 "object": "list",
  "total_count": 1,
  "has_more": false,
  "data": [
    {
      "id": "card_15wTKjD2OOBduWpzVWXmAwVx",
      "object": "card",
      "last4": "4242",
      "brand": "Visa",
      "funding": "credit",
      "exp_month": 10,
      "exp_year": 2020,
      "country": "US",
      "name": "dqminh",
      "address_line1": null,
      "address_line2": null,
      "address_city": null,
      "address_state": null,
      "address_zip": "123456",
      "address_country": null,
      "cvc_check": "pass",
      "address_line1_check": null,
      "address_zip_check": "pass",
      "dynamic_last4": null,
      "metadata": {
      },
      "customer": "cus_68kq6g8lsrFE7o"
    }
  ]
}
`

const delCardResp = `
{
  "deleted": true,
  "id": "card_15wTKjD2OOBduWpzVWXmAwVx"
}
`
