package feedback_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers/feedback"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/feedback")
}

var _ = Describe("controllers/feedback", func() {
	var (
		origHelpScout helpscout.HelpScout
		fakeHelpScout *FakeHelpScout
		db            *db.Database
		svr           *server.Server
		handler       *mux.Router
		res           *httptest.ResponseRecorder

		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token

		username = "cats"
		name     = "CATS"
		email    = "cats@example.com"
	)

	BeforeEach(func() {
		var err error

		origHelpScout = feedback.HelpScout
		fakeHelpScout = &FakeHelpScout{}
		feedback.HelpScout = fakeHelpScout

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: username,
			Password: "123456",
			Name:     name,
			Email:    email,
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		_, err = db.Exec(`UPDATE billing_profiles SET state = 'delinquent'  WHERE user_id = $1;`, u.ID)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		feedback.HelpScout = origHelpScout
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
				userAgent := "Mozilla"
				platform := "MacIntel"
				currentURL := "https://nitrous.com"

				params := &url.Values{}
				params.Set("subject", subject)
				params.Set("body", body)
				params.Set("userAgent", userAgent)
				params.Set("platform", platform)
				params.Set("currentURL", currentURL)

				doRequest("valid-token", params)
				Expect(res.Code).To(Equal(http.StatusOK))

				var createdAt time.Time

				billingProfile, err := u.BillingProfile(db)
				Expect(err).To(BeNil())

				subject = fmt.Sprintf("[nitrous-pro] %s", subject)
				body = fmt.Sprintf("<strong>Username</strong>: %s\n<strong>Name</strong>: %s\n<strong>Email</strong>: %s\n%s\n\n", username, name, email, body)

				// FIXME: Get proper values for workspace
				metadata := strings.Join([]string{
					fmt.Sprintf("<strong>User Agent</strong>: %s", userAgent),
					fmt.Sprintf("<strong>Platform</strong>: %s", platform),
					fmt.Sprintf("<strong>Current URL</strong>: %s", currentURL),
					fmt.Sprintf("<strong>User Info:</strong><br>ID: %v<br>Username: %s<br>Email: %s<br>Confirmed At: %v<br>Phone Authed: %t<br>Billing Profile State: %s",
						u.ID,
						username,
						email,
						createdAt,
						false,
						billingProfile.State),
					fmt.Sprintf("<strong>Workspace(s) Info</strong>: %s<br>", ""),
				}, "<br>")

				customer := helpscout.Person{
					ID:        0,
					Email:     email,
					Username:  username,
					FirstName: name,
					Typ:       "customer",
				}

				mailbox := helpscout.MailboxRef{
					ID:   envvars.HelpScoutMailboxID,
					Name: envvars.HelpScoutMailboxName,
				}

				threads := []*helpscout.Thread{
					&helpscout.Thread{
						Typ: "customer",
						CreatedBy: helpscout.Person{
							Email:     email,
							Username:  username,
							FirstName: name,
							Typ:       "customer",
						},
						Body: body,
					},
					&helpscout.Thread{
						Typ: "note",
						CreatedBy: helpscout.Person{
							ID:  int64(envvars.HelpScoutUserID),
							Typ: "user",
						},
						Body: metadata,
					},
				}

				Expect(fakeHelpScout.CreateConversationCalled).To(Equal(1))
				Expect(fakeHelpScout.Customer).To(Equal(customer))
				Expect(fakeHelpScout.Subject).To(Equal(subject))
				Expect(fakeHelpScout.Mailbox).To(Equal(mailbox))
				Expect(fakeHelpScout.Threads).To(Equal(threads))

				j := map[string]interface{}{}

				err = json.Unmarshal(res.Body.Bytes(), &j)
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

				Expect(fakeHelpScout.CreateConversationCalled).To(Equal(0))

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
