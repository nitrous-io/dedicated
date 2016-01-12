package coupons_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/coupon"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/coupons")
}

var _ = Describe("controllers/coupons", func() {
	var (
		db  *db.Database
		svr *server.Server

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = test.NewTestServer("", db, nil)

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
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /coupons", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "POST", "/api/v0/coupons", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("creates a coupon for the user", func() {
			params := &url.Values{}
			params.Set("code", "50-OFF")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusCreated))

			cp, err := coupon.Find(db, u.ID)
			Expect(err).To(BeNil())

			Expect(cp.UserID).To(Equal(u.ID))
			Expect(cp.Code).To(Equal("50-OFF"))
		})

		It("responds with HTTP 400 Bad Request when code param is not provided", func() {
			res := doRequest(nil)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})

		It("responds with HTTP 409 Conflict when user has an existing coupon", func() {
			cp := &coupon.Coupon{
				UserID: u.ID,
				Code:   "existing-coupon",
			}
			Expect(cp.Insert(db)).To(BeNil())

			params := &url.Values{}
			params.Set("code", "50-OFF")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusConflict))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("coupon_already_exists"))
			Expect(errDesc).To(Equal("you already have an existing coupon"))
		})
	})
})
