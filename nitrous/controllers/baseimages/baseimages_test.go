package baseimages_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/baseimages")
}

var _ = Describe("controllers/baseimages", func() {
	var (
		db  *db.Database
		svr *server.Server
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/base_images", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewRequest("GET", "/api/v0/base_images", nil)
			Expect(err).To(BeNil())

			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		Context("when base images exist", func() {
			BeforeEach(func() {
				var err error
				_, err = db.Exec("INSERT INTO base_images (name, repo, description) values ($1, $2, $3);", "Go", "nitrousio/golang:latest", "")
				Expect(err).To(BeNil())
				_, err = db.Exec("INSERT INTO base_images (name, repo, description) values ($1, $2, $3);", "Node.js", "nitrousio/nodejs:latest", "")
				Expect(err).To(BeNil())
			})

			It("returns serialized base images in json", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					BaseImages []serializers.BaseImage `json:"base_images"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.BaseImages).To(HaveLen(2))

				Expect(j.BaseImages[0].Name).To(Equal("Go"))
				Expect(j.BaseImages[1].Name).To(Equal("Node.js"))
			})
		})

		It("returns an empty array when no base images exist", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				BaseImages []serializers.BaseImage `json:"base_images"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.BaseImages).To(HaveLen(0))
		})
	})
})
