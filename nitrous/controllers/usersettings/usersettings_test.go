package usersettings_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
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
		svr = test.NewTestServer("", db, nil)
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

		It("responds with the user's settings as well as defaults", func() {
			_, err := db.Exec(`UPDATE user_settings SET ide_settings = '{"tab_size": 3}'::JSON WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				UserSettings map[string]interface{} `json:"user_settings"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.UserSettings["tab_size"]).To(Equal(float64(3)))
			Expect(j.UserSettings["match_brackets"]).To(BeTrue())
		})
	})

	Describe("POST /api/v0/user/settings", func() {
		doRequest := func(jsonStr string) *httptest.ResponseRecorder {
			req, err := test.NewBodyRequest("POST", "/api/v0/user/settings", jsonStr)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.Token))
			req.Header.Set("Content-type", "application/json")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("updates the user's settings and responds with the new settings on toggle", func() {
			_, err := db.Exec(`UPDATE user_settings SET ide_settings = '{"tab_size": 9}'::JSON WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			jsonStr1 := `{
                                "tab_size": 3,
                                "auto_close_brackets": false,
                                "remove_trailing_whitespace_on_save": false,
                                "keybindings": "vim",
                                "ruler_column": 120,
                                "theme": "solarized light"
                        }`

			res1 := doRequest(jsonStr1)
			Expect(res1.Code).To(Equal(http.StatusOK))

			j1 := struct {
				UserSettings map[string]interface{} `json:"user_settings"`
			}{}
			err = json.NewDecoder(res1.Body).Decode(&j1)
			Expect(err).To(BeNil())
			serialized := j1.UserSettings
			Expect(serialized["tab_size"]).To(Equal(float64(3)))
			Expect(serialized["auto_close_brackets"]).To(BeFalse())
			Expect(serialized["remove_trailing_whitespace_on_save"]).To(BeFalse())
			Expect(serialized["keybindings"]).To(Equal("vim"))
			Expect(serialized["ruler_column"]).To(Equal(float64(120)))
			Expect(serialized["theme"]).To(Equal("solarized light"))

			settings, err := u.Settings(db)
			Expect(err).To(BeNil())
			m, err := settings.IdeSettingsMap()
			Expect(err).To(BeNil())
			Expect(m["tab_size"]).To(Equal(float64(3)))
			Expect(m["auto_close_brackets"]).To(BeFalse())
			Expect(m["remove_trailing_whitespace_on_save"]).To(BeFalse())
			Expect(m["keybindings"]).To(Equal("vim"))
			Expect(m["ruler_column"]).To(Equal(float64(120)))
			Expect(m["theme"]).To(Equal("solarized light"))

			jsonStr2 := `{
                                "tab_size": 4,
                                "auto_close_brackets": true,
                                "remove_trailing_whitespace_on_save": true,
                                "keybindings": "default",
                                "ruler_column": 150,
                                "theme": "solarized dark"
                        }`

			res2 := doRequest(jsonStr2)
			Expect(res2.Code).To(Equal(http.StatusOK))

			j2 := struct {
				UserSettings map[string]interface{} `json:"user_settings"`
			}{}
			err = json.NewDecoder(res2.Body).Decode(&j2)
			Expect(err).To(BeNil())
			serialized = j2.UserSettings
			Expect(serialized["tab_size"]).To(Equal(float64(4)))
			Expect(serialized["auto_close_brackets"]).To(BeTrue())
			Expect(serialized["remove_trailing_whitespace_on_save"]).To(BeTrue())
			Expect(serialized["keybindings"]).To(Equal("default"))
			Expect(serialized["ruler_column"]).To(Equal(float64(150)))
			Expect(serialized["theme"]).To(Equal("solarized dark"))

			settings, err = u.Settings(db)
			Expect(err).To(BeNil())
			m, err = settings.IdeSettingsMap()
			Expect(err).To(BeNil())
			Expect(m["tab_size"]).To(Equal(float64(4)))
			Expect(m["auto_close_brackets"]).To(BeTrue())
			Expect(m["remove_trailing_whitespace_on_save"]).To(BeTrue())
			Expect(m["keybindings"]).To(Equal("default"))
			Expect(m["ruler_column"]).To(Equal(float64(150)))
			Expect(m["theme"]).To(Equal("solarized dark"))
		})

		It("should handle legacy params sent by older IDE clients", func() {
			settings, err := u.Settings(db)
			Expect(err).To(BeNil())
			Expect(settings.AutoCloseBrackets).To(BeTrue())
			Expect(settings.Keybindings).To(Equal("default"))
			Expect(settings.RulerColumn).To(Equal(int64(0)))
			Expect(settings.Theme).To(Equal("solarized dark"))

			params := &url.Values{}
			params.Set("tab_size", "3")
			params.Set("auto_close_brackets", "false")
			params.Set("remove_trailing_whitespace_on_save", "false")
			params.Set("keybindings", "vim")
			params.Set("ruler_column", "120")
			params.Set("theme", "visual studio")

			req, err := test.NewAuthedFormRequest(t, "POST", "/api/v0/user/settings", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				UserSettings map[string]interface{} `json:"user_settings"`
			}{}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			serialized := j.UserSettings
			Expect(serialized["tab_size"]).To(Equal(float64(3)))
			Expect(serialized["auto_close_brackets"]).To(BeFalse())
			Expect(serialized["remove_trailing_whitespace_on_save"]).To(BeFalse())
			Expect(serialized["keybindings"]).To(Equal("vim"))
			Expect(serialized["ruler_column"]).To(Equal(float64(120)))
			Expect(serialized["theme"]).To(Equal("visual studio"))

			settings, err = u.Settings(db)
			Expect(err).To(BeNil())
			m, err := settings.IdeSettingsMap()
			Expect(err).To(BeNil())
			Expect(m["tab_size"]).To(Equal(float64(3)))
			Expect(m["auto_close_brackets"]).To(BeFalse())
			Expect(m["remove_trailing_whitespace_on_save"]).To(BeFalse())
			Expect(m["keybindings"]).To(Equal("vim"))
			Expect(m["ruler_column"]).To(Equal(float64(120)))
			Expect(m["theme"]).To(Equal("visual studio"))
		})
	})
})
