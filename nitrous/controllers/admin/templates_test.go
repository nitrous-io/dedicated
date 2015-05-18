package admin_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("controllers/admin", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		authTokenInputs test.AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr := server.NewServer("", db, logger, nil, &dns.FakeProvider{})
		authTokenInputs = test.AuthTokenInputs{Handler: svr.Router()}

		handler = svr.Router()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

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

	Describe("POST /api/v0/admin/templates", func() {
		var (
			err error
		)

		doRequest := func(t *token.Token, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "POST", "/api/v0/admin/templates", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(t, nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("responds with created template on a successful request", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusCreated))
				var j struct {
					Template *serializers.Template `json:"template"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				tmpl := j.Template
				Expect(tmpl.ID).ToNot(BeZero())
				Expect(tmpl.Slug).To(Equal("rails-stack"))
				Expect(tmpl.Name).To(Equal("Ruby on Rails"))
				Expect(tmpl.Description).To(Equal("A Rails stack template"))
				Expect(tmpl.Image).To(Equal("ubuntu:18.04"))
				Expect(tmpl.Privileged).To(Equal(true))
				Expect(tmpl.Type).To(Equal("stack"))
			})

			It("responds with 400 bad request if slug is empty", func() {
				p := &url.Values{}
				p.Set("slug", "")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("slug required"))
			})

			It("responds with 400 bad request if name is empty", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("name required"))
			})

			It("responds with 400 bad request if image is empty", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("image required"))
			})

			It("responds with 400 bad request if type is invalid", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "some-other")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("invalid type"))
			})

			It("responds with 400 bad request if config is invalid", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "invalid_config" }`)
				p.Set("type", "stack")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("config invalid"))
			})

			It("responds with 500 server error for other errors", func() {
				db.Close()

				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, p)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/admin/templates"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/admin/template/{slug}", func() {
		var (
			tmpl *template.Template
			err  error
		)

		doRequest := func(t *token.Token, slug string) *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/admin/template/"+slug, nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			tmpl = &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Image:       "ubuntu:18.04",
				Config:      `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`,
				Type:        template.TypeStack,
			}
			err := tmpl.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(t, tmpl.Slug)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("responds with matched template by slug", func() {
				res := doRequest(t, tmpl.Slug)

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Template *serializers.AdminTemplate `json:"template"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				serialized := j.Template
				Expect(serialized.ID).ToNot(BeZero())
				Expect(serialized.Slug).To(Equal(tmpl.Slug))
				Expect(serialized.Name).To(Equal(tmpl.Name.String))
				Expect(serialized.Description).To(Equal(tmpl.Description.String))
				Expect(serialized.Image).To(Equal(tmpl.Image))
				Expect(serialized.Type).To(Equal(tmpl.Type))
				Expect(serialized.Config).To(Equal(tmpl.Config))
			})

			It("responds with 404 not found if invalid slug is given", func() {
				res := doRequest(t, "cobol")

				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("template not found"))
			})

			It("responds with 500 server error for other errors", func() {
				db.Close()

				res := doRequest(t, tmpl.Slug)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/admin/template/rails"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("PUT /api/v0/admin/template/{slug}", func() {
		var (
			tmpl *template.Template
			err  error
		)

		doRequest := func(t *token.Token, slug string, params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "PUT", "/api/v0/admin/template/"+slug, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			tmpl = &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Image:       "ubuntu:18.04",
				Config:      `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`,
				Type:        template.TypeStack,
			}
			err := tmpl.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when the requester is not an admin", func() {
			It("responds with 404 Not Found", func() {
				res := doRequest(t, "", nil)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				_, err = db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())
			})

			It("responds with updated template on a successful request", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack-2")
				p.Set("name", "Ruby on Rails (new)")
				p.Set("description", "A Rails stack template (new)")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Template *serializers.Template `json:"template"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				tmpl := j.Template
				Expect(tmpl.ID).ToNot(BeZero())
				Expect(tmpl.Slug).To(Equal("rails-stack-2"))
				Expect(tmpl.Name).To(Equal("Ruby on Rails (new)"))
				Expect(tmpl.Description).To(Equal("A Rails stack template (new)"))
				Expect(tmpl.Image).To(Equal("ubuntu:18.04"))
				Expect(tmpl.Privileged).To(Equal(true))
				Expect(tmpl.Type).To(Equal("stack"))
			})

			It("responds with 400 bad request if slug is empty", func() {
				p := &url.Values{}
				p.Set("slug", "")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("slug required"))
			})

			It("responds with 400 bad request if name is empty", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("name required"))
			})

			It("responds with 400 bad request if image is empty", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("image required"))
			})

			It("responds with 400 bad request if type is invalid", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "some-other")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("invalid type"))
			})

			It("responds with 400 bad request if config is invalid", func() {
				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "invalid_config" }`)
				p.Set("type", "stack")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("config invalid"))
			})

			It("responds with 500 server error for other errors", func() {
				db.Close()

				p := &url.Values{}
				p.Set("slug", "rails-stack")
				p.Set("name", "Ruby on Rails")
				p.Set("description", "A Rails stack template")
				p.Set("image", "ubuntu:18.04")
				p.Set("config", `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true, "privileged": true }`)
				p.Set("type", "stack")
				res := doRequest(t, tmpl.Slug, p)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/admin/template/test"
				authTokenInputs.Method = "PUT"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
