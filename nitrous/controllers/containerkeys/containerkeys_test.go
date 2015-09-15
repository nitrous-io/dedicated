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
	"github.com/nitrous-io/nitrous-common/sshkey"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/containerkeys")
}

var _ = Describe("controllers/containerkeys", func() {
	type getGitHubKeysFunc func(cn *container.Container, h *host.Host, env *middleware.Env) ([]string, error)
	type removeGitHubKeyFunc func(cn *container.Container, h *host.Host, env *middleware.Env) error

	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs test.AuthTokenInputs

		mockClient *freighterclient.MockClient

		u  *user.User
		t  *token.Token
		hs *test.HostStack
		c  *container.Container

		key0   = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDATOQiODD9fe8IrqVuQKCN/8PyqCXOYrREqpmsi4Jx3m4VdwP0IwrmOylvkDYGG7HVOHKvlncwrIkDJKXlyplaqs6O5D+daY1JLuosqckGPzBlOFchdUjuqiDxdLPBEfOYr2ANZjfLjAyuGqsNAXwhn5+LHKXiXg7Le6feuEsZhP0JyAnEg9diLcmyv8f2rOPqM3L2SmZ38GbWJm1rO7+LVUmroNNa1X1UJHyQFyNOEtXHbYuoZ0xEui6UDJU/DAoVxSfTRzg9XKUWArTsKeGRmJoQBmRRtoFVe8kbs1qnJQCnHnkyH6BhoqFZ/wl7YNRvsGAta1Q1qiqLFSOYZvbT"
		fn0, _ = sshkey.Md5Fingerprint(key0)
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		test.StubDefaultFreighterFn(mockClient)

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)
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
			mockClient.ListContainerKeysReturn = []*freighterclient.ContainerKey{
				&freighterclient.ContainerKey{Key: key0, Fingerprint: fn0},
			}
			doRequest("valid-token")
			Expect(res.Code).To(Equal(http.StatusOK))

			j := struct {
				Keys []freighterclient.ContainerKey `json:"keys"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Keys).To(HaveLen(1))
			Expect(j.Keys[0].Key).To(Equal(key0))
			Expect(j.Keys[0].Fingerprint).To(Equal(fn0))
		})

		Context("when container is not running", func() {
			It("should return container not running error", func() {
				mockClient.ListContainerKeysError = freighterclient.ErrContainerNotRunning
				doRequest("valid-token")
				Expect(res.Code).To(Equal(http.StatusConflict))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("container_not_running"))
				Expect(errDesc).To(Equal("container should be running to perform this operation"))
			})
		})

		Context("when pub key ops are not supported", func() {
			It("should return pub key ops are not supported error", func() {
				mockClient.ListContainerKeysError = freighterclient.ErrPubKeyOpsNotSupported
				doRequest("valid-token")
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("pub_key_ops_not_supported"))
				Expect(errDesc).To(Equal("public key operations are not supported"))
			})
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
				_, err := db.Exec("UPDATE user_profiles SET github_oauth_token_json = '{}' WHERE user_id = $1", u.ID)
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

		Context("when some other error occurs", func() {
			It("responds with an internal server error", func() {
				containerkeys.AddGitHubKeysFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) {
					return "", errors.New("some error occurred")
				}

				doRequest()
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})

	Describe("GET /api/v0/containers/{container}/container_keys/github", func() {
		var originalGetGitHubKeysFunc getGitHubKeysFunc
		doRequest := func() {
			uri := fmt.Sprintf("/api/v0/containers/%s/container_keys/github", c.Slug)
			req, err := test.NewAuthedRequest(t, "GET", uri, nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			originalGetGitHubKeysFunc = containerkeys.GetGitHubKeysFunc
		})

		AfterEach(func() {
			containerkeys.GetGitHubKeysFunc = originalGetGitHubKeysFunc
		})

		It("returns container key that is added to github", func() {
			containerkeys.GetGitHubKeysFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) ([]string, error) {
				return []string{key0}, nil
			}
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := struct {
				Keys []string `json:"keys"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Keys).To(HaveLen(1))
			Expect(j.Keys[0]).To(Equal(key0))
		})

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
				_, err := db.Exec("UPDATE user_profiles SET github_oauth_token_json = '{}' WHERE user_id = $1", u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("github_token_not_valid"))
				Expect(errDesc).To(Equal("GitHub token is not valid"))
			})
		})
	})

	Describe("DELETE /api/v0/containers/{container}/container_keys/github", func() {
		var originalRemoveFromGitHubFunc removeGitHubKeyFunc
		var removeGitHubKeyFuncCalled int

		doRequest := func() {
			uri := fmt.Sprintf("/api/v0/containers/%s/container_keys/github", c.Slug)
			req, err := test.NewAuthedRequest(t, "DELETE", uri, nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			removeGitHubKeyFuncCalled = 0
			originalRemoveFromGitHubFunc = containerkeys.RemoveFromGitHubFunc
			mockClient.ListContainerKeysReturn = []*freighterclient.ContainerKey{
				&freighterclient.ContainerKey{Key: key0, Fingerprint: fn0},
			}
		})

		AfterEach(func() {
			containerkeys.RemoveFromGitHubFunc = originalRemoveFromGitHubFunc
		})

		It("remove specfied key from github", func() {
			containerkeys.RemoveFromGitHubFunc = func(cn *container.Container, h *host.Host, env *middleware.Env) error {
				removeGitHubKeyFuncCalled++
				return nil
			}
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(removeGitHubKeyFuncCalled).To(Equal(1))
		})

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
				_, err := db.Exec("UPDATE user_profiles SET github_oauth_token_json = '{}' WHERE user_id = $1", u.ID)
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("github_token_not_valid"))
				Expect(errDesc).To(Equal("GitHub token is not valid"))
			})
		})
	})
})
