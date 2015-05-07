package users_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/nitrous/controllers/users"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/verification"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
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
		params          *url.Values
		authTokenInputs test.AuthTokenInputs
		u               *user.User
		t               *token.Token
		ft              *test.FakeTracker
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, logger, ft)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}
		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
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

	Describe("POST /api/v0/users/email_signup", func() {
		doRequest := func() {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/users/email_signup", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should respond OK if no email is provided", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]string{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))
		})

		It("should respond OK if the email is invalid", func() {
			params = &url.Values{}
			params.Set("email", "foo.com")
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]string{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))
		})

		It("should create an email record if the email is fine", func() {
			params = &url.Values{}
			params.Set("email", "bar@foo.com")
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]string{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))

			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM email_captures WHERE email_address = $1;`, "bar@foo.com").Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})

	})

	Describe("POST /api/v0/users", func() {
		doRequest := func() {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/users", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when valid params are received", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foo@example.com")
				params.Set("username", "foo")
				params.Set("password", "123456")
				params.Set("name", "Foo Bar")
				params.Set("promo_code", "")
				doRequest()
			})

			It("creates a user record in the db", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Name).To(Equal("Foo Bar"))
				Expect(u.CreatedAt).NotTo(BeNil())
			})

			It("encrypts the password using bcrypt", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(bcrypt.Verify(u.EncryptedPassword, "123456")).To(BeTrue())
			})

			It("responds with created and serialized user", func() {
				var j struct {
					User *serializers.User `json:"user"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.User.Username).To(Equal("foo"))
				Expect(j.User.Email).To(Equal("foo@example.com"))
			})
		})

		for _, v := range []string{"email", "username", "password"} {
			Context(fmt.Sprintf(`when the required param "%s" is missing`, v), func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("email", "foo@example.com")
					params.Set("username", "foo")
					params.Set("password", "123456")
					params.Del(v)
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal(v + " can't be blank"))
				})
			})
		}

		Context("when other types of validation fails", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foobar")
				params.Set("username", "foo")
				params.Set("password", "123456")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("email is not valid"))
			})
		})

		Context("using promocodes", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foo@example.com")
				params.Set("username", "foo")
				params.Set("password", "123456")
				params.Set("name", "Foo Bar")
			})

			It("should respond with a bad request if the promo code does not exist", func() {
				params.Set("promo_code", "everythingfree")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_promo_code"))
				Expect(j["error_description"]).To(Equal("promo code does not exist"))
			})

			It("should assign the promocode against the user if valid", func() {
				c := &credittype.CreditType{
					Name:         "nye2013",
					Description:  sql.NullString{"Desc", true},
					Active:       true,
					DefaultValue: 900,
					Stock:        sql.NullInt64{19, true},
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())

				params.Set("promo_code", "nye2013")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.PromoCode.Valid).To(BeTrue())
				Expect(u.PromoCode.String).To(Equal("nye2013"))
			})
		})

		Context("with an existing user", func() {
			BeforeEach(func() {
				_, err := db.Exec("INSERT INTO users (username, email, encrypted_password) VALUES ('foo', 'foo@example.com', '!@#$%')")
				Expect(err).To(BeNil())
				params = &url.Values{}
				params.Set("email", "lol@example.com")
				params.Set("username", "bar")
				params.Set("password", "123456")
			})

			Context("when an existing username is given", func() {
				BeforeEach(func() {
					params.Set("username", "foo")
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal("username is taken"))
				})
			})

			Context("when an existing email is given", func() {
				BeforeEach(func() {
					params.Set("email", "foo@example.com")
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal("email is taken"))
				})
			})
		})
	})

	Describe("PUT /api/v0/user", func() {
		doRequest := func(params *url.Values) {
			var err error
			req, err = test.NewAuthedFormRequest(t, "PUT", "/api/v0/users", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should block if the user has billing issues", func() {
			test.MarkDelinquentUser(db, u)
			params := &url.Values{}
			params.Set("name", "Something")
			doRequest(params)
			test.VerifyDelinquentUserResponse(res)
		})

		Context("when valid fields are given", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("password", "abcdef")
				params.Set("email", "tupdate@example.com")
				params.Set("name", "Token Update")
				doRequest(params)
			})

			It("updates fields", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				updated, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(bcrypt.Verify(updated.EncryptedPassword, "abcdef")).To(BeTrue())
				Expect(updated.Name).To(Equal("Token Update"))
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *u.UpdatedAt))
			})
		})

		It("should remove all other login tokens on password change", func() {
			_, err := db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u.ID, "toke1")
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("password", "abcdef")
			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			type con struct {
				Token string `db:"token"`
			}
			tokens := []con{}
			err = db.Select(&tokens, `SELECT token FROM tokens WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			Expect(tokens).To(ConsistOf([]con{{Token: t.Token}}))
		})

		Context("when an existing email is given", func() {
			BeforeEach(func() {
				u = &user.User{
					Username: "taken",
					Password: "123456",
					Email:    "taken@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("email", "taken@example.com")
				doRequest(params)
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("validation_failed"))
				Expect(errDesc).To(Equal("email is taken"))
			})
		})

		Context("when validation fails", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("password", "a")
				doRequest(params)
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("validation_failed"))
				Expect(errDesc).To(ContainSubstring("too short"))
			})
		})

		Context("when username is given", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("username", "nocando")
				doRequest(params)
			})

			It("does not update", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.Username).To(Equal("tokenuser"))
				Expect(usr.UpdatedAt).To(Equal(u.UpdatedAt))
			})
		})

		Context("when an email is sent in the request", func() {
			var (
				fakeMailer *test.FakeMailer
				origMailer mailer.Mailer
			)

			BeforeEach(func() {
				_, err := db.Exec(`UPDATE users SET confirmed_at = now() WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				origMailer = user.Mailer
				fakeMailer = &test.FakeMailer{}
				user.Mailer = fakeMailer
			})

			It("should not fire off an email if the email is the same", func() {
				params := &url.Values{}
				params.Set("email", u.Email)
				doRequest(params)
				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(BeEmpty())

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should fire off an email if the email differs from the current value", func() {
				params := &url.Values{}
				params.Set("email", "att@example.com")
				doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(Equal("att@example.com"))

				Expect(fakeMailer.SendMailCalled).To(BeTrue())
				Expect(fakeMailer.Tos).To(Equal([]string{`"tokenuser" <att@example.com>`}))
			})

			It("should not fire off an email if the email is the same as the unconfirmed email", func() {
				_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'att@example.com' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("email", "att@example.com")
				doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(Equal("att@example.com"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should revert to the original email if provided while unconfirmed is different", func() {
				_, err := db.Exec(`UPDATE users SET confirmation_token = 'TOKEN', confirmation_sent_at = now(), confirmed_at = now(), unconfirmed_email = 'att@example.com' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("email", u.Email)
				doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(BeEmpty())
				Expect(usr.ConfirmedAt.Valid).To(BeTrue())
				Expect(usr.ConfirmationSentAt.Valid).To(BeFalse())
				Expect(usr.ConfirmationToken.String).To(BeEmpty())

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})
		})
	})

	Describe("POST /api/v0/user/phone", func() {
		doRequest := func(params *url.Values) {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/phone", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("updates the user's phone and phone_country_code", func() {
			params := &url.Values{}
			params.Set("phone", "(888)8-44-44 Ծ_Ծ") // test stripping of non-numeric
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Phone.String).To(Equal("88884444"))
			Expect(updated.PhoneCountryCode.String).To(Equal("65"))
			Expect(*updated.UpdatedAt).To(BeTemporally(">", *u.UpdatedAt))

			var j struct {
				User *serializers.User `json:"user"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.User.Username).To(Equal("tokenuser"))
			Expect(j.User.Phone).To(Equal("88884444"))
			Expect(j.User.PhoneCountryCode).To(Equal("65"))
			Expect(j.User.PhoneAuthed).To(BeFalse())
		})

		It("creates a verification record for the user's phone", func() {
			// Confirm that user doesn't already have a verification record.
			v, err := verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(Equal(sql.ErrNoRows))

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			v, err = verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(BeNil())
			Expect(v.UserID).To(Equal(u.ID))
			Expect(v.Phone).To(Equal("+6588884444"))
			Expect(v.Code).NotTo(BeEmpty())
		})

		It("responds with HTTP 400 Bad Request when phone is not given", func() {
			params := &url.Values{}
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
		})

		It("responds with HTTP 400 Bad Request when phone_country_code is not given", func() {
			params := &url.Values{}
			params.Set("phone", "88884444")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
		})

		It("responds with HTTP 400 Bad Request when phone number is already in the database", func() {
			// Create another user with a phone number.
			u2 := &user.User{
				Username: "anotheruser",
				Password: "123456",
				Email:    "anotheruser@example.com",
			}
			err := test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())
			phone := "88884444"
			phoneCountryCode := "65"
			_, err = db.Exec(`UPDATE users SET phone = $1, phone_country_code = $2, phone_authed = $3 WHERE id = $4;`, phone, phoneCountryCode, true, u2.ID)

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
			Expect(errDesc).To(Equal("phone number already exists"))
		})

		It("responds with HTTP 400 Bad Request when phone is already authed", func() {
			_, err := db.Exec(`
				UPDATE users SET phone = '5551231234', phone_country_code = '1', phone_authed = true WHERE id = $1
			`, u.ID)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("phone_authed"))

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.PhoneAuthed).To(BeTrue())
			Expect(u2.Phone.String).To(Equal("5551231234"))
		})
	})

	Describe("POST /api/v0/user/resend_phone_code", func() {
		doRequest := func() {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/resend_phone_code", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("creates a new verification for the user's phone", func() {
			// Update user's phone and add an existing verification.
			err := u.UpdatePhone(db, "88884444", "65")
			Expect(err).To(BeNil())
			v1, err := verification.Generate(db, u.ID, u.PhoneNumber())
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			v2, err := verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(BeNil())
			Expect(v2.Phone).To(Equal("+6588884444"))
			Expect(v2.Code).NotTo(Equal(v1.Code))

			// Verify response is { "sent": true }.
			j := map[string]bool{}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["sent"]).To(BeTrue())
		})

		It("responds with HTTP 400 Bad Request if user has not given us his phone number", func() {
			Expect(u.Phone.String).To(BeEmpty())
			Expect(u.PhoneCountryCode.String).To(BeEmpty())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("POST /api/v0/user/verify_phone", func() {
		doRequest := func(params *url.Values) {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/verify_phone", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		var v *verification.Verification

		BeforeEach(func() {
			var err error
			// Update user's phone and add an existing verification.
			err = u.UpdatePhone(db, "88884444", "65")
			Expect(err).To(BeNil())
			v, err = verification.Generate(db, u.ID, u.PhoneNumber())
			Expect(err).To(BeNil())
		})

		It("sets phone_authed to true if verification code is correct", func() {
			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			// Verify response is { "verified": true }.
			j := map[string]bool{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["verified"]).To(BeTrue())

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.PhoneAuthed).To(BeTrue())
		})

		It("sets user's billing profile to free tier if phone number belongs to a whitelisted country", func() {
			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateFree))
		})

		It("does not change user's billing profile to free tier if phone number does not belong to a whitelisted country", func() {
			// Put only Japan on the whitelist.
			orig := users.PhoneCountryWhitelist
			users.PhoneCountryWhitelist = map[string]string{
				"Japan": "81",
			}
			defer func() {
				users.PhoneCountryWhitelist = orig
			}()

			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateNoCard))
		})

		It("does not change a paid user's billing profile", func() {
			// Set user's billing profile to paid.
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StatePaid, u.ID)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaid))
		})

		It("responds with HTTP 400 Bad Request if verification code is wrong", func() {
			code := "WRONGCODE!"
			Expect(code).NotTo(Equal(v.Code))
			params := &url.Values{}
			params.Set("code", code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
			Expect(errDesc).To(Equal("code is incorrect"))
		})

		It("responds with HTTP 400 Bad Request when code is not given", func() {
			doRequest(nil)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
		})

		It("responds with HTTP 400 Bad Request if user has not given us his phone number", func() {
			// Create another user that does not have a phone in his record.
			u = &user.User{
				Username: "anotheruser",
				Password: "123456",
				Email:    "anotheruser@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			t = &token.Token{
				UserID: u.ID,
				Token:  "anothertoken",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("code", "SOMECODE")
			doRequest(params)

			doRequest(nil)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
		})
	})

	Describe("GET /api/v0/user", func() {
		doRequest := func(token string) {
			var err error
			req, err = test.NewRequest("GET", "/api/v0/user", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {

			var (
				userinfo map[string]interface{}
			)

			ItReturnsUserDetails := func() {
				It("returns user details", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					var ok bool
					userinfo, ok = j["user"].(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(userinfo).NotTo(BeNil())
					Expect(userinfo["username"]).To(Equal("tokenuser"))
					Expect(userinfo["email"]).To(Equal("tokenuser@example.com"))
					Expect(userinfo["created_at"]).NotTo(BeNil())
					Expect(userinfo["updated_at"]).NotTo(BeNil())
					Expect(userinfo["password"]).To(BeNil())
					Expect(userinfo["encrypted_password"]).To(BeNil())

					userprofile, ok := userinfo["user_profile"].(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(userprofile["advanced"]).NotTo(BeTrue())
				})
			}

			Context("when user has billing issues", func() {
				BeforeEach(func() {
					test.MarkDelinquentUser(db, u)
					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()
			})

			Context("when user doesn't have billing issues", func() {
				BeforeEach(func() {
					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()

				It("responds with state", func() {
					billingprofile, ok := userinfo["billing_profile"].(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(billingprofile["state"]).To(Equal("no_card"))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/user/confirm", func() {
		doRequest := func(token string) {
			var err error
			params := &url.Values{}
			params.Set("token", token)
			req, err = test.NewRequest("POST", "/api/v0/user/confirm", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return a bad request when the token is not provided", func() {
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("validation_failed"))
			Expect(j["error_description"]).To(Equal("`token` must be provided"))
		})

		Context("with correct user", func() {

			var u *user.User
			curTime := time.Now()

			BeforeEach(func() {
				u = &user.User{
					Username: "qux",
					Name:     "quxboy",
					Password: "123456",
					Email:    "qux@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = 'ABCD' WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
				Expect(err).To(BeNil())
			})

			It("should error out if the token is incorrect", func() {
				doRequest("sdas")

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should successfully return a token for the user", func() {
				Expect(u.ConfirmedAt.Valid).To(BeFalse(), "expect user to not confirmed yet")
				doRequest("ABCD")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["access_token"]).ToNot(BeNil())
				Expect(j["token_type"]).ToNot(BeNil())

				t, err := token.FindByToken(db, j["access_token"].(string))
				Expect(err).To(BeNil())
				Expect(t).ToNot(BeNil())
				Expect(t.UserID).To(Equal(u.ID))

				// and track the confirmation on mixpanel
				Expect(ft.PropsCalled).To(HaveLen(2))
				pc := ft.PropsCalled[0]
				Expect(pc.UserID).To(Equal(u.ID))
				Expect(pc.Mode).To(Equal("setnx"))
				ctime, ok := pc.PropsSet["confirmed_email_date"]
				Expect(ok).To(BeTrue())
				pqt, ok := ctime.(time.Time)
				Expect(ok).To(BeTrue())
				Expect(pqt).To(BeTemporally("~", time.Now(), time.Minute))

				// and update email
				pc2 := ft.PropsCalled[1]
				Expect(pc2.UserID).To(Equal(u.ID))
				Expect(pc2.Mode).To(Equal("set"))
				Expect(pc2.PropsSet["$email"]).To(Equal(u.Email))
			})

			It("should communicate if the confirmation has lapsed", func() {
				_, err := db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = 'ABCD' WHERE id = $2;`, curTime.Add(-73*time.Hour), u.ID)
				Expect(err).To(BeNil())

				doRequest("ABCD")

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("confirmation_expired"))
				Expect(j["error_description"]).To(Equal(user.ErrConfirmationLapsed.Error()))
				Expect(j["confirmed"]).To(BeNil())
			})

		})
	})

	Describe("POST /api/v0/user/resend_confirm", func() {
		doRequest := func(token, appToken string) {
			var err error
			params := &url.Values{}
			params.Set("token", token)
			params.Set("app_token", appToken)
			req, err = test.NewRequest("POST", "/api/v0/user/resend_confirm", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return a bad request when the token is not provided", func() {
			doRequest("", "")
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("validation_failed"))
			Expect(j["error_description"]).To(Equal("`token` must be provided"))
		})

		Context("with remaining confirmation token", func() {
			var u *user.User
			curTime := time.Now()

			BeforeEach(func() {
				u = &user.User{
					Username: "qux",
					Name:     "quxboy",
					Password: "123456",
					Email:    "qux@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = 'ABCD' WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
				Expect(err).To(BeNil())
			})

			It("should error out if the token is incorrect", func() {
				doRequest("sdas", "")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should error out if the user has been deleted", func() {
				_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				Expect(u.ConfirmationSentAt.Valid).To(BeFalse(), "expect confirmation to not be sent")
				doRequest("sdas", "")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should send the confirmation token if the token matches", func() {
				Expect(u.ConfirmationSentAt.Valid).To(BeFalse(), "expect confirmation to not be sent")
				doRequest("ABCD", "")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(BeNil())
				Expect(j["confirmation_sent"]).To(BeTrue())

				u, err = user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.ConfirmationToken.String).ToNot(BeEmpty())
				Expect(u.ConfirmationToken.String).ToNot(Equal("ABCD"))
				Expect(u.ConfirmationSentAt.Valid).To(BeTrue(), "expect confirmation to be sent")
			})
		})

		Context("with user's app token", func() {
			var u *user.User
			var t *token.Token

			BeforeEach(func() {
				u = &user.User{
					Username: "qux",
					Name:     "quxboy",
					Password: "123456",
					Email:    "qux@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				t = &token.Token{
					UserID: u.ID,
					Token:  "toke",
				}

				err = t.Insert(db)
				Expect(err).To(BeNil())
			})

			It("should error out if the token is incorrect", func() {
				doRequest("", "tokey")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should error out if the token is invalid", func() {
				_, err := db.Exec(`UPDATE tokens SET last_seen_at = now() - interval '14 days' WHERE token = 'toke';`)
				Expect(err).To(BeNil())

				doRequest("", "toke")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should error out if the user has been deleted", func() {
				_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				doRequest("", "toke")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should send the confirmation token if the token matches", func() {
				Expect(u.ConfirmationSentAt.Valid).To(BeFalse(), "expect confirmation to not be sent")
				doRequest("", "toke")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(BeNil())
				Expect(j["confirmation_sent"]).To(BeTrue())

				u, err = user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.ConfirmationToken.String).ToNot(BeEmpty())
				Expect(u.ConfirmationToken.String).ToNot(Equal("ABCD"))
				Expect(u.ConfirmationSentAt.Valid).To(BeTrue(), "expect confirmation to be sent")
			})
		})
	})

	Describe("POST /api/v0/user/dismiss_first_time", func() {
		doRequest := func() {
			var err error
			req, err = test.NewRequest("POST", "/api/v0/user/dismiss_first_time", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)
			handler.ServeHTTP(res, req)
		}

		It("should set the first_time to FALSE", func() {
			Expect(u.FirstTime).To(BeTrue())

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u.FirstTime).To(BeFalse())
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user/dismiss_first_time"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
