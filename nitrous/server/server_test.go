package server_test

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "server")
}

var _ = Describe("server/debug", func() {
	var (
		svr     *server.Server
		db      *db.Database
		handler *mux.Router
		params  *url.Values

		originalTrust = envvars.TrustKey
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)

		handler = svr.Router()
		params = &url.Values{}
		envvars.TrustKey = "super_secret"
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		envvars.TrustKey = originalTrust
	})

	Describe("GET /api/v0/debug", func() {
		doRequest := func(url string) *httptest.ResponseRecorder {
			req, err := NewRequest("GET", url, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should show debug info if trust key is valid", func() {
			params.Set("trust_key", "super_secret")
			res := doRequest("/api/v0/debug")
			Expect(res.Code).To(Equal(200))

			res = doRequest("/api/v0/debug/pprof/heap")
			Expect(res.Code).To(Equal(200))
		})

		It("should show 404 if no trust key provided", func() {
			res := doRequest("/api/v0/debug")
			Expect(res.Code).To(Equal(404))

			res = doRequest("/api/v0/debug/pprof/heap")
			Expect(res.Code).To(Equal(404))
		})

		It("should show 404 if invalid trust key provided", func() {
			params.Set("trust_key", "not_this")
			res := doRequest("/api/v0/debug")
			Expect(res.Code).To(Equal(404))

			res = doRequest("/api/v0/debug/pprof/heap")
			Expect(res.Code).To(Equal(404))
		})
	})
})
