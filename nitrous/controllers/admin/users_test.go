package admin_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/db"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("controllers/admin", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		authTokenInputs test.AuthTokenInputs
		ft              *test.FakeTracker

		u *user.User
		t *token.Token

		trgt *user.User
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr := test.NewTestServer("", db, ft)
		authTokenInputs = test.AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		trgt = &user.User{
			Username: "bar",
			Password: "123456",
			Name:     "bar",
			Email:    "bar@example.com",
		}
		err = test.WrapInTransaction(db, trgt.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/admin/users/{username}", func() {
		var (
			err error
		)

		doRequest := func(t *token.Token, u string) *httptest.ResponseRecorder {
			path := fmt.Sprintf("/api/v0/admin/users/%s", u)
			req, err := test.NewAuthedFormRequest(t, "GET", path, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("should respond with 404 Not Found if the user is not an admin", func() {
			_, err = db.Exec(`UPDATE users SET admin=false WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			res := doRequest(t, "bar")
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("returns an error if the user cannot be found", func() {
			res := doRequest(t, "bara")
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("responds with information about the user", func() {
			res := doRequest(t, "bar")
			Expect(res.Code).To(Equal(http.StatusOK))

			var ser struct {
				Users []*serializers.AdminUser `json:"users"`
			}
			err := json.NewDecoder(res.Body).Decode(&ser)
			Expect(err).To(BeNil())
			Expect(ser.Users).To(HaveLen(1))

			u := ser.Users[0]
			Expect(u.ID).To(Equal(int64(trgt.ID)))
			Expect(u.BillingProfile).ToNot(BeNil())
			Expect(u.UserProfile).ToNot(BeNil())
		})

		It("responds with information about the deleted users", func() {
			_, err := db.Exec(`UPDATE users SET banned_at=now(), banned_by_id=$1, deleted_at = now() WHERE id = $2;`, u.ID, trgt.ID)
			Expect(err).To(BeNil())

			trgt2 := &user.User{
				Username: "bar",
				Password: "123456",
				Name:     "bar",
				Email:    "bar@example.com",
			}
			err = test.WrapInTransaction(db, trgt2.Insert)
			Expect(err).To(BeNil())

			res := doRequest(t, "bar")
			Expect(res.Code).To(Equal(http.StatusOK))

			var ser struct {
				Users []*serializers.AdminUser `json:"users"`
			}
			err = json.NewDecoder(res.Body).Decode(&ser)
			Expect(err).To(BeNil())
			Expect(ser.Users).To(HaveLen(2))

			u := ser.Users[0]
			Expect(u.ID).To(Equal(int64(trgt2.ID)))
			Expect(u.BillingProfile).ToNot(BeNil())
			Expect(u.UserProfile).ToNot(BeNil())

			del := ser.Users[1]
			Expect(del.ID).To(Equal(int64(trgt.ID)))
			Expect(del.BannedAt).ToNot(BeNil())
			Expect(del.BannedBy).To(Equal("foo"))
			Expect(del.DeletedAt).ToNot(BeNil())
		})
	})

	Describe("DELETE /api/v0/admin/users/{id}/ban", func() {
		var (
			err error
		)

		doRequest := func(t *token.Token, id int64) *httptest.ResponseRecorder {
			path := fmt.Sprintf("/api/v0/admin/users/%d/ban", id)
			req, err := test.NewAuthedFormRequest(t, "DELETE", path, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("should respond with 404 Not Found if the user is not an admin", func() {
			_, err = db.Exec(`UPDATE users SET admin=false WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			res := doRequest(t, u.ID)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("returns an error if the user cannot be found", func() {
			res := doRequest(t, -1)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("should not continue if there is a host block", func() {
			tx, err := db.Beginx()
			Expect(err).To(BeNil())
			defer tx.Rollback()

			locked, err := ndb.TransactionLock(tx, ndb.HOST_CREATE_LOCK, int(trgt.ID))
			Expect(err).To(BeNil())
			Expect(locked).To(BeTrue())

			res := doRequest(t, trgt.ID)
			Expect(res.Code).To(Equal(http.StatusConflict))
		})

		It("should return an error if the terminate fails", func() {
			hs1, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: trgt,
			})
			Expect(err).To(BeNil())

			tFuncCalled := false
			tFunc := func(tx *sqlx.Tx, h *host.Host, headers http.Header, initiatorID int64) error {
				tFuncCalled = true
				Expect(h.ID).To(Equal(hs1.H.ID))
				Expect(initiatorID).To(Equal(u.ID))
				return errors.New("Woops!")
			}

			admin.SetTerminateFunc(tFunc)
			res := doRequest(t, trgt.ID)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(tFuncCalled).To(BeTrue())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM users WHERE id = $1 AND deleted_at IS NULL AND banned_at IS NULL`, trgt.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())
		})

		It("should continue if the host termination succeeds", func() {
			hs1, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:        "my-host-1",
					HourlyPrice: 2000,
				},
				U: trgt,
			})
			Expect(err).To(BeNil())

			cnr := &container.Container{
				CID:    "cafebeans",
				UserID: hs1.U.ID,
				HostID: hs1.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
			}
			err = cnr.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, cnr.ID, trgt.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO verifications
					(user_id, phone, code)
					VALUES ($1, '123456789', '61');`, trgt.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2), ($1, $3);", trgt.ID, "toke1", "toke2")
			Expect(err).To(BeNil())

			tFuncCalled := false
			tFunc := func(tx *sqlx.Tx, h *host.Host, headers http.Header, initiatorID int64) error {
				tFuncCalled = true
				Expect(h.ID).To(Equal(hs1.H.ID))
				Expect(initiatorID).To(Equal(u.ID))
				return nil
			}

			admin.SetTerminateFunc(tFunc)
			res := doRequest(t, trgt.ID)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(tFuncCalled).To(BeTrue())

			var found bool
			err = db.Get(&found, `SELECT COUNT(*) = 1 FROM users WHERE id = $1
					AND deleted_at IS NOT NULL AND banned_at IS NOT NULL AND banned_by_id = $2`, trgt.ID, u.ID)
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue())

			queries := []string{
				"SELECT COUNT(*) FROM container_collabs WHERE user_id = $1",
				"SELECT COUNT(*) FROM verifications WHERE user_id = $1",
				"SELECT COUNT(*) FROM tokens WHERE user_id = $1",
			}

			for _, qry := range queries {
				var count int
				err = db.Get(&count, qry, trgt.ID)
				Expect(err).To(BeNil())
				Expect(count).To(BeZero())
			}
		})

		It("should set Mixpanel properties", func() {
			res := doRequest(t, trgt.ID)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(ft.PropsCalled).To(HaveLen(1))
			pc := ft.PropsCalled[0]
			Expect(pc.UserID).To(Equal(trgt.ID))
			Expect(pc.Mode).To(Equal("set"))
			Expect(pc.PropsSet["deleted_date"]).To(BeTemporally("~", time.Now(), time.Minute))
			Expect(pc.PropsSet["banned_date"]).To(BeTemporally("~", time.Now(), time.Minute))
		})
	})
})
