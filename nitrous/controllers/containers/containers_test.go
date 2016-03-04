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
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/usercontainerdata"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/hmactoken"
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

		originalHostHostname string
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = NewTestServer("", db, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
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

		originalHostHostname = envvars.HostHostname
		envvars.HostHostname = "n2odev.com"
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		UnstubFreighterFn()
		envvars.HostHostname = originalHostHostname
	})

	Describe("WithContainerAndHost", func() {
		var (
			env           *middleware.Env
			handler       *mux.Router
			c             *container.Container
			allowCollab   bool
			blockFreeUser bool
			handlerCalled bool
		)

		BeforeEach(func() {
			env = &middleware.Env{
				DB:   db,
				User: u,
			}

			handlerCalled = false
			allowCollab = false
			blockFreeUser = false

			h := func(w http.ResponseWriter, r *http.Request) {
				opts := []containers.WithContainerAndHostOpt{}
				if allowCollab == true {
					opts = append(opts, containers.AllowCollab)
				}
				if blockFreeUser == true {
					opts = append(opts, containers.BlockFreeUser)
				}
				mw := containers.WithContainerAndHost(func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
					Expect(cn).NotTo(BeNil())
					Expect(h).NotTo(BeNil())
					Expect(h.State).To(Equal(host.StateRunning))
					handlerCalled = true
					return nil
				}, opts...)
				err := mw(env, w, r)
				if err != nil {
					svr.HandleError(env, w, r, err)
				}
			}
			handler = mux.NewRouter()
			handler.HandleFunc("/{slug}/action", h).Methods("GET")
		})

		Context("when there is no container with the given slug", func() {
			It("returns a not found error", func() {
				req, err := NewRequest("GET", "/missing-container/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(handlerCalled).To(BeFalse())

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("container_not_found"))
				errDesc, _ := j["error_description"].(string)
				Expect(errDesc).To(Equal("container does not exist"))
			})
		})

		Context("when a container with the given slug exists", func() {
			BeforeEach(func() {
				c = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())

				handlerCalled = false
			})

			It("runs the wrapped handler", func() {
				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(handlerCalled).To(BeTrue())
			})

			It("it should return a forbidden if the handler blocks free users", func() {
				blockFreeUser = true

				_, err := db.Exec(`INSERT INTO billing_profiles (state, user_id) VALUES ('free', $1) ;`, u.ID)
				Expect(err).To(BeNil())

				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusForbidden))
				_, errDesc := ErrorSlug(res.Body)
				Expect(errDesc).To(Equal("only paid users have permission to access this section"))
				Expect(handlerCalled).To(BeFalse())
			})

			It("it should return an internal server error if the handler blocks free users but there is no billing profile record", func() {
				blockFreeUser = true

				_, err := db.Exec(`DELETE FROM billing_profiles where user_id = $1;`, u.ID)
				Expect(err).To(BeNil())

				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				Expect(handlerCalled).To(BeFalse())
			})
		})

		Context("when a container is shared with a user", func() {
			var owner *user.User

			BeforeEach(func() {
				owner = &user.User{
					Username: "owner",
					Email:    "owner@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, owner.Insert)
				Expect(err).To(BeNil())

				c = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: owner.ID,
					HostID: hs.H.ID,
				}
				err = c.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, u.ID)
				Expect(err).To(BeNil())
				handlerCalled = false
			})

			It("it should not run if the handler does not permit sharing", func() {
				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(handlerCalled).To(BeFalse())
			})

			It("it should run if the handler does permit sharing", func() {
				allowCollab = true

				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(handlerCalled).To(BeTrue())
			})

			It("it should not run if the handler does permit sharing but isn't shared", func() {
				allowCollab = true

				_, err := db.Exec(`DELETE FROM container_collabs WHERE user_id = $1;`, u.ID)
				Expect(err).To(BeNil())

				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(handlerCalled).To(BeFalse())
			})

			It("it should return a forbidden if the handler permits sharing but blocks free users", func() {
				allowCollab = true
				blockFreeUser = true

				_, err := db.Exec(`INSERT INTO billing_profiles (state, user_id) VALUES ('free', $1) ;`, owner.ID)
				Expect(err).To(BeNil())

				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusForbidden))
				_, errDesc := ErrorSlug(res.Body)
				Expect(errDesc).To(Equal("only paid users have permission to access this section"))
				Expect(handlerCalled).To(BeFalse())
			})

			It("it should return an internal server error if the handler permits sharing but blocks free users and there is no billing profile record", func() {
				allowCollab = true
				blockFreeUser = true

				_, err := db.Exec(`DELETE FROM billing_profiles where user_id = $1;`, owner.ID)
				Expect(err).To(BeNil())

				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				Expect(handlerCalled).To(BeFalse())
			})
		})
	})

	Describe("GET /api/v0/containers", func() {
		var (
			c         *container.Container
			fc        *freighterclient.Container
			touchedAt time.Time

			startedAt = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func(token, host string) {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/containers?host=%s", host), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "dominant-intercooler-42",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Rails app server", true},
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"C": {"1", true},
					},
				},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			touchedAt = time.Date(2013, time.November, 10, 0, 0, 0, 0, time.UTC)
			c.TouchedAt = &touchedAt
			err = c.Update(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       "dominant-intercooler-42",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				User:       "guest",
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		It("should allow the user even if it has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("valid-token", "foo")
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("calls the Freighter client to list containers", func() {
			doRequest("valid-token", "foo")
			Expect(mockClient.ListCalled).To(Equal(1))
		})

		It("responds with the containers from Freighter and augments them with data from the database", func() {
			doRequest("valid-token", "foo")

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.Container `json:"containers"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(1))
			Expect(j.Containers[0].ID).To(Equal(c.ID))
			Expect(j.Containers[0].CID).To(Equal("deadbeef"))
			Expect(j.Containers[0].UserID).To(Equal(u.ID))
			Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
			Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
			Expect(j.Containers[0].IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Containers[0].GatewayIP).To(Equal("1.2.3.0"))
			Expect(j.Containers[0].State).To(Equal("running"))
			Expect(j.Containers[0].Name).To(Equal("Rails app server"))
			Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
			Expect(j.Containers[0].Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(j.Containers[0].Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(j.Containers[0].ExitCode).To(Equal(0))
			Expect(j.Containers[0].User).To(Equal("guest"))
			Expect(j.Containers[0].Metadata["C"]).To(Equal("1"))
			Expect(j.Containers[0].StartedAt).To(Equal(&startedAt))
			Expect(j.Containers[0].FinishedAt).To(BeNil())
			Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
			Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
			Expect(j.Containers[0].TerminatedAt).To(BeNil())
		})

		It("keeps the list ordered by slug", func() {
			secondaryContainer := &freighterclient.Container{
				ID:         "deadbeefy",
				Name:       "dominant-intercooler-43",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{secondaryContainer, fc}

			doRequest("valid-token", "foo")

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.Container `json:"containers"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(2))
			Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
			Expect(j.Containers[1].Slug).To(Equal("dominant-intercooler-43"))
		})

		It("should replace the image name if it is not a reconfig", func() {
			fc.Image = "a-new-image"

			doRequest("valid-token", "foo")

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.Container `json:"containers"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(1))
			Expect(j.Containers[0].Image).To(Equal("a-new-image"))
		})

		Context("when the Freighter client returns a freighter error", func() {
			BeforeEach(func() {
				mockClient.ListError = errors.New("freighter: you are not Chuck Norris")
			})

			It("lets the request continue but returns a list of errors", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Errors []string `json:"errors"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				// the frontend explicitly matches on this error; if
				// expectation below is changed, fix the frontend matcher as
				// well
				Expect(j.Errors).To(ConsistOf("Unable to communicate with freighter for host foo"))
			})
		})

		Context("when the Freighter client returns a generic error", func() {
			BeforeEach(func() {
				mockClient.ListError = errors.New("you are not Chuck Norris")
			})

			It("lets the request continue but returns a list of errors", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Errors []string `json:"errors"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j.Errors).To(ConsistOf("Unable to locate containers for host foo"))
			})
		})

		Context("when the Freighter client returns no error", func() {
			BeforeEach(func() {
				mockClient.ListError = nil
				mockClient.Containers = []*freighterclient.Container{}
			})

			It("it should not include the errors key in the body", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j).ToNot(HaveKey("errors"))
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

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].Image).To(Equal("new-image"))
				Expect(j.Containers[0].Slug).To(Equal("new-name-42"))
				Expect(j.Containers[0].IPAddress).To(Equal("2.4.6.8"))
				Expect(j.Containers[0].State).To(Equal("stopped"))
			})
		})

		Context("when there are more containers from Freighter than in the database", func() {
			BeforeEach(func() {
				fc2 := &freighterclient.Container{
					ID:         "cafebabe",
					Name:       "wicked-baby-69",
					Host:       "1.2.3.5",
					Gateway:    "1.2.3.0",
					State:      "running",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					ExitCode:   0,
					User:       "wheel",
					StartedAt:  startedAt,
					FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
				}
				mockClient.Containers = []*freighterclient.Container{fc, fc2}
			})

			It("responds with all the containers from Freighter", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(2))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
				Expect(j.Containers[0].IPAddress).To(Equal("1.2.3.4"))
				Expect(j.Containers[0].GatewayIP).To(Equal("1.2.3.0"))
				Expect(j.Containers[0].State).To(Equal("running"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].Cmd).To(Equal([]string{"/sbin/init"}))
				Expect(j.Containers[0].Entrypoint).To(Equal([]string{"/bin/blah"}))
				Expect(j.Containers[0].ExitCode).To(Equal(0))
				Expect(j.Containers[0].User).To(Equal("guest"))
				Expect(j.Containers[0].StartedAt).To(Equal(&startedAt))
				Expect(j.Containers[0].FinishedAt).To(BeNil())
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())

				Expect(j.Containers[1].ID).To(Equal(int64(0))) // Container exists in Freighter but not in DB.
				Expect(j.Containers[1].CID).To(Equal("cafebabe"))
				Expect(j.Containers[1].UserID).To(Equal(u.ID))
				Expect(j.Containers[1].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[1].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[1].Slug).To(Equal("wicked-baby-69"))
				Expect(j.Containers[1].IPAddress).To(Equal("1.2.3.5"))
				Expect(j.Containers[1].GatewayIP).To(Equal("1.2.3.0"))
				Expect(j.Containers[1].State).To(Equal("running"))
				Expect(j.Containers[1].Name).To(Equal(""))
				Expect(j.Containers[1].Cmd).To(Equal([]string{"/sbin/init"}))
				Expect(j.Containers[1].Entrypoint).To(Equal([]string{"/bin/blah"}))
				Expect(j.Containers[1].ExitCode).To(Equal(0))
				Expect(j.Containers[1].User).To(Equal("wheel"))
				Expect(j.Containers[1].StartedAt).To(Equal(&startedAt))
				Expect(j.Containers[1].FinishedAt).To(BeNil())
				Expect(j.Containers[1].CreatedAt).To(BeNil())
				Expect(j.Containers[1].UpdatedAt).To(BeNil())
				Expect(j.Containers[1].TerminatedAt).To(BeNil())
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

		Context("when host is stopped", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("should return containers belonging to the host (without the freighter-hydrated details)", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())
				// ip address, gateway will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateHostStopped.String()))

				Expect(j.Containers[0].Cmd).To(BeNil())
				Expect(j.Containers[0].Entrypoint).To(BeNil())
				Expect(j.Containers[0].ExitCode).To(Equal(0))
				Expect(j.Containers[0].User).To(Equal(""))
				Expect(j.Containers[0].StartedAt).To(BeNil())
				Expect(j.Containers[0].FinishedAt).To(BeNil())
			})
		})

		Context("when host is not running but not stopped", func() {
			BeforeEach(func() {
				hs.H.State = host.StateRestarting
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("should return containers belonging to the host with unknown state", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateUnknown.String()))
			})
		})

		Context("when host IP address is not available", func() {
			var c1 *container.Container
			var hs1 *HostStack
			var err error

			BeforeEach(func() {
				hs1, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:      "bar",
						IPAddress: sql.NullString{},
					},
					U:  u,
					R:  hs.R,
					P:  hs.P,
					PT: hs.PT,
				})
				Expect(err).To(BeNil())

				c1 = &container.Container{
					CID:    "cafedead",
					Slug:   "forever-container-21",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs1.H.ID,
					Name:   sql.NullString{"Rails app server", true},
				}
				err = c1.Insert(db)
				Expect(err).To(BeNil())

				// test the actual client
				UnstubFreighterFn()
			})

			It("should return all containers belonging to the host (without the freighter-hydrated details)", func() {
				doRequest("valid-token", "bar")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].ID).To(Equal(c1.ID))
				Expect(j.Containers[0].CID).To(Equal("cafedead"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs1.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("forever-container-21"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())
				// ip address, gateway will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateHostStopped.String()))

				Expect(j.Containers[0].Cmd).To(BeNil())
				Expect(j.Containers[0].Entrypoint).To(BeNil())
				Expect(j.Containers[0].ExitCode).To(Equal(0))
				Expect(j.Containers[0].User).To(Equal(""))
				Expect(j.Containers[0].StartedAt).To(BeNil())
				Expect(j.Containers[0].FinishedAt).To(BeNil())
			})
		})

		Context("when host param is not provided", func() {
			var c1 *container.Container
			var hs1 *HostStack
			var err error

			BeforeEach(func() {
				hs1, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:      "bar",
						IPAddress: sql.NullString{},
					},
					U:  u,
					R:  hs.R,
					P:  hs.P,
					PT: hs.PT,
				})
				Expect(err).To(BeNil())

				c1 = &container.Container{
					CID:    "cafedead",
					Slug:   "forever-container-21",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs1.H.ID,
					Name:   sql.NullString{"Rails app server", true},
				}
				err = c1.Insert(db)
				Expect(err).To(BeNil())

				// We are updating the state here to be stopped
				// so that there are no Freighter API calls
				// the way we stub the Freighter API right now
				// doesnt' allow us to get different responses
				// for different API requests (similar to stub chain)
				hs.H.State = host.StateStopped
				err = hs.H.Update(db)
				Expect(err).To(BeNil())

				hs1.H.State = host.StateStopped
				err = hs1.H.Update(db)
				Expect(err).To(BeNil())

				UnstubFreighterFn()
			})

			It("should return all containers belonging to the users' hosts", func() {
				doRequest("valid-token", "")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(2))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())
				// ip address, gateway will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateHostStopped.String()))

				Expect(j.Containers[1].ID).To(Equal(c1.ID))
				Expect(j.Containers[1].CID).To(Equal("cafedead"))
				Expect(j.Containers[1].UserID).To(Equal(u.ID))
				Expect(j.Containers[1].HostID).To(Equal(hs1.H.ID))
				Expect(j.Containers[1].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[1].Slug).To(Equal("forever-container-21"))
				Expect(j.Containers[1].Name).To(Equal("Rails app server"))
				Expect(j.Containers[1].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[1].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[1].TerminatedAt).To(BeNil())
				// ip address, gateway state will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[1].IPAddress).To(Equal(""))
				Expect(j.Containers[1].GatewayIP).To(Equal(""))
				Expect(j.Containers[1].State).To(Equal(container.StateHostStopped.String()))
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
				Expect(errType).To(Equal("host_not_found"))

				errDesc, _ := j["error_description"].(string)
				Expect(errDesc).To(Equal("host does not exist"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/containers"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("fetchContainersWithHosts()", func() {
		var (
			receiver *user.User
			c        *container.Container
		)

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "dominant-intercooler-42",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Rails app server", true},
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"C": {"1", true},
					},
				},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			c2 := &container.Container{
				CID:    "invisible",
				Slug:   "invisible-machine",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
			}
			err = c2.Insert(db)
			Expect(err).To(BeNil())

			receiver = &user.User{
				Username: "guest",
				Email:    "guest@example.com",
				Password: "123456",
			}
			err = WrapInTransaction(db, receiver.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, receiver.ID)
			Expect(err).To(BeNil())
		})

		It("should return nil if there is nothing shared", func() {
			res, err := containers.FetchContainersWithHosts(db, u.ID)
			Expect(err).To(BeNil())
			Expect(res).To(BeNil())
		})

		It("should return the container map and hosts", func() {
			res, err := containers.FetchContainersWithHosts(db, receiver.ID)
			Expect(err).To(BeNil())
			Expect(res).ToNot(BeNil())
			Expect(res.Hosts[0].Host).To(Equal(*hs.H))
			Expect(res.Hosts[0].UserUsername).To(Equal(u.Username))
			Expect(res.Hosts[0].UserName).To(Equal(u.Name))
			Expect(res.Hosts[0].GravatarHash).To(Equal("b48def645758b95537d4424c84d1a9ff"))
			Expect(res.HostContainers[hs.H.ID]).To(ConsistOf(c))
		})

		It("should not include hosts that are terminated", func() {
			_, err := db.Exec(`UPDATE hosts SET terminated_at = now() WHERE id = $1;`, hs.H.ID)
			Expect(err).To(BeNil())

			res, err := containers.FetchContainersWithHosts(db, receiver.ID)
			Expect(err).To(BeNil())
			Expect(res).ToNot(BeNil())
			Expect(res.Hosts).To(BeEmpty())
		})

		It("should not include hosts that are orphaned", func() {
			_, err := db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2;`, host.StateOrphan, hs.H.ID)
			Expect(err).To(BeNil())

			res, err := containers.FetchContainersWithHosts(db, receiver.ID)
			Expect(err).To(BeNil())
			Expect(res).ToNot(BeNil())
			Expect(res.Hosts).To(BeEmpty())
		})

		It("should not include hosts that are migrating", func() {
			_, err := db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2;`, host.StateMigrating, hs.H.ID)
			Expect(err).To(BeNil())

			res, err := containers.FetchContainersWithHosts(db, receiver.ID)
			Expect(err).To(BeNil())
			Expect(res).ToNot(BeNil())
			Expect(res.Hosts).To(BeEmpty())
		})
	})

	Describe("GET /api/v0/shared_containers", func() {
		var (
			receiver      *user.User
			receiverToken *token.Token
			c             *container.Container
			fc            *freighterclient.Container
			startedAt     = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func() {
			req, err := NewRequest("GET", "/api/v0/shared_containers", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+receiverToken.Token)
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "dominant-intercooler-42",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Rails app server", true},
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"C": {"1", true},
					},
				},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			c2 := &container.Container{
				CID:    "invisible",
				Slug:   "invisible-machine",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
			}
			err = c2.Insert(db)
			Expect(err).To(BeNil())

			receiver = &user.User{
				Username: "guest",
				Email:    "guest@example.com",
				Password: "123456",
			}
			err = WrapInTransaction(db, receiver.Insert)
			Expect(err).To(BeNil())

			receiverToken = &token.Token{
				UserID: receiver.ID,
				Token:  "ABC",
			}
			err = receiverToken.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, receiver.ID)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       "dominant-intercooler-42",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				User:       "guest",
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			fc2 := &freighterclient.Container{
				ID:         "invisible",
				Name:       "invisible-machine-v3",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				User:       "guest",
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc, fc2}
		})

		It("should return an empty list when no containers are shared", func() {
			_, err := db.Exec(`DELETE FROM container_collabs WHERE user_id = $1;`, receiver.ID)
			Expect(err).To(BeNil())

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.SharedContainer `json:"containers"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(0))
		})

		It("should not return the container if it has been terminated", func() {
			_, err := db.Exec(`UPDATE containers SET terminated_AT = now() WHERE id = $1;`, c.ID)
			Expect(err).To(BeNil())

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.SharedContainer `json:"containers"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(0))
		})

		It("should not return the container if its host has been terminated", func() {
			_, err := db.Exec(`UPDATE hosts SET terminated_AT = now() WHERE id = $1;`, c.HostID)
			Expect(err).To(BeNil())

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.SharedContainer `json:"containers"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(0))
		})

		It("should not return containers from deleted users", func() {
			_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, c.UserID)
			Expect(err).To(BeNil())

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.SharedContainer `json:"containers"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(0))
		})

		It("should only return the shared containers with host info", func() {
			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.SharedContainer `json:"containers"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(1))

			Expect(j.Containers[0].CID).To(Equal("deadbeef"))
			Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
			Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
			Expect(j.Containers[0].IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Containers[0].State).To(Equal("running"))
			Expect(j.Containers[0].UserUsername).To(Equal("foo"))
			Expect(j.Containers[0].UserGravatarHash).To(Equal("b48def645758b95537d4424c84d1a9ff"))
			Expect(j.Containers[0].Host.ID).To(Equal(hs.H.ID))
		})

		It("should handle multiple hosts", func() {
			hs1, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:      "bar",
					IPAddress: sql.NullString{},
				},
				U:  u,
				R:  hs.R,
				P:  hs.P,
				PT: hs.PT,
			})
			Expect(err).To(BeNil())

			c1 := &container.Container{
				CID:    "cafedead",
				Slug:   "forever-container-21",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs1.H.ID,
				Name:   sql.NullString{"Rails app server", true},
			}
			err = c1.Insert(db)
			Expect(err).To(BeNil())

			hs1.H.State = host.StateStopped
			err = hs1.H.Update(db)
			Expect(err).To(BeNil())

			hs.H.State = host.StateStopped
			err = hs.H.Update(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c1.ID, receiver.ID)
			Expect(err).To(BeNil())

			UnstubFreighterFn()
			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.SharedContainer `json:"containers"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(2))

			Expect(j.Containers[0].CID).To(Equal("deadbeef"))
			Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
			Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
			Expect(j.Containers[0].IPAddress).To(Equal(""))
			Expect(j.Containers[0].State).To(Equal("host_stopped"))
			Expect(j.Containers[0].UserUsername).To(Equal("foo"))
			Expect(j.Containers[0].UserGravatarHash).To(Equal("b48def645758b95537d4424c84d1a9ff"))

			Expect(j.Containers[1].CID).To(Equal("cafedead"))
			Expect(j.Containers[1].Image).To(Equal("nitrous-base"))
			Expect(j.Containers[1].Slug).To(Equal("forever-container-21"))
			Expect(j.Containers[1].IPAddress).To(Equal(""))
			Expect(j.Containers[1].State).To(Equal("host_stopped"))
			Expect(j.Containers[1].UserUsername).To(Equal("foo"))
			Expect(j.Containers[1].UserGravatarHash).To(Equal("b48def645758b95537d4424c84d1a9ff"))
		})
	})

	Describe("POST /api/v0/containers", func() {
		var params *url.Values
		var createResult *containers.CreateResult
		var createError error

		var receivedHost *host.Host
		var receivedOptions url.Values

		var previousCreateFn func(*host.Host, url.Values, *middleware.Env) (*containers.CreateResult, error)

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("slug", "elixir-web-1")
			params.Set("name", "Elixir Web Server 1")
			params.Set("image", "elixir")
			params.Add("env[]", "LANG=C")
			params.Add("env[]", "NODE=nope")
			params.Add("port[]", "3000:8080")
			params.Add("port[]", "0.0.0.0::4552")
			params.Add("port[]", "0.0.0.0:8081:11211")

			mockClient.CreatedContainer = &freighterclient.Container{
				ID:      "deadbeef",
				Name:    "elixir-web-1",
				Host:    "",
				Gateway: "",
				State:   "running",
				Image:   "elixir",
				ImageID: "abc123",
				User:    "guest",
			}

			createResult = nil
			createError = nil

			receivedHost = nil
			receivedOptions = url.Values{}

			previousCreateFn = containers.PerformCreateFn
			containers.PerformCreateFn = func(h *host.Host, options url.Values, env *middleware.Env) (*containers.CreateResult, error) {
				receivedHost = h
				receivedOptions = options

				return createResult, createError
			}
		})

		AfterEach(func() {
			containers.PerformCreateFn = previousCreateFn
			createResult = nil
			createError = nil
			receivedHost = nil
			receivedOptions = nil
		})

		doRequest := func(token, host string, params *url.Values) {
			if params == nil {
				params = &url.Values{}
			}
			params.Set("host", host)
			req, err := NewFormRequest("POST", "/api/v0/containers", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		It("should return an error if the call to Create fails", func() {
			createError = errors.New("Unable to make TCP connection")

			doRequest("valid-token", "foo", params)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			Expect(receivedHost.ID).To(Equal(hs.H.ID))
			Expect(receivedOptions).To(Equal(*params))
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			createResult = &containers.CreateResult{
				Container: &serializers.Container{
					ID:    232,
					CID:   "x23",
					State: "starting",
					Image: "img1",
				},
			}

			doRequest("valid-token", "foo", params)
			VerifyDelinquentUserResponse(res)
		})

		Context("when container is successfully created", func() {
			BeforeEach(func() {
				createResult = &containers.CreateResult{
					Container: &serializers.Container{
						ID:    232,
						CID:   "x23",
						State: "starting",
						Image: "img1",
					},
				}
				doRequest("valid-token", "foo", params)
			})

			It("should return a serialized form of the Container", func() {
				Expect(res.Code).To(Equal(http.StatusCreated))

				var des struct {
					Container *serializers.Container
				}
				err := json.NewDecoder(res.Body).Decode(&des)
				Expect(err).To(BeNil())
				Expect(*des.Container).To(Equal(*createResult.Container))
			})
		})

		It("should return a error information if provided", func() {
			createResult = &containers.CreateResult{
				Container: &serializers.Container{
					ID:    232,
					CID:   "x23",
					State: "starting",
					Image: "img1",
				},
				ErrorMessage:     "validation_error",
				ErrorDescription: "Name is empty",
			}

			doRequest("valid-token", "foo", params)
			Expect(res.Code).To(Equal(http.StatusCreated))

			var des struct {
				Container        *serializers.Container `json:"container"`
				Error            string                 `json:"error"`
				ErrorDescription string                 `json:"error_description"`
			}
			err := json.NewDecoder(res.Body).Decode(&des)
			Expect(err).To(BeNil())
			Expect(*des.Container).To(Equal(*createResult.Container))
			Expect(des.Error).To(Equal("validation_error"))
			Expect(des.ErrorDescription).To(Equal("Name is empty"))
		})
	})

	Describe("POST /containers/{slug}/commit", func() {
		doRequest := func(name string) {
			params := &url.Values{}
			if name != "" {
				params.Set("name", name)
			}
			req, err := NewAuthedFormRequest(t, "POST",
				"/api/v0/containers/elixir-web-1/commit",
				params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c := &container.Container{
				CID:    "deadbeef",
				Slug:   "elixir-web-1",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			mockClient.CommitImage = &freighterclient.Image{
				ID:          "1a",
				Name:        "elixir-staging:__snapshot__20140813-000000",
				Tag:         "latest",
				CreatedAt:   1903,
				ActualSize:  13,
				VirtualSize: 29,
			}

			fc := &freighterclient.Container{
				ID:         "deadbeef",
				Name:       "elixir-web-1",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				StartedAt:  time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("elixir-staging")
			VerifyDelinquentUserResponse(res)
		})

		It("creates a new image record of the snapshot", func() {
			doRequest("elixir-staging")

			i, err := image.Find(db, u, hs.H, "elixir-staging:__snapshot__20140813-000000:latest")
			Expect(err).To(BeNil())
			Expect(i.UserID).To(Equal(u.ID))
			Expect(i.HostID).To(Equal(hs.H.ID))
			Expect(i.FreighterID).To(Equal("1a"))
			Expect(i.Name).To(Equal("elixir-staging"))
			Expect(i.Slug).To(MatchRegexp("elixir-staging:__snapshot__20140813-000000"))
			Expect(i.Tag).To(Equal("latest"))
			Expect(i.IsSnapshot).To(Equal(true))
			Expect(i.OriginalImage.String).To(Equal("nitrous-base"))
		})

		It("returns image and container on success", func() {
			doRequest("elixir-staging")
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(mockClient.CommitCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
			Expect(mockClient.ReceivedName).To(MatchRegexp("elixir-staging:__snapshot__[0-9]{8}-[0-9]{6}"))
			var j struct {
				Image     *serializers.Image     `json:"image"`
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Image).NotTo(BeNil())
			Expect(j.Image.Name).To(Equal("elixir-staging"))
			Expect(j.Image.Slug).To(MatchRegexp("elixir-staging:__snapshot__20140813-000000"))
			Expect(j.Image.Tag).To(Equal("latest"))
			Expect(j.Container).NotTo(BeNil())
			Expect(j.Container.Slug).To(Equal("elixir-web-1"))
			Expect(j.Container.State).To(Equal("running"))
		})

		It("creates a slug from the name param", func() {
			mockClient.CommitImage = &freighterclient.Image{
				ID:          "1a",
				Name:        "node4-2-1-12-3-2015-2-45-37-pm",
				Tag:         "__snapshot__20140813-000000",
				CreatedAt:   1903,
				ActualSize:  13,
				VirtualSize: 29,
			}

			// At one point we were prefilling the snapshot name like so.
			doRequest("node4.2.1 12/3/2015 2:45:37 PM")

			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(mockClient.CommitCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
			Expect(mockClient.ReceivedName).To(MatchRegexp("node4-2-1-12-3-2015-2-45-37-pm:__snapshot__[0-9]{8}-[0-9]{6}"))

			i, err := image.Find(db, u, hs.H, "node4-2-1-12-3-2015-2-45-37-pm:__snapshot__20140813-000000")
			Expect(err).To(BeNil())
			Expect(i.UserID).To(Equal(u.ID))
			Expect(i.HostID).To(Equal(hs.H.ID))
			Expect(i.FreighterID).To(Equal("1a"))
			Expect(i.Name).To(Equal("node4.2.1 12/3/2015 2:45:37 PM"))
			Expect(i.Slug).To(Equal("node4-2-1-12-3-2015-2-45-37-pm"))
			Expect(i.Tag).To(Equal("__snapshot__20140813-000000"))
		})

		It("returns a server error when freighter fails", func() {
			mockClient.InfoError = errors.New("you are not Chuck Norris")
			doRequest("elixir-staging")
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})

		It("requires name parameter", func() {
			doRequest("")
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("image name is required"))
		})

		It("handles image already exists error", func() {
			mockClient.CommitError = freighterclient.ErrImageExists
			doRequest("taken")
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("image already exists"))
		})

		It("handles illegal tag name error", func() {
			mockClient.CommitError = freighterclient.ErrIllegalTagName
			doRequest("name:tag with spaces")
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("Illegal tag name: only [A-Za-z0-9_.-] are allowed, minimum 1, maximum 128 in length"))
		})

		It("publishes an image created event with the committed image", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("elixir-staging")

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventImageCreated))

			evtData, ok := evt.Data.(map[string]*serializers.Image)
			Expect(ok).To(BeTrue())
			img, ok := evtData["image"]
			Expect(ok).To(BeTrue())
			Expect(img).NotTo(BeNil())
			Expect(img.Slug).To(Equal("elixir-staging:__snapshot__20140813-000000"))
			Expect(img.Tag).To(Equal("latest"))

			close(done)
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/containers/elixir-web-1/commit"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /containers/{slug}", func() {
		var (
			c         *container.Container
			fc        *freighterclient.Container
			slug      = "foo"
			token     = ""
			startedAt = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func() {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/containers/%s", slug), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			token = "valid-token"

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   slug,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       slug,
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				User:       "guest",
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc}
			mockClient.ConfigContainerConfig = &freighterclient.ContainerConfig{
				PortBindings: []string{"public:4040:5050/tcp", "public:2020:22/tcp", "private:4000:3000/tcp"},
				Env: []string{
					"NITROUS_IMAGE_VERSION=1.2.3",
					"PAGER=less",
					"NITROUS_SSH_ENABLED=true",
					"NITROUS_USERNAME=nitrous",
				},
				Mounts: []string{
					"database-75:/var/lib/postgresql/data",
					"/magic/bla/slug:/.nitrous-container-slug:ro",
					"/magic/bla/ssh:/nitrous/ssh:ro",
				},
				Cmd: []string{"/bin/yes"},
			}
		})

		It("should allow the user even if it has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("calls the Freighter client to get the container's info and config", func() {
			doRequest()
			Expect(mockClient.InfoCalled).To(Equal(1))
			Expect(mockClient.ConfigCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Container.ID).To(Equal(c.ID))
			Expect(j.Container.CID).To(Equal("deadbeef"))
			Expect(j.Container.UserID).To(Equal(u.ID))
			Expect(j.Container.HostID).To(Equal(hs.H.ID))
			Expect(j.Container.Image).To(Equal("nitrous-base"))
			Expect(j.Container.Slug).To(Equal(slug))
			Expect(j.Container.Name).To(Equal("Elixir server"))
			Expect(j.Container.IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Container.GatewayIP).To(Equal("1.2.3.0"))
			Expect(j.Container.State).To(Equal("running"))
			Expect(j.Container.Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(j.Container.Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(j.Container.ExitCode).To(Equal(0))
			Expect(j.Container.User).To(Equal("guest"))
			Expect(j.Container.StartedAt).To(Equal(&startedAt))
			Expect(j.Container.FinishedAt).To(BeNil())
			Expect(j.Container.CreatedAt).NotTo(BeNil())
			Expect(j.Container.UpdatedAt).NotTo(BeNil())
			Expect(j.Container.TerminatedAt).To(BeNil())
			Expect(j.Container.SSHURL).To(Equal("ssh://nitrous@foo.n2odev.com:2020"))
			Expect(j.Container.Config.Cmd).To(Equal(mockClient.ConfigContainerConfig.Cmd))
			Expect(j.Container.Config.Env).To(Equal(mockClient.ConfigContainerConfig.Env))
			Expect(j.Container.Config.Ports).To(Equal(mockClient.ConfigContainerConfig.PortBindings))
			Expect(j.Container.Config.Mounts).To(Equal([]string{"database-75:/var/lib/postgresql/data"}))
		})

		It("includes the container userdata if available", func() {
			ucd := &usercontainerdata.UserContainerData{
				ContainerID: c.ID,
				UserID:      u.ID,
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"Key": sql.NullString{"Value", true},
					},
				},
			}
			nstmt, err := db.PrepareNamed(`INSERT INTO user_container_data
                                (user_id, container_id, metadata)
                                VALUES (:user_id, :container_id, :metadata) RETURNING *;`)
			Expect(err).To(BeNil())
			Expect(nstmt.QueryRowx(ucd).StructScan(ucd)).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Container.ID).To(Equal(c.ID))
			Expect(j.Container.UserData["Key"]).To(Equal("Value"))
		})

		Describe("SSH URL", func() {
			It("should have valid URL with valid parameters", func() {
				testCases := []struct {
					config      *freighterclient.ContainerConfig
					expectedURL string
				}{
					{
						&freighterclient.ContainerConfig{
							PortBindings: []string{"public:2020:22/tcp"},
							Env: []string{
								"NITROUS_SSH_ENABLED=true",
								"NITROUS_USERNAME=nitrous",
							},
						},
						"ssh://nitrous@foo.n2odev.com:2020",
					},
					{
						&freighterclient.ContainerConfig{
							PortBindings: []string{"public:2020:22/tcp"},
							Env: []string{
								"NITROUS_SSH_ENABLED=true",
							},
						},
						"ssh://root@foo.n2odev.com:2020",
					},
				}

				for _, testCase := range testCases {
					mockClient.ConfigContainerConfig = testCase.config
					doRequest()
					Expect(res.Code).To(Equal(http.StatusOK))

					var j struct {
						Container *serializers.Container `json:"container"`
					}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					cn := j.Container

					Expect(cn.SSHURL).To(Equal(testCase.expectedURL))
				}
			})

			It("should not have ssh_url field with invalid parameters", func() {
				testConfigs := []*freighterclient.ContainerConfig{
					{
						PortBindings: []string{"public:2020:22/tcp"},
						Env: []string{
							"NITROUS_USERNAME=nitrous",
						},
					},
					{
						PortBindings: []string{"public:2020:22/tcp"},
						Env: []string{
							"NITROUS_SSH_ENABLED=false",
							"NITROUS_USERNAME=nitrous",
						},
					},
					{
						PortBindings: []string{"public:8080:80/tcp"},
						Env: []string{
							"NITROUS_SSH_ENABLED=true",
							"NITROUS_USERNAME=nitrous",
						},
					},
					{
						PortBindings: []string{"private:2020:22/tcp"},
						Env: []string{
							"NITROUS_SSH_ENABLED=true",
							"NITROUS_USERNAME=nitrous",
						},
					},
					{
						PortBindings: []string{"public:2020:22/udp"},
						Env: []string{
							"NITROUS_SSH_ENABLED=true",
							"NITROUS_USERNAME=nitrous",
						},
					},
				}

				for _, config := range testConfigs {
					mockClient.ConfigContainerConfig = config
					doRequest()
					Expect(res.Code).To(Equal(http.StatusOK))

					var j struct {
						Container *serializers.Container `json:"container"`
					}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					cn := j.Container

					Expect(cn.SSHURL).To(Equal(""))
				}
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

			Context("When the error message is 'container not found'", func() {
				BeforeEach(func() {
					mockClient.InfoError = errors.New("container not found")
				})

				It("responds with 404", func() {
					doRequest()
					Expect(res.Code).To(Equal(http.StatusNotFound))
				})

				It("marks the container as terminated", func() {
					doRequest()
					c, err := container.Find(db, hs.U, slug)
					Expect(err).To(MatchError(sql.ErrNoRows))
					Expect(c).To(BeNil())
				})
			})
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with the container with its state set to host_stopped", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Container *serializers.Container `json:"container"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.ID).To(Equal(c.ID))
				Expect(j.Container.CID).To(Equal("deadbeef"))
				Expect(j.Container.UserID).To(Equal(u.ID))
				Expect(j.Container.HostID).To(Equal(hs.H.ID))
				Expect(j.Container.Image).To(Equal("nitrous-base"))
				Expect(j.Container.Slug).To(Equal(slug))
				Expect(j.Container.Name).To(Equal("Elixir server"))
				Expect(j.Container.IPAddress).To(Equal(""))
				Expect(j.Container.GatewayIP).To(Equal(""))
				Expect(j.Container.State).To(Equal(container.StateHostStopped.String()))
				Expect(j.Container.ExitCode).To(Equal(0))
				Expect(j.Container.User).To(Equal(""))
				Expect(j.Container.StartedAt).To(BeNil())
				Expect(j.Container.FinishedAt).To(BeNil())
				Expect(j.Container.CreatedAt).NotTo(BeNil())
				Expect(j.Container.UpdatedAt).NotTo(BeNil())
				Expect(j.Container.TerminatedAt).To(BeNil())
			})
		})

		Context("when host IP address is not available", func() {
			BeforeEach(func() {
				hs.H.IPAddress = sql.NullString{}
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with the container with its state set to host_stopped", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Container *serializers.Container `json:"container"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.ID).To(Equal(c.ID))
				Expect(j.Container.CID).To(Equal("deadbeef"))
				Expect(j.Container.UserID).To(Equal(u.ID))
				Expect(j.Container.HostID).To(Equal(hs.H.ID))
				Expect(j.Container.Image).To(Equal("nitrous-base"))
				Expect(j.Container.Slug).To(Equal(slug))
				Expect(j.Container.Name).To(Equal("Elixir server"))
				Expect(j.Container.IPAddress).To(Equal(""))
				Expect(j.Container.GatewayIP).To(Equal(""))
				Expect(j.Container.State).To(Equal(container.StateHostStopped.String()))
				Expect(j.Container.ExitCode).To(Equal(0))
				Expect(j.Container.User).To(Equal(""))
				Expect(j.Container.StartedAt).To(BeNil())
				Expect(j.Container.FinishedAt).To(BeNil())
				Expect(j.Container.CreatedAt).NotTo(BeNil())
				Expect(j.Container.UpdatedAt).NotTo(BeNil())
				Expect(j.Container.TerminatedAt).To(BeNil())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s", slug)
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	type controlTestInputs struct {
		action             string
		actionCounter      *int
		actionError        *error
		tempState          string
		expectedState      string
		expectedEventNames []controllers.EventName
	}

	itBehavesLikeAControlAction := func(inputs *controlTestInputs) {
		var (
			c  *container.Container
			fc *freighterclient.Container

			slug       = "elixir-web-1"
			startedAt  = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			finishedAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func(slug string) {
			req, err := NewRequest("POST", fmt.Sprintf("/api/v0/containers/%s/%s", slug, inputs.action), nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   slug,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       slug,
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "stopped",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   126,
				User:       "guest",
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		expectSerializedContainer := func(cn *serializers.Container) {
			Expect(cn.ID).To(Equal(c.ID))
			Expect(cn.CID).To(Equal("deadbeef"))
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.Image).To(Equal("nitrous-base"))
			Expect(cn.Slug).To(Equal(slug))
			Expect(cn.IPAddress).To(Equal("1.2.3.4"))
			Expect(cn.GatewayIP).To(Equal("1.2.3.0"))
			Expect(cn.State).To(Equal(inputs.expectedState))
			Expect(cn.Name).To(Equal("Elixir server"))
			Expect(cn.HostID).To(Equal(hs.H.ID))
			Expect(cn.Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(cn.Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(cn.ExitCode).To(Equal(126))
			Expect(cn.User).To(Equal("guest"))
			Expect(cn.StartedAt).To(Equal(&startedAt))
			Expect(cn.FinishedAt).To(Equal(&finishedAt))
			Expect(cn.CreatedAt).NotTo(BeNil())
			Expect(cn.UpdatedAt).NotTo(BeNil())
			Expect(cn.TerminatedAt).To(BeNil())
		}

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("elixir-web-1")
			VerifyDelinquentUserResponse(res)
		})

		It("calls the Freighter client to perform the control action", func() {
			doRequest("elixir-web-1")
			Expect(*inputs.actionCounter).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest("elixir-web-1")

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			expectSerializedContainer(j.Container)
		})

		It("publishes a state transition events for the container", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("elixir-web-1")

			msgs := []pubsub.Message{}
			finished := make(chan struct{})
			go func() {
				msgs = append(msgs, <-ch, <-ch)
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail(fmt.Sprintf("timeout while waiting for %s", inputs.action))
			case <-finished:
				// evt1 - when invoking the action
				evt1, ok := msgs[0].(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt1.Name).To(Equal(inputs.expectedEventNames[0]))

				data, ok := evt1.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				cn, ok := data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.ID).To(Equal(c.ID))
				Expect(cn.State).To(Equal(inputs.tempState))

				// evt2 - after completing the action
				evt2, ok := msgs[1].(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt2.Name).To(Equal(inputs.expectedEventNames[1]))

				data, ok = evt2.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				cn, ok = data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				expectSerializedContainer(cn)
			}
		})

		It("publishes a state transition events for the container's collaborators", func() {
			recv := &user.User{
				Username: "recv",
				Email:    "recv@example.com",
				Password: "123456",
			}
			err := WrapInTransaction(db, recv.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, recv.ID)
			Expect(err).To(BeNil())

			ch := svr.Events.Subscribe(recv.Topic())
			defer svr.Events.Unsubscribe(ch, recv.Topic())

			doRequest("elixir-web-1")

			msgs := []pubsub.Message{}
			finished := make(chan struct{})
			go func() {
				msgs = append(msgs, <-ch, <-ch)
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail(fmt.Sprintf("timeout while waiting for %s", inputs.action))
			case <-finished:
				// evt1 - when invoking the action
				evt1, ok := msgs[0].(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt1.Name).To(Equal(inputs.expectedEventNames[0]))

				data, ok := evt1.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				cn, ok := data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.ID).To(Equal(c.ID))
				Expect(cn.State).To(Equal(inputs.tempState))

				// evt2 - after completing the action
				evt2, ok := msgs[1].(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt2.Name).To(Equal(inputs.expectedEventNames[1]))

				data, ok = evt2.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				cn, ok = data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				expectSerializedContainer(cn)
			}
		})

		It("should not publish a state transition events for unrelated users", func() {
			recv := &user.User{
				Username: "recv",
				Email:    "recv@example.com",
				Password: "123456",
			}
			err := WrapInTransaction(db, recv.Insert)
			Expect(err).To(BeNil())

			ch := svr.Events.Subscribe(recv.Topic())
			defer svr.Events.Unsubscribe(ch, recv.Topic())

			doRequest("elixir-web-1")

			msgs := []pubsub.Message{}
			finished := make(chan struct{})
			go func() {
				msgs = append(msgs, <-ch, <-ch)
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Expect(msgs).To(BeEmpty())
			case <-finished:
				Expect(msgs).To(BeEmpty())
			}
		})

		It("unlocks the container after the action", func() {
			doRequest("elixir-web-1")

			// check if the container is locakable
			expiresIn := time.Duration(5) * time.Second
			err := c.Lock(container.StateStopping, expiresIn, db)
			Expect(err).To(BeNil())
		})

		Context("when container is already locked", func() {
			It("responds with HTTP 409 Conflict", func() {
				expiresIn := time.Duration(5) * time.Second
				err := c.Lock(container.StateStopping, expiresIn, db)
				Expect(err).To(BeNil())

				doRequest("elixir-web-1")

				Expect(res.Code).To(Equal(http.StatusConflict))

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("container_locked"))
			})
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				*inputs.actionError = errors.New("control action failed")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest("elixir-web-1")

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("internal_server_error"))
			})

			It("unlocks the container", func() {
				doRequest("elixir-web-1")

				cntnr, err := container.FindByCID(db, "deadbeef")
				Expect(err).To(BeNil())
				Expect(cntnr.LockedUntil).To(BeNil())
			})

			Context("When the error message is 'container not found'", func() {
				BeforeEach(func() {
					*inputs.actionError = errors.New("container not found")
				})

				It("responds with 404", func() {
					doRequest("elixir-web-1")
					Expect(res.Code).To(Equal(http.StatusNotFound))
				})

				It("marks the container as terminated", func() {
					doRequest("elixir-web-1")
					c, err := container.Find(db, hs.U, slug)
					Expect(err).To(MatchError(sql.ErrNoRows))
					Expect(c).To(BeNil())
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/%s", slug, inputs.action)
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	}

	Describe("GET /api/v0/containers/{slug}/config", func() {
		var (
			c *container.Container
		)

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "dominant-intercooler-42",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Rails app server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(slug string) {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/containers/%s/config", slug), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		ItRespondsWithConfig := func() {
			It("responds with the various configs", func() {
				mockClient.ConfigContainerConfig = &freighterclient.ContainerConfig{
					PortBindings: []string{"4040:5050", "4000:3000"},
					Env:          []string{"NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"},
					Mounts:       []string{"database-75:/var/lib/postgresql/data"},
					Cmd:          []string{"/bin/yes"},
				}

				doRequest(c.Slug)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.ConfigCalled).To(Equal(1))
				var j struct {
					Config *serializers.ContainerConfig `json:"config"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Config.Cmd).To(Equal(mockClient.ConfigContainerConfig.Cmd))
				Expect(j.Config.Env).To(Equal(mockClient.ConfigContainerConfig.Env))
				Expect(j.Config.Ports).To(Equal(mockClient.ConfigContainerConfig.PortBindings))
				Expect(j.Config.Mounts).To(Equal(mockClient.ConfigContainerConfig.Mounts))
			})

			It("responds exclusing nitrous specific mounts", func() {
				mockClient.ConfigContainerConfig = &freighterclient.ContainerConfig{
					PortBindings: []string{"4040:5050", "4000:3000"},
					Env:          []string{"NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"},
					Mounts: []string{
						"database-75:/var/lib/postgresql/data",
						"/magic/bla/slug:/.nitrous-container-slug:ro",
						"/magic/bla/ssh:/nitrous/ssh:ro",
					},
					Cmd: []string{"/bin/yes"},
				}

				doRequest(c.Slug)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.ConfigCalled).To(Equal(1))
				var j struct {
					Config *serializers.ContainerConfig `json:"config"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Config.Cmd).To(Equal(mockClient.ConfigContainerConfig.Cmd))
				Expect(j.Config.Env).To(Equal(mockClient.ConfigContainerConfig.Env))
				Expect(j.Config.Ports).To(Equal(mockClient.ConfigContainerConfig.PortBindings))
				Expect(j.Config.Mounts).To(Equal([]string{"database-75:/var/lib/postgresql/data"}))
			})
		}

		Context("when theres billing issues", func() {
			BeforeEach(func() {
				MarkDelinquentUser(db, u)
			})

			ItRespondsWithConfig()
		})

		Context("when valid token is provided", func() {
			ItRespondsWithConfig()
		})

		It("responds with 404 if container is not present", func() {
			doRequest("nonesuch")
			Expect(res.Code).To(Equal(http.StatusNotFound))
			Expect(mockClient.ConfigCalled).To(Equal(0))
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/config", c.Slug)
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("Container lock test", func() {
		errorSpy := func() (func(error), func() error) {
			var caughtError error

			errHandler := func(err error) {
				Expect(err).ToNot(BeNil())
				Expect(caughtError).To(BeNil())
				caughtError = err
			}

			errorCaught := func() error {
				return caughtError
			}

			return errHandler, errorCaught
		}

		taskSpy := func(err error) (func() error, func() bool) {
			called := false

			task := func() error {
				Expect(called).To(BeFalse())
				called = true
				return err
			}

			taskCalled := func() bool {
				return called
			}

			return task, taskCalled
		}

		Describe("Generic lock test", func() {
			It("Task, locker and unlocker should be called if all is completed successfully", func() {
				locker, lockerCalled := taskSpy(nil)
				unlocker, unlockerCalled := taskSpy(nil)
				errHandler, errorCaught := errorSpy()
				taskCalled := false

				err := containers.DoExclusive(locker, unlocker, errHandler,
					func() error {
						Expect(taskCalled).To(BeFalse())
						Expect(lockerCalled()).To(BeTrue())
						Expect(unlockerCalled()).To(BeFalse())
						taskCalled = true

						return nil
					})

				Expect(err).To(BeNil())
				Expect(taskCalled).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())
				Expect(errorCaught()).To(BeNil())
			})

			It("Task and unlocker should not be called if locker function failed", func() {
				lockErr := errors.New("Container is locked")

				locker, lockerCalled := taskSpy(lockErr)
				task, taskCalled := taskSpy(nil)
				unlocker, unlockerCalled := taskSpy(nil)
				errHandler, errorCaught := errorSpy()

				err := containers.DoExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(lockErr))
				Expect(lockerCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeFalse())
				Expect(taskCalled()).To(BeFalse())
				Expect(errorCaught()).To(Equal(lockErr))
			})

			It("Unlocker should always be called even when task failed", func() {
				taskErr := errors.New("Error in task")

				locker, lockerCalled := taskSpy(nil)
				task, taskCalled := taskSpy(taskErr)
				unlocker, unlockerCalled := taskSpy(nil)
				errHandler, errorCaught := errorSpy()

				err := containers.DoExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(taskErr))
				Expect(lockerCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())
				Expect(taskCalled()).To(BeTrue())
				Expect(errorCaught()).To(Equal(taskErr))
			})

			It("Unlocker error should be handled by error handler", func() {
				unlockErr := errors.New("Failed to unlock container")

				locker, lockerCalled := taskSpy(nil)
				task, taskCalled := taskSpy(nil)
				unlocker, unlockerCalled := taskSpy(unlockErr)
				errHandler, errorCaught := errorSpy()

				err := containers.DoExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(unlockErr))
				Expect(lockerCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())
				Expect(taskCalled()).To(BeTrue())
				Expect(errorCaught()).To(Equal(unlockErr))
			})

			It("Unlock error should override task error if both failed", func() {
				taskErr := errors.New("Error in task")
				unlockErr := errors.New("Failed to unlock container")

				taskErrHandler, taskErrorCaught := errorSpy()
				unlockErrHandler, unlockErrorCaught := errorSpy()

				locker, lockerCalled := taskSpy(nil)
				task, taskCalled := taskSpy(taskErr)
				unlocker, unlockerCalled := taskSpy(unlockErr)

				errHandler := func(err error) {
					if taskErrorCaught() == nil {
						taskErrHandler(err)
					} else {
						unlockErrHandler(err)
					}
				}

				err := containers.DoExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(unlockErr))
				Expect(lockerCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())
				Expect(taskCalled()).To(BeTrue())
				Expect(taskErrorCaught()).To(Equal(taskErr))
				Expect(unlockErrorCaught()).To(Equal(unlockErr))
			})

			It("Unlocker should always be called even on panic", func() {
				locker, lockerCalled := taskSpy(nil)
				unlocker, unlockerCalled := taskSpy(nil)
				errHandler, errorCaught := errorSpy()

				var err error
				var taskCalled, panicRecovered = false, false

				func() {
					defer func() {
						if r := recover(); r != nil {
							panicRecovered = true
						}
					}()

					err = containers.DoExclusive(locker, unlocker, errHandler,
						func() error {
							taskCalled = true
							panic("test panic")
						})
				}()

				Expect(err).To(BeNil())
				Expect(taskCalled).To(BeTrue())
				Expect(panicRecovered).To(BeTrue())
				Expect(lockerCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())
				Expect(errorCaught()).To(BeNil())
			})
		})

		asyncTaskSpy := func(err error) (func() error, <-chan struct{}) {
			calledChan := make(chan struct{})

			task := func() error {
				close(calledChan) // send end signal

				return err
			}

			return task, calledChan
		}

		asyncErrorSpy := func() (func(error), <-chan error) {
			errChan := make(chan error)

			errHandler := func(err error) {
				errChan <- err
			}

			return errHandler, errChan
		}

		Describe("Async generic lock test", func() {
			It("Task and unlocker should eventually be called on success", func() {
				locker, lockerCalled := taskSpy(nil)
				errHandler, errorCaught := errorSpy()

				task, taskCalledChan := asyncTaskSpy(nil)
				unlocker, unlockerCalledChan := asyncTaskSpy(nil)

				err := containers.DoAsyncExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(BeNil())
				Expect(lockerCalled()).To(BeTrue())

				<-taskCalledChan
				<-unlockerCalledChan

				Expect(errorCaught()).To(BeNil())
			})

			It("Should return immediately if locker fail", func() {
				lockErr := errors.New("Container is locked")

				locker, lockerCalled := taskSpy(lockErr)
				errHandler, errorCaught := errorSpy()

				task, taskCalledChan := asyncTaskSpy(nil)
				unlocker, unlockerCalledChan := asyncTaskSpy(nil)

				err := containers.DoAsyncExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(lockErr))
				Expect(lockerCalled()).To(BeTrue())
				Expect(errorCaught()).To(Equal(lockErr))

				Consistently(taskCalledChan).ShouldNot(Receive())
				Consistently(unlockerCalledChan).ShouldNot(Receive())
			})

			It("Async task error should be handled by error handler", func() {
				taskErr := errors.New("Task error")

				locker, lockerCalled := taskSpy(nil)

				task, taskCalledChan := asyncTaskSpy(taskErr)
				unlocker, unlockerCalledChan := asyncTaskSpy(nil)

				errHandler, errorChan := asyncErrorSpy()

				err := containers.DoAsyncExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(BeNil())
				Expect(lockerCalled()).To(BeTrue())

				<-taskCalledChan

				err = <-errorChan
				Expect(err).To(Equal(taskErr))

				<-unlockerCalledChan
			})

			It("Unlock error should be handled by error handler", func() {
				unlockErr := errors.New("Error Unlocking container")

				locker, lockerCalled := taskSpy(nil)

				task, taskCalledChan := asyncTaskSpy(nil)
				unlocker, unlockerCalledChan := asyncTaskSpy(unlockErr)

				errHandler, errorChan := asyncErrorSpy()

				err := containers.DoAsyncExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(BeNil())
				Expect(lockerCalled()).To(BeTrue())

				<-taskCalledChan
				<-unlockerCalledChan

				err = <-errorChan
				Expect(err).To(Equal(unlockErr))
			})

			It("Both task and unlock errors should be handled by error handler", func() {
				taskErr := errors.New("Task error")
				unlockErr := errors.New("Error Unlocking container")

				locker, lockerCalled := taskSpy(nil)

				task, taskCalledChan := asyncTaskSpy(taskErr)
				unlocker, unlockerCalledChan := asyncTaskSpy(unlockErr)

				errHandler, errorChan := asyncErrorSpy()

				err := containers.DoAsyncExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(BeNil())
				Expect(lockerCalled()).To(BeTrue())

				<-taskCalledChan
				err = <-errorChan
				Expect(err).To(Equal(taskErr))

				<-unlockerCalledChan
				err = <-errorChan
				Expect(err).To(Equal(unlockErr))
			})
		})

		Describe("Container lock test", func() {
			var (
				slug     = "elixir-web-1"
				lockTime = 5 * time.Minute

				cn  *container.Container
				fc  *freighterclient.Container
				env *middleware.Env
			)

			doRequest := func(action string) *httptest.ResponseRecorder {
				req, err := NewRequest("POST", fmt.Sprintf("/api/v0/containers/%s/%s", slug, action), nil)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				res = httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}

			getContainerInfo := func() *serializers.Container {
				req, err := NewRequest("GET", fmt.Sprintf("/api/v0/containers/%s", slug), nil)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				res = httptest.NewRecorder()

				handler.ServeHTTP(res, req)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Container *serializers.Container `json:"container"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				return j.Container
			}

			BeforeEach(func() {
				cn = &container.Container{
					CID:    "deadbeef",
					Slug:   slug,
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
					Name:   sql.NullString{"Elixir server", true},
				}
				err := cn.Insert(db)
				Expect(err).To(BeNil())

				fc = &freighterclient.Container{
					ID:    "deadbeef",
					Name:  slug,
					State: "stopped",
					Image: "nitrous-base",
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				mockClient.ConfigContainerConfig = &freighterclient.ContainerConfig{}

				env = &middleware.Env{
					DB: db,
				}
			})

			It("Other container operations should fail when container is doing exlusive task", func() {
				locker := containers.LockerFn(cn, env, lockTime, container.StateStarting)
				unlocker := containers.UnlockerFn(cn, env)

				taskCalled := false
				errHandler, errorCaught := errorSpy()

				cn2 := getContainerInfo()
				Expect(cn2.State).To(Equal("stopped"))

				err := containers.DoExclusive(locker, unlocker, errHandler,
					func() error {
						taskCalled = true

						cn2 = getContainerInfo()
						Expect(cn2.State).To(Equal("starting"))

						// DoExclusive() already acquired the container lock
						// when we enter this function. So naturally any other
						// requests that require lock would fail.
						res := doRequest("start")
						Expect(res.Code).To(Equal(http.StatusConflict))

						return nil
					})

				Expect(err).To(BeNil())
				Expect(errorCaught()).To(BeNil())
				Expect(taskCalled).To(BeTrue())

				cn2 = getContainerInfo()
				Expect(cn2.State).To(Equal("stopped"))

				res := doRequest("start")
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("Unlock failure will cause subsequent locks to fail and should be logged", func() {
				locker := containers.LockerFn(cn, env, lockTime, container.StateStarting)

				unlockErr := errors.New("Failed to unlock container")

				task, taskCalled := taskSpy(nil)
				unlocker, unlockerCalled := taskSpy(unlockErr)
				errHandler, errorCaught := errorSpy()

				err := containers.DoExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(unlockErr))
				Expect(errorCaught()).To(Equal(unlockErr))
				Expect(taskCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())

				cn2 := getContainerInfo()
				Expect(cn2.State).To(Equal("starting"))

				res := doRequest("start")
				Expect(res.Code).To(Equal(http.StatusConflict))
			})

			It("Lock expired after unlock failure should allow other container lock operations again", func() {
				locker := containers.LockerFn(cn, env, -1*time.Second, container.StateStarting)

				taskErr := errors.New("Failed to start container")
				unlockErr := errors.New("Failed to unlock container")

				task, taskCalled := taskSpy(taskErr)
				unlocker, unlockerCalled := taskSpy(unlockErr)
				errHandler := func(err error) {} // ignore

				err := containers.DoExclusive(locker, unlocker, errHandler, task)

				Expect(err).To(Equal(unlockErr))
				Expect(taskCalled()).To(BeTrue())
				Expect(unlockerCalled()).To(BeTrue())

				cn2 := getContainerInfo()
				Expect(cn2.State).To(Equal("stopped"))

				res := doRequest("start")
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("Async container lock should return while critical section running in background", func() {
				taskErr := errors.New("Reconfiguration error")

				locker := containers.LockerFn(cn, env, lockTime, container.StateReconfiguring)
				unlocker := containers.UnlockerFn(cn, env)

				taskStartChan := make(chan struct{})
				taskResumeChan := make(chan struct{})
				unlockChan := make(chan struct{})

				wrappedUnlocker := func() error {
					err := unlocker()
					close(unlockChan)
					return err
				}

				errHandler, errorCaught := errorSpy()

				cn2 := getContainerInfo()
				Expect(cn2.State).To(Equal("stopped"))

				err := containers.DoAsyncExclusive(locker, wrappedUnlocker, errHandler,
					func() error {
						defer GinkgoRecover()

						res := doRequest("start")

						Expect(res.Code).To(Equal(http.StatusConflict))

						cn2 = getContainerInfo()
						Expect(cn2.State).To(Equal("reconfiguring"))

						// inform outside task is called
						close(taskStartChan)

						// wait to finish reconfiguration
						<-taskResumeChan

						return taskErr
					})

				Expect(err).To(BeNil())
				Expect(errorCaught()).To(BeNil())

				// wait for reconfiguration to start
				<-taskStartChan

				res := doRequest("start")

				Expect(res.Code).To(Equal(http.StatusConflict))

				cn2 = getContainerInfo()
				Expect(cn2.State).To(Equal("reconfiguring"))

				// resume reconfiguration
				close(taskResumeChan)

				// wait for unlock
				<-unlockChan

				Expect(errorCaught()).To(Equal(taskErr))

				cn2 = getContainerInfo()
				Expect(cn2.State).To(Equal("stopped"))

				res = doRequest("start")
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})
	})

	Describe("POST /api/v0/containers/{slug}/start", func() {
		var i = &controlTestInputs{}

		BeforeEach(func() {
			i.action = "start"
			i.actionCounter = &mockClient.StartCalled
			i.actionError = &mockClient.StartError
			i.tempState = "starting"
			i.expectedState = "running"
			i.expectedEventNames = []controllers.EventName{controllers.EventContainerStarting, controllers.EventContainerStarted}
		})

		itBehavesLikeAControlAction(i)

		It("calls the Freighter client to start the container", func() {
			c := &container.Container{
				CID:    "deadbead",
				Slug:   "foo",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
			mockClient.StartError = errors.New("start failed")

			params := &url.Values{}
			params.Add("env[]", "LANG=C")
			params.Add("env[]", "NODE=nope")
			params.Add("port[]", "3000:8080")
			params.Add("port[]", "0.0.0.0::4552")

			req, err := NewFormRequest("POST", "/api/v0/containers/foo/start", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			Expect(mockClient.StartCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbead"))
			conf := mockClient.ReceivedConfig
			Expect(conf.Env).To(ConsistOf("LANG=C", "NODE=nope"))
			Expect(conf.PortBindings).To(ConsistOf("3000:8080", "0.0.0.0::4552"))
		})

		Describe("Port mappings", func() {
			var c *container.Container

			BeforeEach(func() {
				c = &container.Container{
					CID:    "deadbead",
					Slug:   "foo",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())
			})

			doRequest := func() {
				req, err := NewRequest("POST", "/api/v0/containers/foo/start", nil)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				handler.ServeHTTP(res, req)
			}

			It("updates port mappings", func() {
				fc := &freighterclient.Container{
					ID:         "deadbead",
					Name:       "foo",
					Host:       "1.2.3.4",
					Gateway:    "1.2.3.0",
					State:      "starting",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					User:       "guest",
					Ports: map[string][]string{
						"3000": []string{"63000"},
					},
					Env: []string{
						"NITROUS_PREVIEW_PORT=3000",
					},
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				doRequest()

				updatedContainer, err := container.FindByCID(db, "deadbead")
				Expect(err).To(BeNil())

				var portMappings map[string]string
				err = json.Unmarshal([]byte(updatedContainer.PortMappings), &portMappings)
				Expect(err).To(BeNil())
				Expect(portMappings["preview"]).To(Equal("63000"))
			})

			It("does not update preview port if NITROUS_PREVIEW_PORT is not specified", func() {
				fc := &freighterclient.Container{
					ID:         "deadbead",
					Name:       "foo",
					Host:       "1.2.3.4",
					Gateway:    "1.2.3.0",
					State:      "starting",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					User:       "guest",
					Ports: map[string][]string{
						"3000": []string{"63000"},
					},
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				doRequest()

				updatedContainer, err := container.FindByCID(db, "deadbead")
				Expect(err).To(BeNil())

				var portMappings map[string]string
				err = json.Unmarshal([]byte(updatedContainer.PortMappings), &portMappings)
				Expect(err).To(BeNil())
				Expect(portMappings["3000"]).To(Equal("63000"))
				_, ok := portMappings["preview"]
				Expect(ok).To(BeFalse())
			})

			It("does not update preview port if there is no port mapping for NITROUS_PREVIEW_PORT", func() {
				fc := &freighterclient.Container{
					ID:         "deadbead",
					Name:       "foo",
					Host:       "1.2.3.4",
					Gateway:    "1.2.3.0",
					State:      "starting",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					User:       "guest",
					Ports: map[string][]string{
						"4000": []string{"63000"},
					},
					Env: []string{
						"NITROUS_PREVIEW_PORT=3000",
					},
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				doRequest()

				updatedContainer, err := container.FindByCID(db, "deadbead")
				Expect(err).To(BeNil())

				var portMappings map[string]string
				err = json.Unmarshal([]byte(updatedContainer.PortMappings), &portMappings)
				Expect(err).To(BeNil())
				Expect(portMappings["4000"]).To(Equal("63000"))
				_, ok := portMappings["preview"]
				Expect(ok).To(BeFalse())
			})
		})
	})

	Describe("POST /api/v0/containers/{slug}/stop", func() {
		var i = &controlTestInputs{}

		BeforeEach(func() {
			i.action = "stop"
			i.actionCounter = &mockClient.StopCalled
			i.actionError = &mockClient.StopError
			i.tempState = "stopping"
			i.expectedState = "stopped"
			i.expectedEventNames = []controllers.EventName{controllers.EventContainerStopping, controllers.EventContainerStopped}
		})

		itBehavesLikeAControlAction(i)
	})

	Describe("POST /api/v0/containers/{name}/restart", func() {
		var i = &controlTestInputs{}

		BeforeEach(func() {
			i.action = "restart"
			i.actionCounter = &mockClient.RestartCalled
			i.actionError = &mockClient.RestartError
			i.tempState = "restarting"
			i.expectedState = "running"
			i.expectedEventNames = []controllers.EventName{controllers.EventContainerRestarting, controllers.EventContainerRestarted}
		})

		itBehavesLikeAControlAction(i)

		Describe("Port Mappings", func() {
			var c *container.Container

			BeforeEach(func() {
				c = &container.Container{
					CID:    "deadbead",
					Slug:   "foo",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())
			})

			doRequest := func() {
				req, err := NewRequest("POST", "/api/v0/containers/foo/restart", nil)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				handler.ServeHTTP(res, req)
			}

			It("updates preview port", func() {
				fc := &freighterclient.Container{
					ID:         "deadbead",
					Name:       "foo",
					Host:       "1.2.3.4",
					Gateway:    "1.2.3.0",
					State:      "starting",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					User:       "guest",
					Ports: map[string][]string{
						"3000": []string{"63000"},
					},
					Env: []string{
						"NITROUS_PREVIEW_PORT=3000",
					},
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				doRequest()

				updatedContainer, err := container.FindByCID(db, "deadbead")
				Expect(err).To(BeNil())

				var portMappings map[string]string
				err = json.Unmarshal([]byte(updatedContainer.PortMappings), &portMappings)
				Expect(err).To(BeNil())
				Expect(portMappings["preview"]).To(Equal("63000"))
			})

			It("does not update preview port if NITROUS_PREVIEW_PORT is not specified ", func() {
				fc := &freighterclient.Container{
					ID:         "deadbead",
					Name:       "foo",
					Host:       "1.2.3.4",
					Gateway:    "1.2.3.0",
					State:      "starting",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					User:       "guest",
					Ports: map[string][]string{
						"3000": []string{"63000"},
					},
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				doRequest()

				updatedContainer, err := container.FindByCID(db, "deadbead")
				Expect(err).To(BeNil())

				var portMappings map[string]string
				err = json.Unmarshal([]byte(updatedContainer.PortMappings), &portMappings)
				Expect(err).To(BeNil())
				Expect(portMappings["3000"]).To(Equal("63000"))
				_, ok := portMappings["preview"]
				Expect(ok).To(BeFalse())
			})

			It("does not update preview port if there is not mapping port for NITROUS_PREVIEW_PORT", func() {
				fc := &freighterclient.Container{
					ID:         "deadbead",
					Name:       "foo",
					Host:       "1.2.3.4",
					Gateway:    "1.2.3.0",
					State:      "starting",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					User:       "guest",
					Ports: map[string][]string{
						"3000": []string{"63000"},
					},
					Env: []string{
						"NITROUS_PREVIEW_PORT=4000",
					},
				}
				mockClient.Containers = []*freighterclient.Container{fc}
				doRequest()

				updatedContainer, err := container.FindByCID(db, "deadbead")
				Expect(err).To(BeNil())

				var portMappings map[string]string
				err = json.Unmarshal([]byte(updatedContainer.PortMappings), &portMappings)
				Expect(err).To(BeNil())
				Expect(portMappings["3000"]).To(Equal("63000"))
				_, ok := portMappings["preview"]
				Expect(ok).To(BeFalse())
			})
		})
	})

	Describe("DELETE /containers/{slug}", func() {
		var (
			c          *container.Container
			fc         *freighterclient.Container
			slug       = "elixir-web-1"
			token      = ""
			startedAt  = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			finishedAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
		)

		BeforeEach(func() {
			token = "valid-token"

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   slug,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       slug,
				Host:       "",
				Gateway:    "",
				State:      "",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   126,
				User:       "guest",
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		doRequest := func() {
			req, err := NewRequest("DELETE", fmt.Sprintf("/api/v0/containers/%s", slug), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		expectSerializedContainer := func(cn *serializers.Container) {
			Expect(cn.ID).To(Equal(c.ID))
			Expect(cn.CID).To(Equal("deadbeef"))
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.Image).To(Equal("nitrous-base"))
			Expect(cn.Slug).To(Equal(slug))
			Expect(cn.Name).To(Equal("Elixir server"))
			Expect(cn.IPAddress).To(Equal(""))
			Expect(cn.GatewayIP).To(Equal(""))
			Expect(cn.State).To(Equal(container.StateTerminated.String()))
			Expect(cn.Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(cn.Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(cn.ExitCode).To(Equal(126))
			Expect(cn.User).To(Equal("guest"))
			Expect(cn.StartedAt).To(Equal(&startedAt))
			Expect(cn.FinishedAt).To(Equal(&finishedAt))
			Expect(cn.CreatedAt).NotTo(BeNil())
			Expect(cn.UpdatedAt).NotTo(BeNil())
			Expect(cn.TerminatedAt).NotTo(BeNil())
		}

		It("calls the Freighter client to terminate the container", func() {
			doRequest()
			Expect(mockClient.TerminateCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest()
			VerifyDelinquentUserResponse(res)
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			expectSerializedContainer(j.Container)
		})

		It("deletes the container from the database", func() {
			doRequest()

			c, err := container.Find(db, hs.U, slug)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(c).To(BeNil())
		})

		It("removes the container's image from the host if the image is a Nitrous template", func() {
			c.Image = "quay.io/nitrous/ubuntu"
			Expect(image.IsNitrousTemplate(c.Image)).To(BeTrue())
			Expect(c.Update(db)).To(BeNil())

			doRequest()

			Expect(mockClient.RemoveImageCalled).To(Equal(1))
			Expect(mockClient.RemoveImageSlug).To(Equal(c.Image))
		})

		It("does not remove the container's image from the host if the image is a not Nitrous template", func() {
			c.Image = "ruby"
			Expect(image.IsNitrousTemplate(c.Image)).To(BeFalse())
			Expect(c.Update(db)).To(BeNil())

			doRequest()

			Expect(mockClient.RemoveImageCalled).To(Equal(0))
		})

		It("deletes the container's image's record if the image is a Nitrous template", func() {
			c.Image = "quay.io/nitrous/ubuntu"
			Expect(image.IsNitrousTemplate(c.Image)).To(BeTrue())
			Expect(c.Update(db)).To(BeNil())

			img := &image.Image{
				UserID:      u.ID,
				HostID:      hs.H.ID,
				FreighterID: "abcdef",
				Slug:        "quay.io/nitrous/ubuntu",
				Tag:         "latest",
			}
			Expect(img.Insert(db)).To(BeNil())

			_, err := image.Find(db, u, hs.H, c.Image)
			Expect(err).To(BeNil())

			doRequest()

			_, err = image.Find(db, u, hs.H, c.Image)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("publishes a container destroyed event with the destroyed container", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest()

			msgs := []pubsub.Message{}
			finished := make(chan struct{})
			go func() {
				msgs = append(msgs, <-ch, <-ch)
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail("timeout while waiting for destroy")
			case <-finished:
				// evt1 - when invoking the action
				evt1, ok := msgs[0].(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt1.Name).To(Equal(controllers.EventContainerDestroying))

				data, ok := evt1.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				cn, ok := data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.ID).To(Equal(c.ID))
				Expect(cn.State).To(Equal("terminating"))

				// evt2 - after completing the action
				evt2, ok := msgs[1].(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt2.Name).To(Equal(controllers.EventContainerDestroyed))

				data, ok = evt2.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				cn, ok = data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				expectSerializedContainer(cn)
			}
		})

		Context("when container is locked", func() {
			It("responds with HTTP 409 Conflict", func() {
				expiresIn := time.Duration(5) * time.Second
				err := c.Lock(container.StateStopping, expiresIn, db)
				Expect(err).To(BeNil())

				doRequest()

				Expect(res.Code).To(Equal(http.StatusConflict))

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("container_locked"))
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

			Context("When the error message is 'container not found'", func() {
				BeforeEach(func() {
					mockClient.TerminateError = errors.New("container not found")
				})

				It("responds with 404", func() {
					doRequest()
					Expect(res.Code).To(Equal(http.StatusNotFound))
				})

				It("marks the container as terminated", func() {
					doRequest()
					c, err := container.Find(db, hs.U, slug)
					Expect(err).To(MatchError(sql.ErrNoRows))
					Expect(c).To(BeNil())
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s", slug)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /containers/{slug}/token", func() {
		var c *container.Container
		secret := "s3cr3t"

		doRequest := func(permissions ...string) {
			p := &url.Values{}
			for _, perm := range permissions {
				p.Add("permission[]", perm)
			}
			req, err := NewAuthedFormRequest(t, "POST", "/api/v0/containers/elixir-web-1/token", p)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "elixir-web-1",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
				Secret: secret,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest()
			VerifyDelinquentUserResponse(res)
		})

		It("returns an HMAC token signed with the shared secret", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string]string
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			err = hmactoken.Verify(nil, j["token"], secret)
			Expect(err).To(BeNil())
		})

		It("returns an HMAC token with only the requested permissions", func() {
			doRequest("attach", "logs")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string]string
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			err = hmactoken.Verify([]string{"attach", "logs"}, j["token"], secret)
			Expect(err).To(BeNil())
			err = hmactoken.Verify([]string{"shell"}, j["token"], secret)
			Expect(err).To(HaveOccurred())
		})

		Context("when container is locked", func() {
			BeforeEach(func() {
				expiresIn := 5 * time.Second
				err := c.Lock(container.StateStopping, expiresIn, db)
				Expect(err).To(BeNil())
			})

			It("does not issue token", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusConflict))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("container_locked"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/containers/elixir-web-1/token"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("prepareConfig()", func() {
		var (
			form url.Values
			hv   map[string]*volume.Volume
		)

		BeforeEach(func() {
			form = url.Values{}
			hv = map[string]*volume.Volume{}
		})

		getConfs := func() *freighterclient.ContainerConfig {
			config := &freighterclient.ContainerConfig{}
			err := containers.PrepareConfig(config, form, hv)
			Expect(err).To(BeNil())
			return config
		}

		It("is empty when nothing relevant is provided", func() {
			fcc := getConfs()
			Expect(fcc.Env).To(BeNil())
			Expect(fcc.Mounts).To(BeNil())
			Expect(fcc.PortBindings).To(BeNil())
		})

		It("populates Env", func() {
			form.Add("env[]", "PAGER=less")
			fcc := getConfs()
			Expect(fcc.Env).To(Equal([]string{"PAGER=less"}))
		})

		Context("populates Mounts", func() {
			BeforeEach(func() {
				form.Add("volume[]", "music:/mpd/music")
				form.Add("volume[]", "backups")
				hv["music"] = &volume.Volume{}
				hv["backups"] = &volume.Volume{}
			})

			It("if host volumes are present", func() {
				vols := getConfs().Mounts
				Expect(vols).To(HaveLen(2))
				Expect(vols).To(ContainElement("music:/mpd/music"))
				Expect(vols).To(ContainElement("backups"))
			})

			It("returns error if host volume requested is absent", func() {
				delete(hv, "backups")
				config := &freighterclient.ContainerConfig{}
				err := containers.PrepareConfig(config, form, hv)
				Expect(err).To(Equal(containers.ErrVolumeNotFound))
			})
		})

		It("populates PortBindings", func() {
			form.Add("port[]", "3030:4040")
			form.Add("port[]", "5050:6060")
			pbs := getConfs().PortBindings
			Expect(pbs).To(ConsistOf("3030:4040", "5050:6060"))
		})

		Context("privileged option", func() {
			It("defaults to nil", func() {
				Expect(getConfs().Privileged).To(BeNil())
			})

			It("can be set to true", func() {
				form.Add("privileged", "true")
				Expect(*getConfs().Privileged).To(BeTrue())
			})
		})

		Context("publish all option", func() {
			It("defaults to nil", func() {
				Expect(getConfs().PublishAll).To(BeNil())
			})

			It("can be set to true", func() {
				form.Add("publish_all", "1")
				Expect(*getConfs().PublishAll).To(BeTrue())
			})
		})

		Context("cmd option", func() {
			It("defaults to nil", func() {
				cmd := getConfs().Cmd
				Expect(cmd).To(BeNil())
			})

			cases := []struct {
				input    string
				expected []string
				errMsg   string
			}{
				{"/bin/bash", []string{"/bin/bash"}, ""},
				{"./foo `echo $SHELL`", []string{"./foo", "`echo $SHELL`"}, ""},
				{"/bin/sh -c echo 'test this please' ",
					[]string{"/bin/sh", "-c", "echo", "test this please"}, ""},
				{"echo $(rm /tmp/ohnoes) ohno",
					[]string{"echo", "$(rm", "/tmp/ohnoes)", "ohno"}, ""},
				{"/bin/cmd\\ with\\ space arg1 arg2",
					[]string{"/bin/cmd with space", "arg1", "arg2"}, ""},
				{"unclosed quote '", []string{}, "container cmd invalid"},
			}

			for _, c2 := range cases {
				c := c2
				It(fmt.Sprintf("handles input: %s", c.input), func() {
					form.Add("cmd", c.input)
					config := &freighterclient.ContainerConfig{}
					err := containers.PrepareConfig(config, form, hv)
					if c.errMsg != "" {
						Expect(err).NotTo(BeNil())
						Expect(err.Error()).To(ContainSubstring(c.errMsg))
					} else {
						Expect(err).To(BeNil())
						Expect(config.Cmd).To(Equal(c.expected))
					}
				})
			}
		})
	})

	Describe("POST /containers/{name}/reconfig", func() {
		var (
			containerName = "elixir-web-1"
			c             *container.Container
			testParams    *url.Values
		)

		doRequest := func(params *url.Values) {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/containers/%s/reconfig", containerName), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			testParams = &url.Values{
				"env[]":       []string{"LANG=C", "NODE=nope"},
				"volume[]":    []string{"music"},
				"port[]":      []string{"3000:8080", "public::4552", "public:8081:11211"},
				"publish_all": []string{"0"},
			}

			v := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
			err := v.Insert(db)
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   containerName,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())

			mockClient.ReconfigContainer = &freighterclient.Container{
				ID:    "livebeef",
				Name:  containerName,
				Host:  "1.2.3.4",
				State: "running",
				Image: "deadbeef-reconfig-1420683989905982895",
				Ports: map[string][]string{
					"3000":  []string{"8080"},
					"4552":  []string{"65123"},
					"11211": []string{"8081"},
				},
				Env: []string{
					"NITROUS_PREVIEW_PORT=3000",
				},
			}
			mockClient.ConfigContainerConfig = &freighterclient.ContainerConfig{}
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest(testParams)
			VerifyDelinquentUserResponse(res)
		})

		It("calls the Freighter client with config from params", func() {
			doRequest(testParams)
			Expect(mockClient.ReconfigCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
			Expect(mockClient.ConfigCalled).To(Equal(1))
			Expect(mockClient.ConfigID).To(Equal("livebeef"))
			conf := mockClient.ReceivedConfig
			Expect(conf.Env).To(Equal([]string{"LANG=C", "NODE=nope"}))
			Expect(conf.Mounts).To(Equal([]string{"music"}))
			Expect(conf.PortBindings).To(HaveLen(3))
			Expect(conf.PortBindings).To(ContainElement("3000:8080"))
			Expect(conf.PortBindings).To(ContainElement("public::4552"))
			Expect(conf.PortBindings).To(ContainElement("public:8081:11211"))
		})

		It("excludes the /opt/nitrous mount if present", func() {
			testParams.Set("volume[]", "/opt/nitrous:/.nitrous:ro")

			doRequest(testParams)
			Expect(mockClient.ReconfigCalled).To(Equal(1))
			Expect(mockClient.ReceivedConfig.Mounts).To(HaveLen(0))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest(testParams)

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			cn := j.Container

			Expect(cn.CID).To(Equal("livebeef"))
			Expect(cn.Image).To(Equal("nitrous-base"))
			Expect(cn.Slug).To(Equal(containerName))
			Expect(cn.IPAddress).To(Equal("1.2.3.4"))
			Expect(cn.State).To(Equal("running"))
			Expect(cn.Name).To(Equal("Elixir server"))
			Expect(cn.UpdatedAt).NotTo(BeNil())
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.HostID).To(Equal(hs.H.ID))
		})

		It("updates container record", func() {
			doRequest(testParams)

			c, err := container.Find(db, hs.U, containerName)
			Expect(err).To(BeNil())
			Expect(c.CID).To(Equal("livebeef"))
			Expect(c.Image).To(Equal("nitrous-base"))
			Expect(c.LockedUntil).To(BeNil())

			var portMappings map[string]string
			err = json.Unmarshal([]byte(c.PortMappings), &portMappings)
			Expect(err).To(BeNil())
			Expect(portMappings["preview"]).To(Equal("8080"))
			Expect(portMappings["4552"]).To(Equal("65123"))
			Expect(portMappings["11211"]).To(Equal("8081"))
		})

		It("updates volumes", func() {
			// create previous volume association 'dotfiles', expect it to be
			// replaced with 'music' as provided in params
			v := &volume.Volume{HostID: hs.H.ID, Slug: "dotfiles"}
			err := v.Insert(db)
			Expect(err).To(BeNil())
			cv := &cvolume.ContainerVolume{ContainerID: c.ID, VolumeID: v.ID}
			err = cv.Insert(db)
			Expect(err).To(BeNil())
			doRequest(testParams)
			cvs := []*cvolume.ContainerVolume{}
			err = db.Select(&cvs, `SELECT cv.id, container_id, volume_id, cv.created_at
				FROM container_volumes cv
				JOIN containers ON containers.id = container_id
				WHERE container_id = $1 AND terminated_at IS NULL`,
				c.ID)
			Expect(err).To(BeNil())
			music, err := volume.Find(db, hs.H.ID, "music")
			Expect(err).To(BeNil())
			Expect(cvs).To(HaveLen(1))
			Expect(cvs[0].VolumeID).To(Equal(music.ID))
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.ReconfigError = errors.New("failed to reconfigure container")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest(testParams)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("internal_server_error"))
			})

			It("updates the container record if Freighter responds with a container", func() {
				doRequest(testParams)

				c, err := container.Find(db, hs.U, containerName)
				Expect(err).To(BeNil())
				Expect(c.CID).To(Equal("livebeef"))
				Expect(c.Image).To(Equal("nitrous-base"))
				Expect(c.LockedUntil).To(BeNil())
			})

			It("parses the error if binding failed", func() {
				mockClient.ReconfigError = errors.New("port is already allocated: Bind for 0.0.0.0:8080")
				doRequest(testParams)

				Expect(res.Code).To(Equal(http.StatusConflict))

				errType, msg := ErrorSlug(res.Body)
				Expect(errType).To(Equal("host_port_in_use"))
				Expect(msg).To(Equal("Container cannot be started, Host port 8080 is already in use by another container."))
			})

			Context("When the error message is 'container not found'", func() {
				BeforeEach(func() {
					mockClient.ReconfigError = errors.New("container not found")
				})

				It("responds with 404", func() {
					doRequest(testParams)
					Expect(res.Code).To(Equal(http.StatusNotFound))
				})

				It("marks the container as terminated", func() {
					doRequest(testParams)
					c, err := container.Find(db, hs.U, containerName)
					Expect(err).To(MatchError(sql.ErrNoRows))
					Expect(c).To(BeNil())
				})
			})
		})

		Context("when called in async mode and freighter is successful", func() {
			var asyncParams = &url.Values{
				"async":       []string{"1"},
				"env[]":       []string{"LANG=C"},
				"volume[]":    []string{"music"},
				"port[]":      []string{"3000:8080", "public::4552"},
				"publish_all": []string{"0"},
			}

			It("responds 202 accepted and calls freighter asynchronously", func() {
				doRequest(asyncParams)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				var j struct {
					Container       *serializers.Container       `json:"container"`
					ContainerConfig *serializers.ContainerConfig `json:"config"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.CID).To(Equal("deadbeef"), "container CID should not have changed")
			})

			It("updates the container record eventually", func() {
				doRequest(asyncParams)

				Eventually(func() (string, error) {
					cn, err := container.Find(db, u, containerName)
					return cn.CID, err
				}).Should(Equal("livebeef"))

				Eventually(func() (*time.Time, error) {
					cn, err := container.Find(db, u, containerName)
					return cn.LockedUntil, err
				}).Should(BeNil())
			})

			It("publishes success event", func() {
				mockClient.ConfigContainerConfig = &freighterclient.ContainerConfig{

					PortBindings: []string{"private:3000:8080", "public:49123:4552", "public:8081:11211"},
					Mounts: []string{
						"music",
						"/magic/bla/slug:/.nitrous-container-slug:ro",
						"/magic/bla/ssh:/nitrous/ssh:ro",
					},
					Env: []string{"LANG=C", "NODE=nope"},
					Cmd: []string{"/bin/yes"},
				}
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(asyncParams)

				evtCh := make(chan *controllers.Event, 1)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerReconfigured {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventContainerReconfigured")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerReconfigured))

					data, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(data["container"]).NotTo(BeNil())

					cnrData, ok := data["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())
					Expect(cnrData.CID).To(Equal("livebeef"))

					cnrConfig, ok := data["config"].(*serializers.ContainerConfig)
					Expect(ok).To(BeTrue())
					Expect(cnrConfig.Mounts).To(Equal([]string{"music"}))
					Expect(cnrConfig.Ports).To(ConsistOf([]string{"private:3000:8080", "public:49123:4552", "public:8081:11211"}))
				}
			})
		})

		Context("when called in async mode and freighter is unsuccessful", func() {
			var asyncParams = &url.Values{
				"async":       []string{"1"},
				"env[]":       []string{"LANG=C"},
				"volume[]":    []string{"music"},
				"port[]":      []string{"3000:8080", "public::4552"},
				"publish_all": []string{"0"},
			}

			BeforeEach(func() {
				mockClient.ReconfigError = errors.New("container blew up")
			})

			It("responds 202 accepted and calls freighter asynchronously", func() {
				doRequest(asyncParams)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				var j struct {
					Container       *serializers.Container       `json:"container"`
					ContainerConfig *serializers.ContainerConfig `json:"config"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.CID).To(Equal("deadbeef"), "container CID should not have changed")
			})

			It("does not update the container record apart from unlocking", func() {
				doRequest(asyncParams)

				Eventually(func() (*time.Time, error) {
					cn, err := container.Find(db, u, containerName)
					return cn.LockedUntil, err
				}).Should(BeNil())
			})

			It("publishes failure event", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(asyncParams)

				evtCh := make(chan *controllers.Event, 1)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerReconfigureFailed {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventContainerReconfigureFailed")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerReconfigureFailed))

					data, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(data["container"]).NotTo(BeNil())
					cnrData, ok := data["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())
					Expect(cnrData.CID).To(Equal("livebeef")) // This should be updated from Freighter.

					errorMsg, ok := data["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(errorMsg).To(Equal("container blew up"))
				}

				cn, err := container.Find(db, u, containerName)
				Expect(err).To(BeNil())
				Expect(cn.CID).To(Equal("livebeef"))
			})

			It("publishes expands the httperror event", func() {
				mockClient.ReconfigError = errors.New("port is already allocated: Bind for 0.0.0.0:8080")

				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(asyncParams)

				evtCh := make(chan *controllers.Event, 1)
				go func() {
					var evt *controllers.Event
					for msg := range ch { // Ignore events we don't care about.
						evt = msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerReconfigureFailed {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventContainerReconfigureFailed")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerReconfigureFailed))

					data, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(data["container"]).NotTo(BeNil())
					cnrData, ok := data["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())
					Expect(cnrData.CID).To(Equal("livebeef")) // This should be updated from Freighter.

					errorMsg, ok := data["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(errorMsg).To(Equal("Container cannot be started, Host port 8080 is already in use by another container."))
				}

				cn, err := container.Find(db, u, containerName)
				Expect(err).To(BeNil())
				Expect(cn.CID).To(Equal("livebeef"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/reconfig", containerName)
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("PUT /containers/{name}/metadata", func() {
		var (
			containerName = "elixir-web-1"
			c             *container.Container
		)

		doRequest := func(params *url.Values) {
			req, err := NewAuthedFormRequest(t, "PUT", fmt.Sprintf("/api/v0/containers/%s/metadata", containerName), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			v := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
			err := v.Insert(db)
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   containerName,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"Original": {"Val", true},
					},
				},
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest(&url.Values{"k": []string{"v"}, "1": []string{"2345"}})
			VerifyDelinquentUserResponse(res)
		})

		It("should allow for setting metadata, keeping existing", func() {
			doRequest(&url.Values{"k": []string{"v"}, "1": []string{"2345"}})
			Expect(res.Code).To(Equal(http.StatusOK))

			c2, err := container.FindByCID(db, c.CID)
			Expect(err).To(BeNil())

			Expect(c2.Metadata.Map["k"].Valid).To(BeTrue())
			Expect(c2.Metadata.Map["k"].String).To(Equal("v"))
			Expect(c2.Metadata.Map["1"].Valid).To(BeTrue())
			Expect(c2.Metadata.Map["1"].String).To(Equal("2345"))
			Expect(c2.Metadata.Map["Original"].String).To(Equal("Val"))
		})

		It("should allow for deleting metadata", func() {
			_, err := db.Exec(`UPDATE containers SET metadata = hstore('key','value') WHERE cid = $1;`, c.CID)
			Expect(err).To(BeNil())

			doRequest(&url.Values{"key": []string{""}, "1": []string{"2345"}})
			Expect(res.Code).To(Equal(http.StatusOK))

			c2, err := container.FindByCID(db, c.CID)
			Expect(err).To(BeNil())

			Expect(c2.Metadata.Map).ToNot(HaveKey("key"))
			Expect(c2.Metadata.Map["1"].Valid).To(BeTrue())
			Expect(c2.Metadata.Map["1"].String).To(Equal("2345"))
		})

		It("should not insert keys longer 255chars", func() {
			t, err := securerandom.Base64(200)
			Expect(err).To(BeNil())
			str := t[:256]

			doRequest(&url.Values{str: []string{"2"}, "allgood": []string{str}})
			Expect(res.Code).To(Equal(http.StatusOK))

			c2, err := container.FindByCID(db, c.CID)
			Expect(err).To(BeNil())

			Expect(c2.Metadata.Map).ToNot(HaveKey(str))
			Expect(c2.Metadata.Map["allgood"].Valid).To(BeTrue())
			Expect(c2.Metadata.Map["allgood"].String).To(Equal(str))
		})
	})

	Describe("PublishContainerEvent", func() {
		var (
			currentUser    *user.User
			containerOwner *user.User
			c              *container.Container
			env            *middleware.Env
		)

		BeforeEach(func() {
			containerOwner = &user.User{
				Username: "owner",
				Password: "123456",
				Email:    "owner@example.com",
			}
			err := WrapInTransaction(db, containerOwner.Insert)
			Expect(err).To(BeNil())

			currentUser = &user.User{
				Username: "currentuser",
				Password: "123456",
				Email:    "currentuser@example.com",
			}
			err = WrapInTransaction(db, currentUser.Insert)
			Expect(err).To(BeNil())
			hs, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:  "host-123",
					Size:  sql.NullString{"micro", true},
					State: host.StateBorn,
				},
				U: containerOwner,
			})
			Expect(err).To(BeNil())
			h := hs.H

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "dominant-intercooler-42",
				Image:  "nitrous-base",
				UserID: containerOwner.ID,
				HostID: h.ID,
				Name:   sql.NullString{"Rails app server", true},
				Metadata: hstore.Hstore{
					Map: map[string]sql.NullString{
						"C": {"1", true},
					},
				},
			}

			err = c.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, currentUser.ID)
			Expect(err).To(BeNil())

			env = &middleware.Env{
				DB:     db,
				User:   currentUser,
				Events: svr.Events,
			}
		})

		It("publishes events to current user", func() {
			ch := svr.Events.Subscribe(currentUser.Topic())
			defer svr.Events.Unsubscribe(ch, currentUser.Topic())

			add := map[string]interface{}{
				"extra": "value",
			}
			containers.PublishContainerEvent(env, controllers.EventContainerStopped, c, add)
			var msg pubsub.Message
			finished := make(chan struct{})
			go func() {
				msg = <-ch
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail(fmt.Sprintf("timeout while waiting for controller events"))
			case <-finished:
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventContainerStopped))

				data, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())

				// Check extra data
				Expect(data["extra"]).To(Equal("value"))

				initiator, ok := data["initiated_by"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(initiator["username"]).To(Equal("currentuser"))
				Expect(initiator["id"]).To(Equal(currentUser.ID))

				cn, ok := data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.ID).To(Equal(c.ID))
			}
		})

		It("publishes events to collaborator", func() {
			ch := svr.Events.Subscribe(containerOwner.Topic())
			defer svr.Events.Unsubscribe(ch, containerOwner.Topic())

			containers.PublishContainerEvent(env, controllers.EventContainerStopped, c, map[string]interface{}{})
			var msg pubsub.Message
			finished := make(chan struct{})
			go func() {
				msg = <-ch
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail(fmt.Sprintf("timeout while waiting for controller events"))
			case <-finished:
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventContainerStopped))

				data, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())

				initiator, ok := data["initiated_by"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(initiator["username"]).To(Equal("currentuser"))
				Expect(initiator["id"]).To(Equal(currentUser.ID))

				cn, ok := data["container"].(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.ID).To(Equal(c.ID))
			}
		})
	})
})

var _ = Describe("FindSSHURL()", func() {
	var originalHostHostname string
	var h *host.Host = &host.Host{
		Slug: "elixir-web-1",
	}

	BeforeEach(func() {
		originalHostHostname = envvars.HostHostname
		envvars.HostHostname = "n2odev.com"
	})

	AfterEach(func() {
		envvars.HostHostname = originalHostHostname
	})

	It("should have valid URL with valid parameters", func() {
		testCases := []struct {
			config      *freighterclient.ContainerConfig
			expectedURL string
		}{
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"public:2020:22/tcp"},
					Env: []string{
						"NITROUS_SSH_ENABLED=true",
						"NITROUS_USERNAME=nitrous",
					},
				},
				"ssh://nitrous@elixir-web-1.n2odev.com:2020",
			},
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"public:2020:22/tcp"},
					Env: []string{
						"NITROUS_SSH_ENABLED=true",
					},
				},
				"ssh://root@elixir-web-1.n2odev.com:2020",
			},
		}

		for _, testCase := range testCases {
			url, err := containers.FindSSHURL(h, testCase.config)
			Expect(err).To(BeNil())
			Expect(url).To(Equal(testCase.expectedURL))
		}
	})

	It("should get error with invalid parameters", func() {
		testCases := []struct {
			config        *freighterclient.ContainerConfig
			expectedError error
		}{
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"public:2020:22/tcp"},
					Env: []string{
						"NITROUS_USERNAME=nitrous",
					},
				},
				containers.ErrSSHNotEnabled,
			},
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"public:2020:22/tcp"},
					Env: []string{
						"NITROUS_SSH_ENABLED=false",
						"NITROUS_USERNAME=nitrous",
					},
				},
				containers.ErrSSHNotEnabled,
			},
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"public:8080:80/tcp"},
					Env: []string{
						"NITROUS_SSH_ENABLED=true",
						"NITROUS_USERNAME=nitrous",
					},
				},
				containers.ErrSSHPortNotFound,
			},
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"private:2020:22/tcp"},
					Env: []string{
						"NITROUS_SSH_ENABLED=true",
						"NITROUS_USERNAME=nitrous",
					},
				},
				containers.ErrSSHPortNotFound,
			},
			{
				&freighterclient.ContainerConfig{
					PortBindings: []string{"public:2020:22/udp"},
					Env: []string{
						"NITROUS_SSH_ENABLED=true",
						"NITROUS_USERNAME=nitrous",
					},
				},
				containers.ErrSSHPortNotFound,
			},
		}

		for _, testCase := range testCases {
			_, err := containers.FindSSHURL(h, testCase.config)
			Expect(err).To(Equal(testCase.expectedError))
		}
	})

	Describe("GetPortMappingsFrom", func() {
		It("returns port mappings from freighterclient container", func() {
			fc := &freighterclient.Container{
				Ports: map[string][]string{
					"3000": []string{"63000"},
					"5000": []string{"8000"},
					"7000": []string{"9000"},
				},
				Env: []string{
					"NITROUS_PREVIEW_PORT=3000",
				},
			}

			var portMappings map[string]string
			portMappigsJson, err := containers.GetPortMappingsJsonFrom(fc)
			Expect(err).To(BeNil())

			err = json.Unmarshal([]byte(portMappigsJson), &portMappings)
			Expect(err).To(BeNil())

			Expect(portMappings).To(HaveLen(4))
			Expect(portMappings["preview"]).To(Equal("63000"))
			Expect(portMappings["3000"]).To(Equal("63000"))
			Expect(portMappings["5000"]).To(Equal("8000"))
			Expect(portMappings["7000"]).To(Equal("9000"))
		})

		It("does not contain preview if NITROUS_PREVIEW_PORT is not specified", func() {
			fc := &freighterclient.Container{
				Ports: map[string][]string{
					"3000": []string{"63000"},
				},
			}

			var portMappings map[string]string
			portMappigsJson, err := containers.GetPortMappingsJsonFrom(fc)
			Expect(err).To(BeNil())

			err = json.Unmarshal([]byte(portMappigsJson), &portMappings)
			Expect(err).To(BeNil())

			Expect(portMappings).To(HaveLen(1))
			Expect(portMappings["3000"]).To(Equal("63000"))
		})

		It("does not contain preview if mapping port is empty", func() {
			fc := &freighterclient.Container{
				Ports: map[string][]string{
					"3000": []string{},
					"5000": []string{"8000"},
					"7000": []string{"9000"},
				},
				Env: []string{
					"NITROUS_PREVIEW_PORT=3000",
				},
			}

			var portMappings map[string]string
			portMappigsJson, err := containers.GetPortMappingsJsonFrom(fc)
			Expect(err).To(BeNil())

			err = json.Unmarshal([]byte(portMappigsJson), &portMappings)
			Expect(err).To(BeNil())

			Expect(portMappings).To(HaveLen(2))
			Expect(portMappings["5000"]).To(Equal("8000"))
			Expect(portMappings["7000"]).To(Equal("9000"))
		})

		It("returns empty array if no exposed ports", func() {
			fc := &freighterclient.Container{
				Ports: map[string][]string{},
				Env: []string{
					"NITROUS_PREVIEW_PORT=3000",
				},
			}

			var portMappings map[string]string
			portMappigsJson, err := containers.GetPortMappingsJsonFrom(fc)
			Expect(err).To(BeNil())

			err = json.Unmarshal([]byte(portMappigsJson), &portMappings)
			Expect(err).To(BeNil())

			Expect(portMappings).To(HaveLen(0))
		})
	})
})
