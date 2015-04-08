package passwords_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
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
		svr = server.NewServer("", db, logger, nil)
		handler = svr.Router()

		origMailer = user.Mailer
		fakeMailer = &FakeMailer{}
		user.Mailer = fakeMailer

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
		user.Mailer = origMailer
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
				envvars.WebHostname = "nitrous.example"

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
				Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app#/reset-password?token=%s", u.ResetPasswordToken.String)))
				Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("Hello %s", u.Name)))
				Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("https://nitrous.example/app#/reset-password?token=%s", u.ResetPasswordToken.String)))
			})
		})

		Describe("when an invalid login is given", func() {
			It("responds with 200 OK, but dont send an email", func() {
				res := doRequest("fake@example.com")
				Expect(res.Code).To(Equal(http.StatusOK))

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

			It("removes all other login tokens", func() {
				_, err := db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u.ID, "toke1")
				Expect(err).To(BeNil())

				err = u.SetResetPasswordToken(db)
				Expect(err).To(BeNil())

				res := doRequest(u.ResetPasswordToken.String, "newpass")
				Expect(res.Code).To(Equal(http.StatusOK))

				type con struct {
					Token string `db:"token"`
				}
				tokens := []con{}
				err = db.Select(&tokens, `SELECT token FROM tokens WHERE user_id = $1`, u.ID)
				Expect(err).To(BeNil())
				Expect(len(tokens)).To(Equal(0))
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
