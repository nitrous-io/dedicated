package templates_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/templates")
}

var _ = Describe("controllers/templates", func() {
	var (
		db  *db.Database
		svr *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil, &dns.FakeProvider{})
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/templates", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewRequest("GET", "/api/v0/templates", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with all templates", func() {
			t1 := &template.Template{
				Slug:        "rails-stack",
				Name:        sql.NullString{"Ruby on Rails", true},
				Description: sql.NullString{"A Rails stack template", true},
				Config:      `{ "containers": [ { "name": "Rails", "slug": "rails", "image": "rails:4.2" } ] }`,
				Type:        template.TypeStack,
			}
			err := t1.Insert(db)
			Expect(err).To(BeNil())
			t2 := &template.Template{
				Slug:        "php-stack",
				Name:        sql.NullString{"PHP", true},
				Description: sql.NullString{"A PHP stack template", true},
				Config:      `{ "containers": [ { "name": "php", "slug": "php", "image": "php:latest" } ] }`,
				Type:        template.TypeStack,
			}
			err = t2.Insert(db)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Templates []serializers.Template `json:"templates"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Templates).To(HaveLen(2))

			Expect(j.Templates[0].Slug).To(Equal("rails-stack"))
			Expect(j.Templates[1].Slug).To(Equal("php-stack"))
		})

		Context("when there are no templates", func() {
			It("responds with an empty array", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Templates []serializers.Template `json:"templates"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Templates).To(BeEmpty())
			})
		})
	})
})
