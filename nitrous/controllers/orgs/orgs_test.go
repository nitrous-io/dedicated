package orgs_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/orgs")
}

var _ = Describe("controllers/orgs", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
		params  *url.Values

		authTokenInputs test.AuthTokenInputs
		u               *user.User
		t               *token.Token
		ft              *test.FakeTracker
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)
		svr.Tracker = ft
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
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

	Describe("POST /api/v0/orgs", func() {
		doRequest := func() {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/orgs", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("email", "Accounting@Nitrous.io")
			params.Set("name", "Nitrous, Inc.")
			params.Set("slug", "nitRous")
		})

		Context("when valid params are received", func() {
			BeforeEach(func() {
				doRequest()
			})

			It("returns 201 Created and creates an org in the db", func() {
				Expect(res.Code).To(Equal(http.StatusCreated))
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())
				Expect(o).NotTo(BeNil())
				Expect(o.ID).NotTo(BeZero())
				Expect(o.Email).To(Equal("accounting@nitrous.io"))
				Expect(o.Name).To(Equal("Nitrous, Inc."))
				Expect(o.Slug).To(Equal("nitrous"))
				Expect(o.CreatedAt).NotTo(BeNil())
				Expect(o.DeletedAt.Valid).To(BeFalse())
			})

			It("responds with a json representation of the org that's been created", func() {
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())

				Expect(len(res.Body.Bytes())).NotTo(BeZero())
				var j struct {
					Org *serializers.Org `json:"org"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Org.ID).To(Equal(o.ID))
				Expect(j.Org.Email).To(Equal("accounting@nitrous.io"))
				Expect(j.Org.Name).To(Equal("Nitrous, Inc."))
				Expect(j.Org.Slug).To(Equal("nitrous"))
			})

			It("adds the user as the owner of the org", func() {
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgId(db, o.ID)
				Expect(err).To(BeNil())

				Expect(m).To(HaveLen(1))
				Expect(m[0].Username).To(Equal("foo"))
				Expect(m[0].Role).To(Equal("owner"))
			})
		})

		Context("when invalid params are received", func() {
			type tc struct {
				param   string
				value   string
				message string
			}
			tcs := []tc{
				{"slug", "", org.ErrSlugBlank.Error()},
				{"slug", "!@$@!$", org.ErrSlugInvalid.Error()},
				{"email", "", org.ErrEmailBlank.Error()},
				{"email", "not.a.valid.email@", org.ErrEmailInvalid.Error()},
				{"email", "hello@yopmail.com", org.ErrEmailBlacklisted.Error()},
				{"name", "", org.ErrNameBlank.Error()},
			}

			for _, t := range tcs {
				func(t tc) {
					Context(fmt.Sprintf(`when "%s" param is "%s"`, t.param, t.value), func() {
						BeforeEach(func() {
							if t.value == "" {
								params.Del(t.param)
							} else {
								params.Set(t.param, t.value)
							}
							doRequest()
						})

						It("responds with 400 bad request", func() {
							Expect(res.Code).To(Equal(http.StatusBadRequest))
							var j map[string]string
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())
							Expect(j["error"]).To(Equal("validation_failed"))
							Expect(j["error_description"]).To(Equal(t.message))
						})
					})
				}(t)
			}
		})

		Context("when a banned email address is provided", func() {
			BeforeEach(func() {
				b := &user.User{
					Username: "banner",
					Password: "123456",
					Email:    "banner@example.com",
				}
				err := test.WrapInTransaction(db, b.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO users (username, email, encrypted_password, deleted_at, banned_at, banned_by_id) VALUES ('foo', 'foo@example.com', '!@#$%', now(), now(), $1)", b.ID)
				Expect(err).To(BeNil())

				params.Set("email", "foo@example.com")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				var j map[string]string
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("email_banned"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
