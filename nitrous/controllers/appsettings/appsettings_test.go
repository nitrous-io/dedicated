package appsettings_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
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
		svr = server.NewServer("", db, logger, nil, &dns.FakeProvider{})
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/app_settings", func() {
		var t *token.Token
		var j map[string]interface{}

		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewRequest("GET", "/api/v0/app_settings", nil)
			if t != nil {
				req.Header.Set("Authorization", "Bearer "+t.Token)
			}
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		itReturnsPublicSettings := func() {
			oldFV := envvars.FreighterdVersion
			envvars.FreighterdVersion = "version-21-2"
			defer func() { envvars.FreighterdVersion = oldFV }()

			oldMT := envvars.MixpanelToken
			envvars.MixpanelToken = "Track Me"
			defer func() { envvars.MixpanelToken = oldMT }()

			oldSK := envvars.StripePublicKey
			envvars.StripePublicKey = "st-001"
			defer func() { envvars.StripePublicKey = oldSK }()

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j = map[string]interface{}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j["mixpanel_token"]).To(Equal("Track Me"))
			Expect(j["stripe_key"]).To(Equal("st-001"))
		}

		It("returns serialized settings in json", func() {
			itReturnsPublicSettings()
			Expect(j["freighterd_version"]).To(BeNil())
		})

		It("should return serialized settings if the token is invalid", func() {
			t = &token.Token{
				Token: "Bla",
			}
			itReturnsPublicSettings()
			Expect(j["freighterd_version"]).To(BeNil())
		})

		Describe("with a valid user", func() {
			var u *user.User
			BeforeEach(func() {
				u = &user.User{
					Username: "foo",
					Password: "123456",
					Name:     "foo",
					Email:    "foo@example.com",
				}
				err := WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				t = &token.Token{
					UserID: u.ID,
					Token:  "valid-token",
				}
				err = t.Insert(db)
				Expect(err).To(BeNil())
			})

			It("should return serialized settings if the user is not an admin", func() {
				itReturnsPublicSettings()
				Expect(j["freighterd_version"]).To(BeNil())
			})

			It("should return freighterd_version if the user is an admin", func() {
				_, err := db.Exec("UPDATE users SET admin = true WHERE id = $1", u.ID)
				Expect(err).To(BeNil())

				itReturnsPublicSettings()
				Expect(j["freighterd_version"]).To(Equal("version-21-2"))
			})

		})
	})
})
