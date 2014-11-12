package passwords_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/passwords"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/passwords")
}

var _ = Describe("controllers/passwords", func() {
	var (
		db         *db.Database
		svr        *server.Server
		handler    *mux.Router
		origMailer mailer.Mailer
		fakeMailer *FakeMailer

		u *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()

		origMailer = passwords.Mailer
		fakeMailer = &FakeMailer{}
		passwords.Mailer = fakeMailer

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		passwords.Mailer = origMailer
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/send_reset_password_token", func() {
		doRequest := func(login string) *httptest.ResponseRecorder {
			p := &url.Values{}
			p.Set("login", login)
			req, err := NewRequest("POST", "/api/v0/send_reset_password_token", p)
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Describe("when a user exist for the given login", func() {
			It("sends the user an email with password reset instructions", func() {
				res := doRequest(u.Email)
				Expect(res.Code).To(Equal(http.StatusOK))

				Expect(fakeMailer.SendMailCalled).To(BeTrue())
				Expect(fakeMailer.From).To(Equal(`"Nitrous" <support@nitrous.io>`))
				Expect(fakeMailer.Tos).To(Equal([]string{fmt.Sprintf("%q <%s>", u.Name, u.Email)}))
				Expect(fakeMailer.Ccs).To(BeNil())
				Expect(fakeMailer.Bccs).To(BeNil())
				Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous" <support@nitrous.io>`))
				Expect(fakeMailer.Subject).To(Equal("[nitrous-pro] Reset password instructions"))
				Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("Hello %s", u.Name)))
				Expect(fakeMailer.HTML).To(ContainSubstring("Change my password"))
				Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("/app#reset_password?token=%s", u.ResetPasswordToken.String)))
			})
		})

		Describe("when an invalid login is given", func() {
			It("responds with 400 Bad Request and dont send an email", func() {
				res := doRequest("fake@example.com")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, errDesc := ErrorSlug(res.Body)
				Expect(err).To(Equal("invalid_login"))
				Expect(errDesc).To(Equal("username or email is required"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})
		})
	})

	Describe("POST /api/v0/reset_password", func() {
		doRequest := func(token, password string) *httptest.ResponseRecorder {
			p := &url.Values{}
			p.Set("reset_token", token)
			p.Set("password", password)
			req, err := NewRequest("POST", "/api/v0/reset_password", p)
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Describe("when given token is valid and password is not empty", func() {
			It("changes user's password'", func() {
				oldPassword := u.EncryptedPassword
				err := u.SetResetPasswordToken(db)
				Expect(err).To(BeNil())

				res := doRequest(u.ResetPasswordToken.String, "newpass")
				Expect(res.Code).To(Equal(http.StatusOK))

				// reload user
				u, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.EncryptedPassword).NotTo(Equal(oldPassword))
			})
		})

		Describe("when given token is valid but password is empty", func() {
			It("does not change user's password", func() {
				oldPassword := u.EncryptedPassword
				err := u.SetResetPasswordToken(db)
				Expect(err).To(BeNil())

				res := doRequest(u.ResetPasswordToken.String, "")
				Expect(res.Code).To(Equal(http.StatusOK))

				// reload user
				u, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.EncryptedPassword).To(Equal(oldPassword))
			})
		})

		Describe("when given token is not valid", func() {
			It("does not change user's password and responds with 404 Not Found'", func() {
				oldPassword := u.EncryptedPassword

				res := doRequest("abcd1234", "newpass")
				Expect(res.Code).To(Equal(http.StatusNotFound))

				// reload user
				u, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.EncryptedPassword).To(Equal(oldPassword))
			})
		})

		Describe("when given token is empty", func() {
			It("does not change user's password and responds with 404 Not Found'", func() {
				oldPassword := u.EncryptedPassword

				res := doRequest("abcd1234", "newpass")
				Expect(res.Code).To(Equal(http.StatusNotFound))

				// reload user
				u, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.EncryptedPassword).To(Equal(oldPassword))
			})
		})
	})
})
