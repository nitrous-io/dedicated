package feedback_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/feedback"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/feedback")
}

var _ = Describe("controllers/feedback", func() {
	var (
		origMailer mailer.Mailer
		fakeMailer *FakeMailer
		db         *db.Database
		svr        *server.Server
		handler    *mux.Router
		res        *httptest.ResponseRecorder

		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		origMailer = feedback.Mailer
		fakeMailer = &FakeMailer{}
		feedback.Mailer = fakeMailer

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil, &dns.FakeProvider{})
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "cats",
			Password: "123456",
			Name:     "CATS",
			Email:    "cats@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		feedback.Mailer = origMailer
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/feedback", func() {
		doRequest := func(token string, params *url.Values) {
			if params == nil {
				params = &url.Values{}
			}
			req, err := NewFormRequest("POST", "/api/v0/feedback", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		ItSendsSuccessfully := func() {
			It("sends successfully", func() {
				subject := "All your server are belong to us"
				body := "You have no chance to survive make your time."

				params := &url.Values{}
				params.Set("subject", subject)
				params.Set("body", body)

				doRequest("valid-token", params)
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(fakeMailer.SendMailCalled).To(BeTrue())
				Expect(fakeMailer.From).To(Equal(`"Nitrous Pro" <mailer@nitrous.io>`))
				Expect(fakeMailer.Tos).To(Equal([]string{`"Nitrous Beta Feedback" <beta@nitrous.io>`}))
				Expect(fakeMailer.Ccs).To(Equal([]string{`"CATS" <cats@example.com>`}))
				Expect(fakeMailer.Bccs).To(BeNil())
				Expect(fakeMailer.ReplyTo).To(Equal(`"CATS" <cats@example.com>`))
				Expect(fakeMailer.Subject).To(Equal("[nitrous-pro] " + subject))
				Expect(fakeMailer.Body).To(Equal("Username: cats\nName: CATS\nEmail: cats@example.com\n" + body))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["status"].(string)).To(Equal("ok"))
			})
		}

		Context("when subject and body are provided", func() {
			ItSendsSuccessfully()
		})

		Context("when user has billing issues", func() {
			BeforeEach(func() {
				MarkDelinquentUser(db, u)
			})

			ItSendsSuccessfully()
		})

		Context("when no params are provided", func() {
			It("does not send email and it responds with 400 bad request", func() {
				doRequest("valid-token", nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"].(string)).To(Equal("invalid_request"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/feedback"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
