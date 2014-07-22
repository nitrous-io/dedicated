package registry_test

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/registry"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/registry")
}

var _ = Describe("controllers/registry", func() {
	var (
		db     *db.Database
		svr    *server.Server
		res    *httptest.ResponseRecorder
		params *url.Values
		env    *middlewares.Env

		u  *user.User
		hs *HostStack

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		registry.FreighterClient = mockClient

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
			U: u,
		})
		Expect(err).To(BeNil())

		env = &middlewares.Env{User: u}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Login", func() {
		doRequest := func(slug string) error {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/registries", params)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"slug": slug}
			return registry.Login(env, svr, res, req)
		}

		Context("with a valid host available for user", func() {
			Context("when username is not provided", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("username", "")
					params.Set("password", "123456")
					params.Set("email", "dockeruser@example.com")
					params.Set("registry", "http://hub.example.com")
				})

				It("returns username required error", func() {
					err := doRequest("foo")
					Expect(err).To(Equal(registry.ErrUsernameRequired))
				})
			})

			Context("when password is not provided", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("username", "dockeruser")
					params.Set("password", "")
					params.Set("email", "dockeruser@example.com")
					params.Set("registry", "http://hub.example.com")
				})

				It("returns password required error", func() {
					err := doRequest("foo")
					Expect(err).To(Equal(registry.ErrPasswordRequired))
				})
			})

			Context("when email is not provided", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("username", "dockeruser")
					params.Set("password", "123456")
					params.Set("email", "")
					params.Set("registry", "http://hub.example.com")
				})

				It("returns email required error", func() {
					err := doRequest("foo")
					Expect(err).To(Equal(registry.ErrEmailRequired))
				})
			})

			Context("when all params are provided", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("username", "dockeruser")
					params.Set("password", "123456")
					params.Set("email", "dockeruser@example.com")
					params.Set("registry", "http://hub.example.com")
				})

				It("calls the Freighter client with the given params to login to the registry", func() {
					err := doRequest("foo")
					Expect(err).To(BeNil())

					Expect(mockClient.RegistryLoginCalled).To(Equal(1))
					Expect(mockClient.ReceivedUsername).To(Equal("dockeruser"))
					Expect(mockClient.ReceivedPassword).To(Equal("123456"))
					Expect(mockClient.ReceivedEmail).To(Equal("dockeruser@example.com"))
					Expect(mockClient.ReceivedRegistry).To(Equal("http://hub.example.com"))
				})

				It("returns HTTP 200 OK if logged in successfully", func() {
					mockClient.RegistryLoginError = nil
					err := doRequest("foo")
					Expect(err).To(BeNil())
					Expect(res.Code).To(Equal(http.StatusOK))
				})

				It("returns error if failed to login", func() {
					mockClient.RegistryLoginError = errors.New("some error occurred")
					err := doRequest("foo")
					Expect(err.Error()).To(Equal("some error occurred"))
				})
			})
		})

		Context("with an invalid host", func() {
			BeforeEach(func() {
				params = &url.Values{}
			})

			It("Returns host not found error", func() {
				err := doRequest("nanana-8928")
				Expect(err).To(Equal(registry.ErrHostNotFound))
			})
		})
	})

	Describe("Logout", func() {
		doRequest := func(slug string) error {
			req, err := NewRequest("DELETE", "/api/v0/hosts/"+slug+"/registries", params)
			Expect(err).To(BeNil())
			env.Vars = map[string]string{"slug": slug}

			return registry.Logout(env, svr, res, req)
		}

		Context("with a valid host available for user", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("registry", "http://hub.example.com")
			})

			It("calls the Freighter client with the given registry to logout", func() {
				err := doRequest("foo")
				Expect(err).To(BeNil())

				Expect(mockClient.RegistryLogoutCalled).To(Equal(1))
				Expect(mockClient.ReceivedRegistry).To(Equal("http://hub.example.com"))
			})

			It("returns HTTP 200 OK if logged in successfully", func() {
				mockClient.RegistryLogoutError = nil
				err := doRequest("foo")
				Expect(err).To(BeNil())
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("returns error if failed to login", func() {
				mockClient.RegistryLogoutError = errors.New("some error occurred")
				err := doRequest("foo")
				Expect(err.Error()).To(Equal("some error occurred"))
			})
		})

		Context("with an invalid host", func() {
			BeforeEach(func() {
				params = &url.Values{}
			})

			It("Returns host not found error", func() {
				err := doRequest("nanana-8928")
				Expect(err).To(Equal(registry.ErrHostNotFound))
			})
		})
	})
})
