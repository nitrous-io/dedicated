package announcements_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
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
	RunSpecs(t, "controllers/announcements")
}

var _ = Describe("controllers/announcements", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router

		h1 *host.Host
		u1 *user.User
		t  *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil, &dns.FakeProvider{})

		handler = svr.Router()

		u1 = &user.User{
			Username: "foo",
			Email:    "foo@example.org",
		}

		h1 = &host.Host{
			Slug: "elixir-web-1",
		}

		_, err = CreateHostStack(db, &HostStack{
			U: u1,
			H: h1,
		})
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u1.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/admin/announcements/new", func() {
		postAnnouncement := func(body string) *httptest.ResponseRecorder {
			req, err := NewBodyRequest("POST", "/api/v0/admin/announcements/new", body)
			Expect(err).To(BeNil())

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+t.Token)

			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			return res
		}

		expiryField := time.Now().Add(5 * time.Hour).Format(time.RFC3339)

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": true,
						"expires_at": "%s"
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u1.ID)
			})

			It("Global announcement should succeed", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": true,
						"expires_at": "%s"
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				anns, err := announcement.FindByHostID(db, h1.ID)
				Expect(err).To(BeNil())

				Expect(len(anns)).To(Equal(1))
				ann := anns[0]

				Expect(ann.ShowToAll).To(BeTrue())
				Expect(ann.HostID).To(Equal(h1.ID))
				Expect(ann.UserID).To(Equal(u1.ID))

				subject, err := ann.Subject()
				Expect(err).To(BeNil())
				Expect(subject).To(Equal("Global announcement for elixir-web-1"))

				message, err := ann.Message()
				Expect(err).To(BeNil())
				Expect(message).To(Equal("Hello foo, We will be upgrading your host elixir-web-1 tomorrow."))
			})

			It("Host announcement should succeed", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Host announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} next week.",
						"show_to_all": false,
						"expires_at": "%s",
						"slugs": ["elixir-web-1"]
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				anns, err := announcement.FindByHostID(db, h1.ID)
				Expect(err).To(BeNil())

				Expect(len(anns)).To(Equal(1))
				ann := anns[0]

				Expect(ann.ShowToAll).To(BeFalse())
				Expect(ann.HostID).To(Equal(h1.ID))
				Expect(ann.UserID).To(Equal(u1.ID))

				subject, err := ann.Subject()
				Expect(err).To(BeNil())
				Expect(subject).To(Equal("Host announcement for elixir-web-1"))

				message, err := ann.Message()
				Expect(err).To(BeNil())
				Expect(message).To(Equal("Hello foo, We will be upgrading your host elixir-web-1 next week."))
			})

			It("Invalid host slug should fail", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": false,
						"expires_at": "%s",
						"slugs": ["elixir-web-1", "host-not-exist"]
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("slugs_not_found"))
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("Empty host slug should fail", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": false,
						"expires_at": "%s",
						"slugs": []
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("no_slug_specified"))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})

			It("Missing slug should fail", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": false,
						"expires_at": "%s"
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("no_slug_specified"))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})

			It("Empty subject should fail", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": true,
						"expires_at": "%s"
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("empty_subject"))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})

			It("Missing message should fail", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"show_to_all": true,
						"expires_at": "%s"
					}
				}`, expiryField)

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("message_subject"))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})

			It("Missing expiry should fail", func() {
				annForm := `{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": true
					}
				}`

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_expiry"))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})

			It("Early expiry should fail", func() {
				annForm := fmt.Sprintf(`{
					"announcement": {
						"subject_template": "Global announcement for {{ .Slug }}",
						"message_template": "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
						"show_to_all": true,
						"expires_at": "%s"
					}
				}`, time.Now().Add(-5*time.Second).Format(time.RFC3339))

				res := postAnnouncement(annForm)
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_expiry"))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})
})
