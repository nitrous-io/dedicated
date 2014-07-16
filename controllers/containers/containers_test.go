package containers_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/containers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/containers")
}

var _ = Describe("controllers/containers", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		containers.FreighterClient = mockClient

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
				Name:      "foo",
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

	Describe("GET /api/v0/hosts/{host}/containers", func() {
		doRequest := func(token, host string) {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/hosts/%s/containers", host), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			Context("when host exists for user", func() {
				Context("when host has containers", func() {
					var (
						c         *container.Container
						fc        *freighterclient.Container
						touchedAt time.Time
					)

					BeforeEach(func() {
						c = &container.Container{
							CID:       "deadbeef",
							Name:      "dominant-intercooler-42",
							IPAddress: sql.NullString{"1.2.3.4", true},
							Image:     "nitrous-base",
							UserID:    u.ID,
							HostID:    hs.H.ID,
							LongName:  sql.NullString{"Rails app server", true},
						}
						err := c.Insert(db)
						Expect(err).To(BeNil())

						touchedAt = time.Date(2013, time.November, 10, 0, 0, 0, 0, time.UTC)
						c.TouchedAt = &touchedAt
						err = c.Update(db)
						Expect(err).To(BeNil())

						fc = &freighterclient.Container{
							ID:    "deadbeef",
							Name:  "dominant-intercooler-42",
							Host:  "1.2.3.4",
							State: "running",
							Image: "nitrous-base",
						}
						mockClient.Containers = []*freighterclient.Container{fc}
					})

					It("calls the Freighter client to list containers", func() {
						doRequest("valid-token", "foo")
						Expect(mockClient.ListCalled).To(Equal(1))
					})

					It("responds with the containers from Freighter and augments them with data from the database", func() {
						doRequest("valid-token", "foo")

						Expect(res.Code).To(Equal(http.StatusOK))

						var j map[string][]container.Container
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						containers, ok := j["containers"]
						Expect(ok).To(BeTrue())
						Expect(containers).To(HaveLen(1))

						// Fields from Freighter API.
						Expect(containers[0].CID).To(Equal("deadbeef"))
						Expect(containers[0].Image).To(Equal("nitrous-base"))
						Expect(containers[0].Name).To(Equal("dominant-intercooler-42"))
						Expect(containers[0].IPAddress.String).To(Equal("1.2.3.4"))
						Expect(containers[0].State).To(Equal("running"))

						// Fields from database.
						Expect(containers[0].LongName.String).To(Equal("Rails app server"))
						Expect(containers[0].TouchedAt.Unix()).To(Equal(touchedAt.Unix()))

						Expect(containers[0].UserID).To(Equal(u.ID))
						Expect(containers[0].HostID).To(Equal(hs.H.ID))
					})

					Context("when the Freighter client returns an error", func() {
						BeforeEach(func() {
							mockClient.ListError = errors.New("you are not Chuck Norris")
						})

						It("responds with HTTP 500 Internal Server Error", func() {
							doRequest("valid-token", "foo")

							Expect(res.Code).To(Equal(http.StatusInternalServerError))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("internal_server_error"))
						})
					})

					Context("when containers from Freighter differs from data in the database", func() {
						BeforeEach(func() {
							fc.Image = "new-image"
							fc.Name = "new-name-42"
							fc.Host = "2.4.6.8"
							fc.State = "stopped"
							mockClient.Containers = []*freighterclient.Container{fc}
						})

						It("prefers the data in the Freighter response", func() {
							doRequest("valid-token", "foo")

							Expect(res.Code).To(Equal(http.StatusOK))

							var j map[string][]container.Container
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())
							containers, ok := j["containers"]
							Expect(ok).To(BeTrue())
							Expect(containers).To(HaveLen(1))

							Expect(containers[0].CID).To(Equal("deadbeef"))
							Expect(containers[0].Image).To(Equal("new-image"))
							Expect(containers[0].Name).To(Equal("new-name-42"))
							Expect(containers[0].IPAddress.String).To(Equal("2.4.6.8"))
							Expect(containers[0].State).To(Equal("stopped"))
						})
					})

					Context("when there are more containers from Freighter than in the database", func() {
						BeforeEach(func() {
							fc2 := &freighterclient.Container{
								ID:    "cafebabe",
								Name:  "wicked-baby-69",
								Host:  "1.2.3.5",
								State: "running",
								Image: "nitrous-base",
							}
							mockClient.Containers = []*freighterclient.Container{fc, fc2}
						})

						It("responds with all the containers from Freighter", func() {
							doRequest("valid-token", "foo")

							Expect(res.Code).To(Equal(http.StatusOK))

							var j map[string][]container.Container
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())
							containers, ok := j["containers"]
							Expect(ok).To(BeTrue())
							Expect(containers).To(HaveLen(2))

							Expect(containers[0].CID).To(Equal("deadbeef"))
							Expect(containers[0].Image).To(Equal("nitrous-base"))
							Expect(containers[0].Name).To(Equal("dominant-intercooler-42"))
							Expect(containers[0].IPAddress.String).To(Equal("1.2.3.4"))
							Expect(containers[0].State).To(Equal("running"))
							Expect(containers[0].UserID).To(Equal(u.ID))
							Expect(containers[0].HostID).To(Equal(hs.H.ID))
							Expect(containers[0].LongName.String).To(Equal("Rails app server"))
							Expect(containers[0].TouchedAt.Unix()).To(Equal(touchedAt.Unix()))

							Expect(containers[1].CID).To(Equal("cafebabe"))
							Expect(containers[1].Image).To(Equal("nitrous-base"))
							Expect(containers[1].Name).To(Equal("wicked-baby-69"))
							Expect(containers[1].IPAddress.String).To(Equal("1.2.3.5"))
							Expect(containers[1].State).To(Equal("running"))
							Expect(containers[1].UserID).To(Equal(u.ID))
							Expect(containers[1].HostID).To(Equal(hs.H.ID))
							Expect(containers[1].LongName.String).To(Equal(""))
							Expect(containers[1].TouchedAt).To(BeNil())
						})
					})
				})

				Context("when host has no containers", func() {
					BeforeEach(func() {
						mockClient.Containers = []*freighterclient.Container{}
					})

					It("responds with an empty array", func() {
						doRequest("valid-token", "foo")

						Expect(res.Code).To(Equal(http.StatusOK))
						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						containers, ok := j["containers"].([]interface{})
						Expect(ok).To(BeTrue())
						Expect(containers).To(BeEmpty())
					})
				})

				Context("when host IP address is not available", func() {
					BeforeEach(func() {
						_, err := CreateHostStack(db, &HostStack{
							H: &host.Host{
								Name:      "bar",
								IPAddress: sql.NullString{},
							},
							U:  u,
							R:  hs.R,
							P:  hs.P,
							PT: hs.PT,
						})
						Expect(err).To(BeNil())
					})

					It("returns HTTP 400 Bad Request", func() {
						doRequest("valid-token", "bar")

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("invalid_request"))

						errDesc, _ := j["error_description"].(string)
						Expect(errDesc).To(Equal("host IP is unknown"))
					})
				})
			})

			Context("when host does not exist for user", func() {
				It("returns HTTP 404 Not Found", func() {
					doRequest("valid-token", "no-such-host")

					Expect(res.Code).To(Equal(http.StatusNotFound))

					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					errType, _ := j["error"].(string)
					Expect(errType).To(Equal("not_found"))

					errDesc, _ := j["error_description"].(string)
					Expect(errDesc).To(Equal("host does not exist"))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/containers"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{host}/containers", func() {
		var params *url.Values

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("name", "elixir-web-1")
			params.Set("longname", "Elixir Web Server 1")
			params.Set("image", "elixir")
		})

		doRequest := func(token, host string, params *url.Values) {
			req, err := NewFormRequest("POST", fmt.Sprintf("/api/v0/hosts/%s/containers", host), params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			Context("when valid params are given", func() {
				It("calls the Freighter client to create a container", func() {
					mockClient.CreateError = errors.New("create failed")

					doRequest("valid-token", "foo", params)

					Expect(mockClient.CreateCalled).To(Equal(1))
					Expect(mockClient.ReceivedName).To(Equal("elixir-web-1"))
					Expect(mockClient.ReceivedImage).To(Equal("elixir"))
				})

				Context("when the Freighter client returns an error", func() {
					BeforeEach(func() {
						mockClient.CreateError = errors.New("only Chuck Norris is allowed")
					})

					It("responds with HTTP 500 Internal Server Error", func() {
						doRequest("valid-token", "foo", params)

						Expect(res.Code).To(Equal(http.StatusInternalServerError))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("internal_server_error"))
					})

					Context("when the error is an InvalidParamError", func() {
						BeforeEach(func() {
							mockClient.CreateError = &freighterclient.InvalidParamError{"invalid name"}
						})

						It("responds with HTTP 400 Bad Request", func() {
							doRequest("valid-token", "foo", params)

							Expect(res.Code).To(Equal(http.StatusBadRequest))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("validation_failed"))
						})
					})
				})

				Context("when Freighter client returns a Container with no error", func() {
					BeforeEach(func() {
						mockClient.CreatedContainer = &freighterclient.Container{
							ID:    "deadbeef",
							Name:  "elixir-web-1",
							Host:  "1.2.3.4",
							State: "running",
							Image: "elixir",
						}
					})

					It("responds with HTTP 201 Created, the container and a status of created", func() {
						doRequest("valid-token", "foo", params)

						Expect(res.Code).To(Equal(http.StatusCreated))

						j := struct {
							Status    string               `json:"status"`
							Container *container.Container `json:"container"`
						}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						Expect(j.Status).To(Equal("created"))
						Expect(j.Container).NotTo(BeNil())
						Expect(j.Container.CID).To(Equal("deadbeef"))
						Expect(j.Container.Image).To(Equal("elixir"))
						Expect(j.Container.Name).To(Equal("elixir-web-1"))
						Expect(j.Container.IPAddress.String).To(Equal("1.2.3.4"))
						Expect(j.Container.State).To(Equal("running"))
						Expect(j.Container.LongName.String).To(Equal("Elixir Web Server 1"))
					})

					Context("when name is already taken", func() {
						BeforeEach(func() {
							c := &container.Container{
								CID:    "facecafe",
								Name:   "elixir-web-1",
								Image:  "nitrous-base",
								UserID: u.ID,
								HostID: hs.H.ID,
							}
							err := c.Insert(db)
							Expect(err).To(BeNil())
						})

						It("responds with HTTP 400 Bad Request", func() {
							doRequest("valid-token", "foo", params)

							Expect(res.Code).To(Equal(http.StatusBadRequest))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("validation_failed"))
							errDesc, _ := j["error_description"].(string)
							Expect(errDesc).To(Equal("name is taken"))
						})
					})

					Context("when longname param not given", func() {
						BeforeEach(func() {
							params.Del("longname")
						})

						It("sets long name to name", func() {
							doRequest("valid-token", "foo", params)

							Expect(res.Code).To(Equal(http.StatusCreated))

							j := struct {
								Status    string               `json:"status"`
								Container *container.Container `json:"container"`
							}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							Expect(j.Status).To(Equal("created"))
							Expect(j.Container.LongName.String).To(Equal("elixir-web-1"))
						})
					})

					Context("when image param is not given", func() {
						BeforeEach(func() {
							params.Del("image")
						})

						It("uses the default image", func() {
							doRequest("valid-token", "foo", params)

							Expect(res.Code).To(Equal(http.StatusCreated))

							j := struct {
								Status    string               `json:"status"`
								Container *container.Container `json:"container"`
							}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							Expect(j.Status).To(Equal("created"))
							Expect(j.Container.Image).To(Equal(containers.DefaultImage))
						})
					})
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/containers"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/hosts/{host}/containers/{name}", func() {
		var (
			host          = "foo"
			containerName = "elixir-web-1"
			token         = ""
		)

		doRequest := func() {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/hosts/%s/containers/%s", host, containerName), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			BeforeEach(func() {
				token = "valid-token"
			})

			Context("when host exists for user", func() {
				var (
					c  *container.Container
					fc *freighterclient.Container
				)

				BeforeEach(func() {
					c = &container.Container{
						CID:       "deadbeef",
						Name:      containerName,
						IPAddress: sql.NullString{"8.8.8.8", true},
						Image:     "nitrous-base",
						UserID:    u.ID,
						HostID:    hs.H.ID,
						LongName:  sql.NullString{"Elixir server", true},
					}
					err := c.Insert(db)
					Expect(err).To(BeNil())

					fc = &freighterclient.Container{
						ID:    "deadbeef",
						Name:  containerName,
						Host:  "1.2.3.4",
						State: "running",
						Image: "nitrous-base",
					}
					mockClient.Containers = []*freighterclient.Container{fc}
				})

				It("calls the Freighter client to get the container's info", func() {
					doRequest()
					Expect(mockClient.InfoCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
				})

				It("responds with the container from Freighter and augments it with data from the database", func() {
					doRequest()

					Expect(res.Code).To(Equal(http.StatusOK))

					var j map[string]container.Container
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					c, ok := j["container"]
					Expect(ok).To(BeTrue())

					// Fields from Freighter API.
					Expect(c.CID).To(Equal("deadbeef"))
					Expect(c.Image).To(Equal("nitrous-base"))
					Expect(c.Name).To(Equal(containerName))
					Expect(c.IPAddress.String).To(Equal("1.2.3.4"))
					Expect(c.State).To(Equal("running"))

					// Fields from database.
					Expect(c.LongName.String).To(Equal("Elixir server"))

					Expect(c.UserID).To(Equal(u.ID))
					Expect(c.HostID).To(Equal(hs.H.ID))
				})

				Context("when container does not exist", func() {
					BeforeEach(func() {
						containerName = "no-such-container"
					})

					It("returns HTTP 404 Not Found", func() {
						doRequest()

						Expect(res.Code).To(Equal(http.StatusNotFound))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("not_found"))
						errDesc, _ := j["error_description"].(string)
						Expect(errDesc).To(Equal("container does not exist"))
					})
				})

				Context("when the Freighter client returns an error", func() {
					BeforeEach(func() {
						mockClient.InfoError = errors.New("you are not Chuck Norris")
					})

					It("responds with HTTP 500 Internal Server Error", func() {
						doRequest()

						Expect(res.Code).To(Equal(http.StatusInternalServerError))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("internal_server_error"))
					})
				})
			})

			Context("when host does not exist for user", func() {
				BeforeEach(func() {
					host = "no-such-host"
				})

				It("returns HTTP 404 Not Found", func() {
					doRequest()

					Expect(res.Code).To(Equal(http.StatusNotFound))

					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					errType, _ := j["error"].(string)
					Expect(errType).To(Equal("not_found"))

					errDesc, _ := j["error_description"].(string)
					Expect(errDesc).To(Equal("host does not exist"))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/hosts/%s/containers/%s", host, containerName)
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("container control actions", func() {
		var (
			host          = "foo"
			containerName = "elixir-web-1"

			action        string
			actionCounter *int
			actionError   *error
			expectedState *string
		)

		doRequest := func(token, host, containerName string) {
			req, err := NewRequest("POST", fmt.Sprintf("/api/v0/hosts/%s/containers/%s/%s", host, containerName, action), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		itBehavesLikeAControlAction := func() {
			Context("when a valid oauth token is given", func() {
				Context("when host exists for user", func() {
					var (
						c  *container.Container
						fc *freighterclient.Container
					)

					BeforeEach(func() {
						c = &container.Container{
							CID:       "deadbeef",
							Name:      containerName,
							IPAddress: sql.NullString{"8.8.8.8", true},
							Image:     "nitrous-base",
							UserID:    u.ID,
							HostID:    hs.H.ID,
							LongName:  sql.NullString{"Elixir server", true},
						}
						err := c.Insert(db)
						Expect(err).To(BeNil())

						fc = &freighterclient.Container{
							ID:    "deadbeef",
							Name:  containerName,
							Host:  "1.2.3.4",
							State: "stopped",
							Image: "nitrous-base",
						}
						mockClient.Containers = []*freighterclient.Container{fc}
					})

					It("calls the Freighter client to perform the control action", func() {
						doRequest("valid-token", "foo", "elixir-web-1")
						Expect(*actionCounter).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
					})

					It("responds with the container from Freighter without data from the database", func() {
						doRequest("valid-token", "foo", "elixir-web-1")

						Expect(res.Code).To(Equal(http.StatusOK))

						var j map[string]container.Container
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())
						c, ok := j["container"]
						Expect(ok).To(BeTrue())

						// Fields from Freighter API.
						Expect(c.CID).To(Equal("deadbeef"))
						Expect(c.Image).To(Equal("nitrous-base"))
						Expect(c.Name).To(Equal(containerName))
						Expect(c.IPAddress.String).To(Equal("1.2.3.4"))
						Expect(c.State).To(Equal(*expectedState))

						// Fields from database.
						Expect(c.LongName.String).To(Equal(""))
						Expect(c.TouchedAt).To(BeNil())
						Expect(c.CreatedAt).To(BeNil())
						Expect(c.UpdatedAt).To(BeNil())

						Expect(c.UserID).To(Equal(u.ID))
						Expect(c.HostID).To(Equal(hs.H.ID))
					})

					Context("when container does not exist", func() {
						It("returns HTTP 404 Not Found", func() {
							doRequest("valid-token", "foo", "no-such-container")

							Expect(res.Code).To(Equal(http.StatusNotFound))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("not_found"))

							errDesc, _ := j["error_description"].(string)
							Expect(errDesc).To(Equal("container does not exist"))
						})
					})

					Context("when the Freighter client returns an error", func() {
						BeforeEach(func() {
							*actionError = errors.New("control action failed")
						})

						It("responds with HTTP 500 Internal Server Error", func() {
							doRequest("valid-token", "foo", "elixir-web-1")

							Expect(res.Code).To(Equal(http.StatusInternalServerError))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("internal_server_error"))
						})
					})
				})

				Context("when host does not exist for user", func() {
					It("returns HTTP 404 Not Found", func() {
						doRequest("valid-token", "no-such-host", "elixir-web-1")

						Expect(res.Code).To(Equal(http.StatusNotFound))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("not_found"))

						errDesc, _ := j["error_description"].(string)
						Expect(errDesc).To(Equal("host does not exist"))
					})
				})
			})

			Context("oauth token failure modes", func() {
				BeforeEach(func() {
					authTokenInputs.Path = fmt.Sprintf("/api/v0/hosts/%s/containers/%s/%s", host, containerName, action)
					authTokenInputs.Method = "POST"
				})

				ItRequiresAuthToken(&authTokenInputs)
			})
		}

		Describe("POST /api/v0/hosts/{host}/containers/{name}/start", func() {
			BeforeEach(func() {
				action = "start"
				actionCounter = &mockClient.StartCalled
				actionError = &mockClient.StartError
				state := "running"
				expectedState = &state
			})

			itBehavesLikeAControlAction()
		})

		Describe("POST /api/v0/hosts/{host}/containers/{name}/stop", func() {
			BeforeEach(func() {
				action = "stop"
				actionCounter = &mockClient.StopCalled
				actionError = &mockClient.StopError
				state := "stopped"
				expectedState = &state
			})

			itBehavesLikeAControlAction()
		})

		Describe("POST /api/v0/hosts/{host}/containers/{name}/restart", func() {
			BeforeEach(func() {
				action = "restart"
				actionCounter = &mockClient.RestartCalled
				actionError = &mockClient.RestartError
				state := "running"
				expectedState = &state
			})

			itBehavesLikeAControlAction()
		})
	})

	Describe("DELETE /hosts/{host}/containers/{name}", func() {
		var (
			host          = "foo"
			containerName = "elixir-web-1"
			token         = ""
		)

		doRequest := func() {
			req, err := NewRequest("DELETE", fmt.Sprintf("/api/v0/hosts/%s/containers/%s", host, containerName), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			BeforeEach(func() {
				token = "valid-token"
			})

			Context("when host exists for user", func() {
				var (
					c  *container.Container
					fc *freighterclient.Container
				)

				BeforeEach(func() {
					c = &container.Container{
						CID:       "deadbeef",
						Name:      containerName,
						IPAddress: sql.NullString{"8.8.8.8", true},
						Image:     "nitrous-base",
						UserID:    u.ID,
						HostID:    hs.H.ID,
						LongName:  sql.NullString{"Elixir server", true},
					}
					err := c.Insert(db)
					Expect(err).To(BeNil())

					fc = &freighterclient.Container{
						ID:    "deadbeef",
						Name:  containerName,
						Host:  "",
						State: "",
						Image: "nitrous-base",
					}
					mockClient.Containers = []*freighterclient.Container{fc}
				})

				It("calls the Freighter client to Terminate the container", func() {
					doRequest()
					Expect(mockClient.TerminateCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
				})

				It("responds with the container from Freighter and augments it with data from the database", func() {
					doRequest()

					Expect(res.Code).To(Equal(http.StatusOK))

					var j map[string]container.Container
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					c, ok := j["container"]
					Expect(ok).To(BeTrue())

					// Fields from Freighter API.
					Expect(c.CID).To(Equal("deadbeef"))
					Expect(c.Image).To(Equal("nitrous-base"))
					Expect(c.Name).To(Equal(containerName))
					Expect(c.IPAddress.String).To(Equal(""))
					Expect(c.State).To(Equal(""))

					// Fields from database.
					Expect(c.LongName.String).To(Equal("Elixir server"))
					Expect(c.UpdatedAt).NotTo(BeNil())

					Expect(c.UserID).To(Equal(u.ID))
					Expect(c.HostID).To(Equal(hs.H.ID))
				})

				It("deletes the container from the database", func() {
					doRequest()

					c, err := container.FindByHostIDAndName(db, hs.H.ID, containerName)
					Expect(err).To(BeNil())
					Expect(c).To(BeNil())
				})

				Context("when container does not exist", func() {
					BeforeEach(func() {
						containerName = "no-such-container"
					})

					It("returns HTTP 404 Not Found", func() {
						doRequest()

						Expect(res.Code).To(Equal(http.StatusNotFound))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("not_found"))

						errDesc, _ := j["error_description"].(string)
						Expect(errDesc).To(Equal("container does not exist"))
					})
				})

				Context("when the Freighter client returns an error", func() {
					BeforeEach(func() {
						mockClient.TerminateError = errors.New("only Chuck Norris can terminate containers")
					})

					It("responds with HTTP 500 Internal Server Error", func() {
						doRequest()

						Expect(res.Code).To(Equal(http.StatusInternalServerError))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("internal_server_error"))
					})
				})
			})

			Context("when host does not exist for user", func() {
				BeforeEach(func() {
					host = "no-such-host"
				})

				It("returns HTTP 404 Not Found", func() {
					doRequest()

					Expect(res.Code).To(Equal(http.StatusNotFound))

					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					errType, _ := j["error"].(string)
					Expect(errType).To(Equal("not_found"))

					errDesc, _ := j["error_description"].(string)
					Expect(errDesc).To(Equal("host does not exist"))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/hosts/%s/containers/%s", host, containerName)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
