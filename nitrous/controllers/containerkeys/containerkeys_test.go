package containerkeys_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containerkeys"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/containerkeys")
}

var _ = Describe("controllers/containerkeys", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs test.AuthTokenInputs

		mockClient *freighterclient.MockClient

		u    *user.User
		t    *token.Token
		hs   *test.HostStack
		c    *container.Container
		key0 string = "sample container key"
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		test.StubDefaultFreighterFn(mockClient)

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
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

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
			U: u,
		})
		Expect(err).To(BeNil())

		c = &container.Container{
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		test.UnstubFreighterFn()
	})

	Describe("GET /api/v0/containers/{container}/container_keys", func() {
		BeforeEach(func() {
			mockClient.ListContainerKeysReturn = nil
		})

		doRequest := func(token string) {
			uri := fmt.Sprintf("/api/v0/containers/%s/container_keys", c.Slug)
			req, err := test.NewAPIRequest(token, "GET", uri, nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return container keys", func() {
			mockClient.ListContainerKeysReturn = []string{key0}
			doRequest("valid-token")
			Expect(res.Code).To(Equal(http.StatusOK))
			j := struct {
				Keys []string `json:"keys"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Keys).To(HaveLen(1))
			Expect(j.Keys[0]).To(Equal(key0))
		})
	})

	Describe("POST /api/v0/containers/{container}/container_keys/github", func() {
		doRequest := func() {
			uri := fmt.Sprintf("/api/v0/containers/%s/container_keys/github", c.Slug)
			req, err := test.NewAuthedRequest(t, "POST", uri, nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		Context("when user hasn't authenticated with GitHub", func() {
			It("responds with an error saying user to connect GitHub", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("github_token_empty"))
				Expect(errDesc).To(Equal("connect your GitHub account"))
			})
		})

		Context("when user's github token is not valid", func() {
			It("responds with token not valid", func() {
				_, err := db.Exec("UPDATE user_profiles SET github_oauth_token_json = '{}' WHERE id = $1", u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("github_token_not_valid"))
				Expect(errDesc).To(Equal("GitHub token is not valid"))
			})
		})

		Context("when the key already exists in GitHub", func() {
			It("responds with key already exists", func() {
				containerkeys.AddGitHubKeysFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) {
					return "", containerkeys.ErrKeyAlreadyExists
				}

				doRequest()
				Expect(res.Code).To(Equal(http.StatusConflict))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("key_already_exists"))
				Expect(errDesc).To(Equal("key already exists"))
			})
		})

		Context("when user has multiple container keys", func() {
			It("responds with multiple container keys exist", func() {
				containerkeys.AddGitHubKeysFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) {
					return "", containerkeys.ErrMultipleKeysExist
				}

				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				err, errDesc := test.ErrorSlug(res.Body)
				Expect(err).To(Equal("multiple_keys_exist"))
				Expect(errDesc).To(Equal("multiple container keys exist"))
			})
		})

		It("returns ok when key is added", func() {
			containerkeys.AddGitHubKeysFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) {
				return "github key added", nil
			}

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))
			j := struct {
				Msg string `json:"msg"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Msg).To(Equal("github key added"))
		})

		Context("when some other error occurrs", func() {
			It("responds with an internal server error", func() {
				containerkeys.AddGitHubKeysFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) {
					return "", errors.New("some error occurred")
				}

				doRequest()
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})
})
