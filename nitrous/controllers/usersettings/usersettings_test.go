package usersettings_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
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
	RunSpecs(t, "controllers/usersettings")
}

var _ = Describe("controllers/userprofiles", func() {
	var (
		db      *ndb.Database
		svr     *server.Server
		handler *mux.Router

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)
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

	Describe("GET /api/v0/user/settings", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/user/settings", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("responds with user's settings serialized as JSON'", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				UserSettings serializers.UserSettings `json:"user_settings"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.UserSettings.UserID).To(Equal(u.ID))

			settings, err := u.Settings(db)
			Expect(err).To(BeNil())
			Expect(j.UserSettings.ID).To(Equal(settings.ID))
			Expect(j.UserSettings.AutoCloseBrackets).To(Equal(settings.AutoCloseBrackets))
		})
	})

	Describe("POST /api/v0/user/settings", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			req, err := test.NewAuthedFormRequest(t, "POST", "/api/v0/user/settings", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("updates the user's settings and responds with the new settings", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())
			Expect(settings.AutoCloseBrackets).To(BeTrue())
			Expect(settings.Keybindings).To(Equal("default"))
			Expect(settings.RulerColumn).To(Equal(int64(0)))
			Expect(settings.Theme).To(Equal("solarized dark"))

			params := &url.Values{}
			params.Set("auto_close_brackets", "false")
			params.Set("keybindings", "vim")
			params.Set("ruler_column", "120")
			params.Set("theme", "solarized light")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				UserSettings serializers.UserSettings `json:"user_settings"`
			}{}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			serialized := j.UserSettings
			Expect(serialized.UserID).To(Equal(u.ID))
			Expect(serialized.AutoCloseBrackets).To(BeFalse())
			Expect(serialized.Keybindings).To(Equal("vim"))
			Expect(serialized.RulerColumn).To(Equal(int64(120)))
			Expect(serialized.Theme).To(Equal("solarized light"))

			settings, err = u.Settings(db)
			Expect(err).To(BeNil())
			Expect(settings.AutoCloseBrackets).To(BeFalse())
			Expect(settings.Keybindings).To(Equal("vim"))
			Expect(settings.RulerColumn).To(Equal(int64(120)))
			Expect(settings.Theme).To(Equal("solarized light"))
		})
	})
})
