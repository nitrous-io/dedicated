package appsettings_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/appsettings")
}

var _ = Describe("controllers/appsettings", func() {
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

	Describe("GET /api/v0/app_settings", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewRequest("GET", "/api/v0/app_settings", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("returns serialized settings in json", func() {
			oldMT := envvars.MixpanelToken
			envvars.MixpanelToken = "Track Me"
			defer func() { envvars.MixpanelToken = oldMT }()

			oldSK := envvars.StripePublicKey
			envvars.StripePublicKey = "st-001"
			defer func() { envvars.StripePublicKey = oldSK }()

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j["mixpanel_token"]).To(Equal("Track Me"))
			Expect(j["stripe_key"]).To(Equal("st-001"))
		})
	})
})
