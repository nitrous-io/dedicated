package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server/routes")
}

var _ = Describe("server/routes", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router
		res     *httptest.ResponseRecorder
	)
	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("invalid route", func() {
		It("should return 404 not found", func() {
			routes := map[string]string{
				"GET": "/api/v1/path",
			}
			for m, r := range routes {
				req, err := NewRequest(m, r, nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			}
		})
	})

	Describe("valid routes", func() {
		routes := [][]string{
			[]string{"POST", "/oauth/token"},

			[]string{"POST", "/api/v0/users"},
			[]string{"GET", "/api/v0/user"},

			[]string{"GET", "/api/v0/hosts"},
			[]string{"POST", "/api/v0/hosts"},
		}
		for _, r := range routes {
			func(method, path string) {
				Context(fmt.Sprintf("%v %v", method, path), func() {
					It("should not return 404 not found", func() {
						req, err := NewRequest(method, path, nil)
						Expect(err).To(BeNil())
						handler.ServeHTTP(res, req)
						Expect(res.Code).NotTo(Equal(http.StatusNotFound))
					})
				})
			}(r[0], r[1])
		}
	})
})
