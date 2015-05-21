package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/users")
}

var _ = Describe("controllers/users", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		req             *http.Request
		svr             *server.Server
		authTokenInputs AuthTokenInputs
		u               *user.User
		t               *token.Token
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = NewTestServer("", db, logger, NewFakeTracker(), &dns.FakeProvider{})
		handler = svr.Router()
		authTokenInputs = AuthTokenInputs{Handler: handler}
		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
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

	Describe("confirming account", func() {

		var u *user.User
		curTime := time.Now()

		doConfirmRequest := func(token string) {
			res = httptest.NewRecorder()
			var err error
			params := &url.Values{}
			params.Set("token", token)
			req, err = NewRequest("POST", "/api/v0/user/confirm", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		doResendRequest := func(token, appToken string) {
			res = httptest.NewRecorder()
			var err error
			params := &url.Values{}
			params.Set("token", token)
			params.Set("app_token", appToken)
			req, err = NewRequest("POST", "/api/v0/user/resend_confirm", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		doUserRequest := func(token string) {
			res = httptest.NewRecorder()
			var err error
			req, err = NewRequest("GET", "/api/v0/user", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = 'ABCD' WHERE id = $2;`, curTime.Add(-98*time.Hour), u.ID)
			Expect(err).To(BeNil())
		})

		It("should allow a user to resend a confirmation email, if the token has lapsed, and confirm it", func() {
			Expect(u.ConfirmedAt.Valid).To(BeFalse(), "expect user to not confirmed yet")
			doConfirmRequest("ABCD")

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("confirmation_expired"))
			Expect(j["access_token"]).To(BeNil())
			Expect(j["token_type"]).To(BeNil())

			doResendRequest("ABCD", "")
			Expect(res.Code).To(Equal(http.StatusOK))

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.ConfirmationToken.Valid).To(BeTrue())
			Expect(u2.ConfirmationToken.String).ToNot(Equal("ABCD"))
			Expect(u2.ConfirmationToken.String).ToNot(BeEmpty())

			doConfirmRequest(u2.ConfirmationToken.String)
			Expect(res.Code).To(Equal(http.StatusOK))
			j = map[string]interface{}{}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(BeNil())
			Expect(j["access_token"]).ToNot(BeNil())
			Expect(j["token_type"]).ToNot(BeNil())

			doUserRequest(j["access_token"].(string))
			Expect(res.Code).To(Equal(http.StatusOK))
			j = map[string]interface{}{}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			userinfo, ok := j["user"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(userinfo).NotTo(BeNil())
			Expect(userinfo["username"]).To(Equal("qux"))
		})

	})

})
