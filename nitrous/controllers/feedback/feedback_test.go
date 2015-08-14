package feedback_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/controllers/feedback"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
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

		bp *billingprofile.BillingProfile

		u  *user.User
		t  *token.Token
		p  *plan.Plan
		s  *subscription.Subscription
		hs *HostStack
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

		_, err = db.Exec(`UPDATE billing_profiles SET state = 'delinquent'  WHERE user_id = $1;`, u.ID)
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, &HostStack{
			PT: &providertemplate.ProviderTemplate{
				Name:     "default-1",
				Provider: "amazon",
				Config: hstore.Hstore{
					Map: map[string]sql.NullString{
						"disable_eip": sql.NullString{"true", true},
					},
				},
			},
		})
		Expect(err).To(BeNil())

		bp, err = u.BillingProfile(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{
			Slug:          "plan-a",
			Name:          "cozy-workplace",
			MaxContainers: 16,
			Storage:       "1024 GB",
			Price:         1024,
			Memory:        "68 GB",
		}
		err = p.Insert(db)
		Expect(err).To(BeNil())

		s = &subscription.Subscription{
			PlanID:           p.ID,
			BillingProfileID: bp.ID,
			State:            "active",
			StripeID:         sql.NullString{"sub_ABC", true},
			Slug:             "Sub1",
		}
		err = WrapInTransaction(db, s.Insert)
		Expect(err).To(BeNil())

		_, err = db.Exec(`INSERT INTO hosts 
(user_id, plan_id, region_id, subscription_id, state, slug, provider_template_id, freighterd_version, docker_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
			u.ID, p.ID, hs.R.ID, s.ID, host.StateRunning, "abc", hs.PT.ID, "xyz", "abc")
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
				body := "You have no <b>chance</b> to survive make your time."
				userAgent := "Mozilla"
				platform := "MacIntel"
				currentURL := "https://nitrous.io"

				params := &url.Values{}
				params.Set("subject", subject)
				params.Set("body", body)
				params.Set("userAgent", userAgent)
				params.Set("platform", platform)
				params.Set("currentURL", currentURL)

				doRequest("valid-token", params)
				Expect(res.Code).To(Equal(http.StatusOK))

				Eventually(func() int {
					return fakeHelpScout.CreateConversationCalled
				}).Should(Equal(1))

				Expect(fakeHelpScout.Customer).To(Equal(helpscout.Person{ID: 0, Email: "cats@example.com", Username: "cats", FirstName: "CATS", Type: "customer"}))
				Expect(fakeHelpScout.Subject).To(Equal("[nitrous-pro] All your server are belong to us"))
				Expect(fakeHelpScout.Mailbox).To(Equal(helpscout.MailboxRef{ID: int(envvars.HelpScoutMailboxID), Name: (envvars.HelpScoutMailboxName)}))
				Expect(fakeHelpScout.Threads[0].Body).To(Equal("You have no &lt;b&gt;chance&lt;/b&gt; to survive make your time."))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("<strong>User Agent</strong>: Mozilla<br>"))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("<strong>Platform</strong>: MacIntel<br>"))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("<strong>Current URL</strong>: https://nitrous.io<br>"))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring(fmt.Sprintf("<strong>User Info:</strong><br>ID: %v", u.ID)))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("<br>Username: cats<br>Email: cats@example.com<br>"))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("Phone Authed: false<br>"))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("Billing Profile State: delinquent<br>"))
				Expect(fakeHelpScout.Threads[1].Body).To(ContainSubstring("<strong>Workspace(s) Info</strong>: <br>Slug: abc<br>Region Identifier: <br>State: running<br>Plan Name: cozy-workplace<br>Subscription State: active<br>"))

				j := map[string]interface{}{}

				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["status"].(string)).To(Equal("ok"))
			})
		}

		Context("when customer, subject, mailbox and body are provided", func() {
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
