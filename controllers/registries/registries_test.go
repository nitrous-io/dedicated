package registries_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/registries"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/registry"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/registries")
}

var _ = Describe("controllers/registries", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		svr             *server.Server
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		u  *user.User
		t  *token.Token
		hs *HostStack

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	type errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	type overrides map[string]string

	unmarshalErrorResponse := func() (error, *errorResponse) {
		errRes := &errorResponse{}
		err := json.Unmarshal(res.Body.Bytes(), errRes)
		return err, errRes
	}

	ItRespondsHostNotFound := func(doRequest func(overrides)) {
		It("responds host not found error", func() {
			doRequest(overrides{"slug": "nanana-8928"})

			Expect(res.Code).To(Equal(http.StatusNotFound))

			err, j := unmarshalErrorResponse()
			Expect(err).To(BeNil())

			Expect(j.Error).To(Equal("not_found"))
			Expect(j.ErrorDescription).To(Equal("host does not exist"))
		})
	}

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		registries.FreighterClient = mockClient

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
			U: u,
		})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/hosts/{slug}/registries", func() {
		Context("when a valid oauth token is given", func() {

			doRequest := func(o overrides) {
				var ok bool

				slug := "foo"
				if _, ok = o["slug"]; ok {
					slug = o["slug"]
					delete(o, "slug")
				}

				if _, ok = o["username"]; !ok {
					o["username"] = "dockeruser"
				}

				if _, ok = o["password"]; !ok {
					o["password"] = "123456"
				}

				if _, ok = o["email"]; !ok {
					o["email"] = "dockeruser@example.com"
				}

				if _, ok = o["registry"]; !ok {
					o["registry"] = "hub.example.com"
				}

				params := &url.Values{}
				for k, v := range o {
					params.Set(k, v)

				}

				req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/registries", params)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				handler.ServeHTTP(res, req)
			}

			Context("with a valid host available for user", func() {
				Context("when username is not provided", func() {
					It("returns username required error", func() {
						doRequest(overrides{"username": ""})

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("invalid_request"))
						Expect(j.ErrorDescription).To(Equal("username is required"))
					})
				})

				Context("when password is not provided", func() {
					It("returns password required error", func() {
						doRequest(overrides{"password": ""})

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("invalid_request"))
						Expect(j.ErrorDescription).To(Equal("password is required"))
					})
				})

				Context("when email is not provided", func() {
					It("returns email required error", func() {
						doRequest(overrides{"email": ""})

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("invalid_request"))
						Expect(j.ErrorDescription).To(Equal("email is required"))
					})
				})

				Context("when all params are provided", func() {
					It("calls the Freighter client with the given params to login to the registry", func() {
						doRequest(overrides{})

						Expect(mockClient.RegistryLoginCalled).To(Equal(1))
						Expect(mockClient.ReceivedUsername).To(Equal("dockeruser"))
						Expect(mockClient.ReceivedPassword).To(Equal("123456"))
						Expect(mockClient.ReceivedEmail).To(Equal("dockeruser@example.com"))
						Expect(mockClient.ReceivedRegistry).To(Equal("hub.example.com"))
					})

					Context("when registry login succeeds", func() {
						BeforeEach(func() {
							mockClient.RegistryLoginError = nil
							doRequest(overrides{})
						})

						It("creates a registry record for the user and host", func() {
							regions, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
							Expect(err).To(BeNil())
							Expect(len(regions)).To(Equal(1))

							r := regions[0]
							Expect(r.Username).To(Equal("dockeruser"))
							Expect(r.Email).To(Equal("dockeruser@example.com"))
							Expect(r.RegistryURL).To(Equal("hub.example.com"))
						})

						It("returns HTTP 200 OK if logged in successfully", func() {
							Expect(res.Code).To(Equal(http.StatusOK))
						})
					})

					Context("when registry login fails", func() {
						BeforeEach(func() {
							mockClient.RegistryLoginError = freighterclient.ErrInvalidParams
							doRequest(overrides{})
						})

						It("creates a registry record for the user and host", func() {
							regions, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
							Expect(err).To(BeNil())
							Expect(len(regions)).To(Equal(0))
						})

						It("informs user if login failed", func() {
							Expect(res.Code).To(Equal(http.StatusBadRequest))

							err, j := unmarshalErrorResponse()
							Expect(err).To(BeNil())

							Expect(j.Error).To(Equal("login_failed"))
							Expect(j.ErrorDescription).To(Equal("registry login failed"))
						})
					})

					It("returns internal server error for other errors", func() {
						mockClient.RegistryLoginError = errors.New("some other error")
						doRequest(overrides{})

						Expect(res.Code).To(Equal(http.StatusInternalServerError))
					})
				})
			})

			Context("with an invalid host", func() {
				ItRespondsHostNotFound(doRequest)
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/registries"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}/registries", func() {
		doRequest := func(o overrides) {
			slug := "foo"
			if _, ok := o["slug"]; ok {
				slug = o["slug"]
			}

			r := "hub.example.com"
			if _, ok := o["registry"]; ok {
				r = o["registry"]
			}

			params := &url.Values{}
			params.Set("registry", r)

			req, err := NewRequest("DELETE", "/api/v0/hosts/"+slug+"/registries", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			r := &registry.Registry{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				RegistryURL: "hub.example.com",
				Username:    "foo",
				Email:       "foo@example.com",
			}
			err := r.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("with a valid host available for user", func() {
				It("calls the Freighter client with the given registry to logout", func() {
					doRequest(overrides{})

					Expect(mockClient.RegistryLogoutCalled).To(Equal(1))
					Expect(mockClient.ReceivedRegistry).To(Equal("hub.example.com"))
				})

				Context("when log out is successful", func() {
					BeforeEach(func() {
						mockClient.RegistryLogoutError = nil
						doRequest(overrides{})
					})

					It("deletes the registry record", func() {
						rx, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
						Expect(err).To(BeNil())
						Expect(rx).To(HaveLen(0))
					})

					It("returns HTTP 200 OK if logged out successfully", func() {
						Expect(res.Code).To(Equal(http.StatusOK))
					})
				})

				Context("when log out fails", func() {
					BeforeEach(func() {
						mockClient.RegistryLogoutError = freighterclient.ErrInvalidParams
						doRequest(overrides{})
					})

					It("does not delete the registry record", func() {
						rx, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
						Expect(err).To(BeNil())
						Expect(rx).To(HaveLen(1))
					})

					It("informs user if logout failed", func() {
						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("logout_failed"))
						Expect(j.ErrorDescription).To(Equal("registry logout failed"))
					})
				})

				It("returns internal server error for other errors", func() {
					mockClient.RegistryLogoutError = errors.New("some other error")
					doRequest(overrides{})

					Expect(res.Code).To(Equal(http.StatusInternalServerError))
				})
			})

			Context("with an invalid host", func() {
				ItRespondsHostNotFound(doRequest)
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/registries"
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
