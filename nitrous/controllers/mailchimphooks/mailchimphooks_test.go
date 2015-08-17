package mailchimphooks_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/mailchimphooks")
}

var _ = Describe("controllers/mailchimphooks", func() {
	var (
		db      *db.Database
		u       *user.User
		res     *httptest.ResponseRecorder
		handler *mux.Router
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		svr := NewTestServer("", db, NewFakeTracker())

		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		envvars.MailChimpIDNewsletter = "newsletter"
		envvars.MailChimpIDAnnouncements = "announcements"
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/mailchimp_hook", func() {
		doRequest := func() {
			req, err := NewRequest("GET", "/api/v0/mailchimp_hook", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should respond with a 200 OK", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("POST /api/v0/mailchimp_hook", func() {
		doRequest := func(params *url.Values) {
			req, err := NewRequest("POST", "/api/v0/mailchimp_hook", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should respond with a 200 OK", func() {
			doRequest(nil)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("should update the user profile for a subscribe", func() {
			_, err := db.Exec("UPDATE user_profiles SET subscribe_newsletter = FALSE, subscribe_announcements = FALSE WHERE user_id = $1", u.ID)
			Expect(err).To(BeNil())

			for _, listID := range []string{"newsletter", "announcements"} {
				params := &url.Values{}
				params.Set("data[list_id]", listID)
				params.Set("data[email]", "tokenuser@example.com")
				params.Set("type", "subscribe")
				doRequest(params)
				Expect(res.Code).To(Equal(http.StatusOK))
			}

			up, err := userprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(up.SubscribeNewsletter).To(BeTrue())
		})

		It("should update the user profile for a unsubscribe", func() {
			_, err := db.Exec("UPDATE user_profiles SET subscribe_newsletter = TRUE, subscribe_announcements = TRUE WHERE user_id = $1", u.ID)
			Expect(err).To(BeNil())

			for _, listID := range []string{"newsletter", "announcements"} {
				params := &url.Values{}
				params.Set("data[list_id]", listID)
				params.Set("data[email]", "tokenuser@example.com")
				params.Set("type", "unsubscribe")
				doRequest(params)
				Expect(res.Code).To(Equal(http.StatusOK))
			}

			up, err := userprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(up.SubscribeNewsletter).To(BeFalse())
		})
	})
})
