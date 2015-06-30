package hosts_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/hosts"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"

	test "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	// shorten key length so tests run faster
	cert.KeyLen = 512
	keypairs.KeyLen = 512
}

func queryHost(db *ndb.Database, id int64) (*host.Host, error) {
	h := &host.Host{}
	if err := db.Get(h, `SELECT * FROM hosts WHERE id = $1`, id); err != nil {
		return nil, err
	}
	return h, nil
}

func hostname(slug string) string {
	return fmt.Sprintf("%s.%s", slug, envvars.HostHostname)
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
}

var _ = Describe("controllers/hosts", func() {
	var (
		db              *ndb.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs test.AuthTokenInputs

		u  *user.User
		t  *token.Token
		ft *test.FakeTracker

		// Save original roundtripper of "global" hostbuilderclient to be
		// restored after each test.
		origHostbuilderRT = hosts.HostbuilderClient.Client.Client.Transport
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = server.NewServer("", db, nil)
		svr.Tracker = ft
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
		Expect(u.ID).NotTo(BeZero())

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
		replaceTransport(hosts.HostbuilderClient, origHostbuilderRT)
	})

	var createCollaborator = func(hs *test.HostStack) (*user.User, *token.Token) {
		recv := &user.User{
			Username: "shared",
			Email:    "shared@example.com",
			Password: "123456",
		}
		err := test.WrapInTransaction(db, recv.Insert)
		Expect(err).To(BeNil())

		tok := &token.Token{
			UserID: recv.ID,
			Token:  "recv",
		}
		err = tok.Insert(db)
		Expect(err).To(BeNil())

		c := &container.Container{
			CID:    "thingo3",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "coffee-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())

		_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, recv.ID)
		Expect(err).To(BeNil())

		return recv, tok
	}

	var checkStoppedEvent = func(evt *controllers.Event) string {
		Expect(evt.Name).To(Equal(controllers.EventContainerStopped))
		evtData, ok := evt.Data.(map[string]interface{})
		Expect(ok).To(BeTrue())
		cn, ok := evtData["container"].(*serializers.Container)
		Expect(cn.State).To(Equal(container.StateStopped.String()))
		return cn.Slug
	}

	Describe("GET /api/v0/hosts", func() {
		doRequest := func() {
			req, err := test.NewRequest("GET", "/api/v0/hosts", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when user has hosts", func() {
			var hs1, hs2 *test.HostStack

			BeforeEach(func() {
				var err error
				hs1, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:           "foo",
						IPAddress:      sql.NullString{"1.2.3.4", true},
						FreighterdPort: sql.NullInt64{64000, true},
						Size:           sql.NullString{"micro", true},
						State:          host.StateRunning,
					},
					U: u,
				})
				Expect(err).To(BeNil())

				hs2, err = test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:           "bar",
						IPAddress:      sql.NullString{"2.4.6.8", true},
						FreighterdPort: sql.NullInt64{64001, true},
						Size:           sql.NullString{"large", true},
						State:          host.StateRunning,
					},
					U: u,
				})
				Expect(err).To(BeNil())
			})

			It("should not be blocked if the user has billing issues", func() {
				test.MarkDelinquentUser(db, u)
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("responds with the authenticated user's hosts", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Hosts []*serializers.Host `json:"hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Hosts).To(HaveLen(2))
				Expect(j.Hosts[0].ID).To(Equal(hs1.H.ID))
				Expect(j.Hosts[0].UserID).To(Equal(hs1.U.ID))
				Expect(j.Hosts[0].Slug).To(Equal("foo"))
				Expect(j.Hosts[0].IPAddress).To(Equal("1.2.3.4"))
				Expect(j.Hosts[0].FreighterdPort).To(Equal(int64(64000)))
				Expect(j.Hosts[0].Size).To(Equal("micro"))
				Expect(j.Hosts[0].State).To(Equal(host.StateRunning))
				Expect(j.Hosts[0].Hostname).To(Equal(hostname("foo")))
				Expect(j.Hosts[0].CreatedAt).NotTo(BeNil())

				Expect(j.Hosts[1].ID).To(Equal(hs2.H.ID))
				Expect(j.Hosts[1].UserID).To(Equal(hs2.U.ID))
				Expect(j.Hosts[1].Slug).To(Equal("bar"))
				Expect(j.Hosts[1].IPAddress).To(Equal("2.4.6.8"))
				Expect(j.Hosts[1].FreighterdPort).To(Equal(int64(64001)))
				Expect(j.Hosts[1].Size).To(Equal("large"))
				Expect(j.Hosts[1].State).To(Equal(host.StateRunning))
				Expect(j.Hosts[1].Hostname).To(Equal(hostname("bar")))
				Expect(j.Hosts[1].CreatedAt).NotTo(BeNil())
			})
		})

		Context("when user has no hosts", func() {
			It("responds with an empty array", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Hosts []*serializers.Host `json:"hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Hosts).To(BeEmpty())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/hosts/{slug}", func() {
		var hs *test.HostStack
		var mockClient *freighterclient.MockClient

		doRequest := func(slug, token string) {
			req, err := test.NewRequest("GET", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			mockClient = &freighterclient.MockClient{}
			mockClient.HostVersionsReturn = &freighterclient.HostVersions{
				DockerVersion:     "1.5.0-nitrous1",
				FreighterdVersion: "1.0.1-deadbeef",
			}
			test.StubDefaultFreighterFn(mockClient)

			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:              "foo",
					IPAddress:         sql.NullString{"1.2.3.4", true},
					FreighterdPort:    sql.NullInt64{64000, true},
					Size:              sql.NullString{"large", true},
					State:             host.StateRunning,
					FreighterdVersion: "1.0.1-deadbeef-1",
					DockerVersion:     "1.5.0-nitrous1",
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			test.UnstubFreighterFn()
		})

		It("should allow the user even if it has billing issues", func() {
			test.MarkDelinquentUser(db, u)
			doRequest(hs.H.Slug, t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("responds with host info from the database", func() {
			doRequest(hs.H.Slug, t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
			Expect(j.Host.UserID).To(Equal(hs.U.ID))
			Expect(j.Host.Slug).To(Equal("foo"))
			Expect(j.Host.IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Host.FreighterdPort).To(Equal(int64(64000)))
			Expect(j.Host.Size).To(Equal("large"))
			Expect(j.Host.State).To(Equal(host.StateRunning))
			Expect(j.Host.Hostname).To(Equal(hostname("foo")))
			Expect(j.Host.RegionID).To(Equal(hs.R.ID))
			Expect(j.Host.PlanID).To(Equal(hs.P.ID))
			Expect(j.Host.CreatedAt).NotTo(BeNil())
			Expect(j.Host.UpdatedAt).NotTo(BeNil())
			Expect(j.Host.TerminatedAt).To(BeNil())
			Expect(j.Host.LiveDockerVersion).To(Equal(""))
			Expect(j.Host.LiveFreighterdVersion).To(Equal(""))
			Expect(j.Host.UpdateAvailable).To(Equal(host.UpdateAvailableSoft))
			Expect(j.Host.PendingRespec).To(Equal(false))
		})

		It("responds with pending_respec set to true if host has a pending respec record", func() {
			hs2, err := test.CreateHostStack(db, &test.HostStack{
				R: hs.R,
			})
			Expect(err).To(BeNil())

			tx, err := db.Beginx()
			Expect(err).To(BeNil())
			r := &respec.Respec{
				HostID:             hs.H.ID,
				ProviderTemplateID: hs2.PT.ID,
			}
			Expect(r.Insert(tx)).To(BeNil())
			Expect(tx.Commit()).To(BeNil())

			doRequest(hs.H.Slug, t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.PendingRespec).To(Equal(true))
		})

		It("responds with an empty update_available if required freighterd & docker version is empty", func() {
			hs.H.DockerVersion = ""
			hs.H.FreighterdVersion = ""
			err := hs.H.Update(db)
			Expect(err).To(BeNil())

			doRequest(hs.H.Slug, t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.UpdateAvailable).To(Equal(""))
		})

		It("should return with a `hard` update_available if the docker versions do not match", func() {
			hs.H.DockerVersion = "1.2.0-nitrous1"
			hs.H.FreighterdVersion = ""
			err := hs.H.Update(db)
			Expect(err).To(BeNil())

			doRequest(hs.H.Slug, t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.UpdateAvailable).To(Equal(host.UpdateAvailableHard))
		})

		It("when self upgrade is disabled, returns empty update_available even if versions do not match", func() {
			envvars.EnableSelfUpgrade = false
			defer func() {
				envvars.EnableSelfUpgrade = true
			}()

			hs.H.DockerVersion = "1.2.0-nitrous1"
			hs.H.FreighterdVersion = "1.0.1-abcde"
			err := hs.H.Update(db)
			Expect(err).To(BeNil())

			doRequest(hs.H.Slug, t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.UpdateAvailable).To(Equal(""))
		})

		It("responds with host info even when called with the host ID", func() {
			doRequest(strconv.FormatInt(hs.H.ID, 10), t.Token)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
		})

		Context("when the host does not belong to the user", func() {
			var (
				anotherUser  *user.User
				anotherToken *token.Token
			)

			BeforeEach(func() {
				anotherUser = &user.User{
					Username: "the-others",
					Password: "winter iz coming",
					Email:    "the-others@the-north.gov",
				}
				err := test.WrapInTransaction(db, anotherUser.Insert)
				Expect(err).To(BeNil())

				anotherToken = &token.Token{
					UserID: anotherUser.ID,
					Token:  "another-users-token",
				}
				err = anotherToken.Insert(db)
				Expect(err).To(BeNil())
			})

			It("responds with 404", func() {
				doRequest(hs.H.Slug, anotherToken.Token)

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("responds with the host record augmented with Docker and Freighter versions when user is an admin", func() {
				_, err := db.Exec("UPDATE users SET admin = true WHERE id = $1", anotherUser.ID)
				Expect(err).To(BeNil())

				doRequest(hs.H.Slug, anotherToken.Token)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))

				Expect(mockClient.HostVersionsCalled).To(Equal(1))
				Expect(j.Host.LiveDockerVersion).To(Equal("1.5.0-nitrous1"))
				Expect(j.Host.LiveFreighterdVersion).To(Equal("1.0.1-deadbeef"))
				Expect(j.Host.UpdateAvailable).To(Equal(host.UpdateAvailableSoft))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})

		Context("container collaborator", func() {
			var (
				recv *user.User
				tok  *token.Token
			)

			BeforeEach(func() {
				recv, tok = createCollaborator(hs)
			})

			It("responds with host info from the database if the user is a collaborator", func() {
				doRequest(hs.H.Slug, tok.Token)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
			})

			It("responds with 404 if the user is not a collaborator", func() {
				_, err := db.Exec(`DELETE FROM container_collabs WHERE user_id = $1;`, recv.ID)
				Expect(err).To(BeNil())

				doRequest(hs.H.Slug, tok.Token)

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})
	})

	Describe("POST /api/v0/hosts/refresh", func() {
		doRequest := func() {
			req, err := test.NewAuthedRequest(t, "POST", "/api/v0/hosts/refresh", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("publishes hosts refreshed events", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timed out waiting for hosts refreshed event")
			case msg := <-ch:
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventHostsRefreshed))
			}
		})
	})

	Describe("PUT /api/v0/hosts/{slug}", func() {
		var hs *test.HostStack

		doRequest := func(params *url.Values) {
			req, err := test.NewAuthedFormRequest(t, "PUT", "/api/v0/hosts/foo", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:           "foo",
					Name:           sql.NullString{"foohost", true},
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					Size:           sql.NullString{"large", true},
					State:          host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		It("should block if the user has billing issues", func() {
			test.MarkDelinquentUser(db, u)
			params := &url.Values{}
			doRequest(params)
			test.VerifyDelinquentUserResponse(res)
		})

		Context("when name field is present and non-empty", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("name", "GhostHost")
				doRequest(params)
			})

			It("updates name", func() {
				updated, err := queryHost(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal("GhostHost"))
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *hs.H.UpdatedAt))
			})
		})

		Context("when name field is present and empty", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("name", "")
				doRequest(params)
			})

			It("nullifies name", func() {
				updated, err := queryHost(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal(""))
				Expect(updated.Name.Valid).To(BeFalse())
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *hs.H.UpdatedAt))
			})
		})

		Context("when unused fields are given", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("slug", "younochange")
				doRequest(params)
			})

			It("does not update fields", func() {
				updated, err := queryHost(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal("foohost"))
				Expect(updated.Slug).To(Equal("foo"))
				Expect(updated.UpdatedAt).To(Equal(hs.H.UpdatedAt))
			})
		})

		It("publishes a host updated event with the updated host", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			params := &url.Values{}
			params.Set("name", "GhostHost")
			doRequest(params)

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timed out waiting for host updated event")
			case msg := <-ch:
				updated, err := queryHost(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(msg).To(Equal(&controllers.Event{
					Name: controllers.EventHostUpdated,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(updated),
					},
				}))
			}
		})
	})

	Describe("GET /api/v0/hosts/{slug}/announcements", func() {
		var (
			err    error
			h1, h2 *host.Host
			expiry time.Time
		)

		doRequest := func(slug string) {
			url := fmt.Sprintf("/api/v0/hosts/%s/announcements", slug)
			req, err := test.NewRequest("GET", url, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			expiry = time.Now().Add(5 * time.Second)

			h1 = &host.Host{
				Slug: "elixir-web-1",
			}

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: h1,
			})
			Expect(err).To(BeNil())

			h2 = &host.Host{
				Slug: "gopher-bar-1",
			}

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: h2,
			})
			Expect(err).To(BeNil())

			hostAnn1 := &announcement.Announcement{
				SubjectTemplate: "Host announcement for {{ .Slug }}",
				MessageTemplate: "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} next week.",
				ShowToAll:       false,
				ExpiresAt:       pq.NullTime{expiry, true},
			}

			err = announcement.InsertForHosts(db, hostAnn1, []int64{h1.ID})
			Expect(err).To(BeNil())

			globalAnn := &announcement.Announcement{
				SubjectTemplate: "Global announcement for {{ .Slug }}",
				MessageTemplate: "Hello {{ .Username }}, We will be upgrading your host {{ .Slug }} tomorrow.",
				ShowToAll:       true,
				ExpiresAt:       pq.NullTime{expiry, true},
			}

			err = announcement.Insert(db, globalAnn)
			Expect(err).To(BeNil())

			hostAnn2 := &announcement.Announcement{
				SubjectTemplate: "Another host announcement for {{ .Slug }}",
				MessageTemplate: "Hey again {{ .Username }}, This is a different announcement regarding your host {{ .Slug }}",
				ShowToAll:       false,
				ExpiresAt:       pq.NullTime{expiry, true},
			}

			err = announcement.InsertForHosts(db, hostAnn2, []int64{h2.ID})
			Expect(err).To(BeNil())
		})

		It("Host h1 should get globalAnn and hostAnn1", func() {
			doRequest("elixir-web-1")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Announcements []*serializers.HostAnnouncement `json:"announcements"`
			}

			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			ann := j.Announcements
			Expect(len(ann)).To(Equal(2))

			// Most recent first
			Expect(*ann[0].Subject).To(Equal("Global announcement for elixir-web-1"))
			Expect(*ann[0].Message).To(Equal("Hello foo, We will be upgrading your host elixir-web-1 tomorrow."))

			Expect(*ann[1].Subject).To(Equal("Host announcement for elixir-web-1"))
			Expect(*ann[1].Message).To(Equal("Hello foo, We will be upgrading your host elixir-web-1 next week."))
		})

		It("Host h2 should get globalAnn and hostAnn2", func() {
			doRequest("gopher-bar-1")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Announcements []*serializers.HostAnnouncement `json:"announcements"`
			}

			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			ann := j.Announcements
			Expect(len(ann)).To(Equal(2))

			// Most recent first
			Expect(*ann[0].Subject).To(Equal("Another host announcement for gopher-bar-1"))
			Expect(*ann[0].Message).To(Equal("Hey again foo, This is a different announcement regarding your host gopher-bar-1"))

			Expect(*ann[1].Subject).To(Equal("Global announcement for gopher-bar-1"))
			Expect(*ann[1].Message).To(Equal("Hello foo, We will be upgrading your host gopher-bar-1 tomorrow."))
		})
	})

	Describe("GET /api/v0/hosts/{slug}/stats", func() {
		var (
			mockClient *freighterclient.MockClient
			hs         *test.HostStack
		)

		doRequest := func(period string) {
			var params *url.Values
			if period != "" {
				params = &url.Values{"period": []string{period}}
			}
			req, err := test.NewRequest("GET", "/api/v0/hosts/foo/stats", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error

			mockClient = &freighterclient.MockClient{}
			mockClient.HostInfoReturn = &freighterclient.HostInfo{
				Uptime:       30,
				MemoryUsage:  12345678,
				MemoryTotal:  17179869184,
				StorageUsage: 2000000000,
				StorageTotal: 1000000000000,
				NumCPU:       2,
				PerCPUUsage:  []float64{0.5, 30},
			}
			test.StubDefaultFreighterFn(mockClient)

			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:      "foo",
					IPAddress: sql.NullString{"1.2.3.4", true},
					State:     host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			test.UnstubFreighterFn()
		})

		It("should allow the user even if it has billing issues", func() {
			test.MarkDelinquentUser(db, u)
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("calls the Freighter client to get host info", func() {
			doRequest("")
			Expect(mockClient.HostInfoCalled).To(Equal(1))
			Expect(mockClient.HostInfoPeriod).To(Equal(100 * time.Millisecond)) // Default is 100ms if not specified.

			doRequest("2s")
			Expect(mockClient.HostInfoCalled).To(Equal(2))
			Expect(mockClient.HostInfoPeriod).To(Equal(2 * time.Second))
		})

		It("responds with host stats from the Freighter API", func() {
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Stats *serializers.HostInfo `json:"stats"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Stats.Uptime).To(Equal(uint64(30)))
			Expect(j.Stats.MemoryUsage).To(Equal(uint64(12345678)))
			Expect(j.Stats.MemoryTotal).To(Equal(uint64(17179869184)))
			Expect(j.Stats.StorageUsage).To(Equal(uint64(2000000000)))
			Expect(j.Stats.StorageTotal).To(Equal(uint64(1000000000000)))
			Expect(j.Stats.NumCPU).To(Equal(2))
			Expect(j.Stats.PerCPUUsage).To(Equal([]float64{0.5, 30}))
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				test.UnstubFreighterFn()
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("workspace is not running"))
			})
		})

		Context("when host does not have an IP address", func() {
			BeforeEach(func() {
				hs.H.IPAddress = sql.NullString{}
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				test.UnstubFreighterFn()
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("workspace is not running"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/stats"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}", func() {
		var (
			hs *test.HostStack
			c  *container.Container
		)

		doRequest := func(slug, token string) {
			req, err := test.NewRequest("DELETE", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:               "honeybadger",
					ProviderIdentifier: sql.NullString{"001", true},
					IPAddress:          sql.NullString{"10.0.0.1", true},
					RegionIdentifier:   sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "beefface",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Slug:   "beef-jerky",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should block if the user has billing issues", func() {
			test.MarkDelinquentUser(db, u)
			doRequest(hs.H.Slug, t.Token)
			test.VerifyDelinquentUserResponse(res)
		})

		It("should request to cancel the subscription if the host is part of a subscription", func() {
			fsub := &test.FakeSubscriptionProvider{}
			hosts.SetSubscriptionProvider(fsub)

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           hs.P.ID,
				BillingProfileID: bp.ID,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, t.Token)

			Expect(rt.Requests).To(HaveLen(0))

			Expect(fsub.CancelCalled).To(BeTrue())
			Expect(fsub.CancelSubscription.ID).To(Equal(sub.ID))
		})

		It("should request to cancel the subscription & terminate the host if the host is a free subscription", func() {
			fsub := &test.FakeSubscriptionProvider{}
			hosts.SetSubscriptionProvider(fsub)

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			sub := &subscription.Subscription{
				StripeID:         sql.NullString{"abc1", true},
				PlanID:           hs.P.ID,
				BillingProfileID: bp.ID,
			}
			Expect(test.WrapInTransaction(db, sub.Insert)).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET subscription_id = $1, free = TRUE WHERE id = $2;", sub.ID, hs.H.ID)
			Expect(err).To(BeNil())

			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, t.Token)

			Expect(rt.Requests).To(HaveLen(1))
			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
			Expect(j.Host.State).To(Equal(host.StateTerminated))

			Expect(fsub.CancelCalled).To(BeTrue())
			Expect(fsub.CancelSubscription.ID).To(Equal(sub.ID))
		})

		It("should send the UUID headers", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, t.Token)

			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())
		})

		It("sends the correct payload to hostbuilder", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, t.Token)

			payload := &struct {
				ID        string `json:"id"`
				IPAddress string `json:"ip_address"`
				Region    string `json:"region_id"`
			}{}
			json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

			Expect(payload.ID).To(Equal(hs.H.ProviderIdentifier.String), "invalid id")
			Expect(payload.IPAddress).To(Equal(hs.H.IPAddress.String), "invalid ip_address")
			Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
		})

		Context("when hostbuilder successfully deletes the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb
			})

			It("deletes the host record", func() {
				doRequest(hs.H.Slug, t.Token)
				_, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))

				// and tracks on mixpanel
				Expect(ft.PropsCalled).To(HaveLen(1))
				pc := ft.PropsCalled[0]
				Expect(pc.UserID).To(Equal(hs.H.UserID))
				Expect(pc.Mode).To(Equal("set"))
				Expect(pc.PropsSet["workspaces_plans"]).To(HaveLen(0))
			})

			It("deletes all container records belonging to the host", func() {
				c2, err := container.Find(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(Equal(c))

				doRequest(hs.H.Slug, t.Token)

				c2, err = container.Find(db, hs.U, c.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c2).To(BeNil())
			})

			It("deletes keypairs associated to the host", func() {
				priv, _ := keypairs.GenerateKey()
				tx, _ := db.Beginx()
				keypairs.Create(tx, priv, "aes_key", hs.H.ID)
				tx.Commit()

				doRequest(hs.H.Slug, t.Token)

				_, err := keypairs.Get(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "host's keypairs should be removed")
			})

			It("deletes certs associated to the host", func() {
				tx, _ := db.Beginx()
				crt := &cert.Cert{
					ServerCert: "servercert",
					ServerKey:  "serverkey",
				}
				Expect(crt.Insert(tx, hs.H.ID)).To(BeNil())
				tx.Commit()

				doRequest(hs.H.Slug, t.Token)
				_, err := cert.Find(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "failed to remove host's certificates")
			})

			It("responds with status OK", func() {
				doRequest(hs.H.Slug, t.Token)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal("honeybadger"))
				Expect(j.Host.IPAddress).To(Equal("10.0.0.1"))
				Expect(j.Host.State).To(Equal(host.StateTerminated))
				Expect(j.Host.Hostname).To(Equal(hostname("honeybadger")))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).NotTo(BeNil())
			})

			It("publishes a host destroyed event with the destroyed host", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, t.Token)

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for host destroyed event")
				case msg := <-ch:
					deleted, err := queryHost(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(msg).To(Equal(&controllers.Event{
						Name: controllers.EventHostDestroyed,
						Data: map[string]*serializers.Host{
							"host": serializers.SerializeHost(deleted),
						},
					}))
				}
			})

			It("logs transitions for the host", func() {
				origState := hs.H.State
				doRequest(hs.H.Slug, t.Token)
				Expect(res.Code).To(Equal(http.StatusOK))

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateTerminated, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusBadRequest,
				})
				hosts.HostbuilderClient = hb
			})

			It("does not delete the host", func() {
				doRequest(hs.H.Slug, t.Token)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest(hs.H.Slug, t.Token)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("when the host does not belong to the user", func() {
			var (
				anotherUser  *user.User
				anotherToken *token.Token
			)

			BeforeEach(func() {
				anotherUser = &user.User{
					Username: "the-others",
					Password: "winter iz coming",
					Email:    "the-others@the-north.gov",
				}
				err := test.WrapInTransaction(db, anotherUser.Insert)
				Expect(err).To(BeNil())

				anotherToken = &token.Token{
					UserID: anotherUser.ID,
					Token:  "another-users-token",
				}
				err = anotherToken.Insert(db)
				Expect(err).To(BeNil())
			})

			It("responds with 404", func() {
				doRequest(hs.H.Slug, anotherToken.Token)

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("deletes the host record when user is an admin", func() {
				_, err := db.Exec("UPDATE users SET admin = true WHERE id = $1", anotherUser.ID)
				Expect(err).To(BeNil())

				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug, anotherToken.Token)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
			})

			It("reassigns the host when `reassign` is sent", func() {
				_, err := db.Exec("UPDATE users SET admin = true WHERE id = $1", anotherUser.ID)
				Expect(err).To(BeNil())

				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				params := &url.Values{}
				params.Set("reassign", "true")

				req, err := test.NewRequest("DELETE", "/api/v0/hosts/"+hs.H.Slug, params)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", anotherToken.Token))
				handler.ServeHTTP(res, req)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))

				var count int
				err = db.QueryRowx(
					"SELECT COUNT(*) FROM hosts WHERE user_id = $1 AND id = $2 AND state = $3",
					anotherUser.ID, hs.H.ID, host.StateTerminated).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("when host is provider identifier is nil", func() {
			It("should not make a request to hostbuilder", func() {
				hs.H.ProviderIdentifier = sql.NullString{String: "", Valid: false}
				hs.H.Update(db)

				// in this context, hostbuilder should not be called
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt := &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusBadRequest,
				}
				replaceTransport(hb, rt)
				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug, t.Token)
				Expect(len(rt.Requests)).To(Equal(0), "expect to not called hostbuilder")
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "DELETE"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/start", func() {
		var hs *test.HostStack

		doRequest := func(slug string) {
			req, err := test.NewRequest("POST", "/api/v0/hosts/"+slug+"/start", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					State:            host.StateStopped,
					IPAddress:        sql.NullString{"10.0.0.1", true},
					RegionIdentifier: sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		It("should send the UUID headers", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug)

			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())
		})

		It("sends the correct payload to hostbuilder", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug)

			payload := &struct {
				ID        string `json:"id"`
				IPAddress string `json:"ip_address"`
				Region    string `json:"region_id"`
			}{}
			json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

			Expect(payload.ID).To(Equal(hs.H.ProviderIdentifier.String), "invalid id")
			Expect(payload.IPAddress).To(Equal(hs.H.IPAddress.String), "invalid ip_address")
			Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
		})

		Context("when host is not in the stopped state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateRestarting
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("workspace is not stopped"))
			})
		})

		Context("when hostbuilder successfully starts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb
			})

			It("updates the host state to running", func() {
				doRequest(hs.H.Slug)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes host starting and started events", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				msgs := []pubsub.Message{}
				finished := make(chan struct{})
				go func() {
					msgs = append(msgs, <-ch, <-ch)
					close(finished)
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out while waiting for events")
				case <-finished:
					evt, ok := msgs[0].(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(evt.Name).To(Equal(controllers.EventHostStarting))
					evtData, ok := evt.Data.(map[string]*serializers.Host)
					Expect(ok).To(BeTrue())
					Expect(evtData["host"].Slug).To(Equal(hs.H.Slug))
					Expect(evtData["host"].State).To(Equal(host.StateStarting))

					h, err := host.Find(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(msgs[1]).To(Equal(&controllers.Event{
						Name: controllers.EventHostStarted,
						Data: map[string]*serializers.Host{
							"host": serializers.SerializeHost(h),
						},
					}))
				}
			})

			It("logs transitions for the host", func() {
				origState := hs.H.State

				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateStarting, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateRunning, host.StateStarting).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("updates software on the host", func() {
				mockClient := &freighterclient.MockClient{}
				test.StubDefaultFreighterFn(mockClient)
				defer test.UnstubFreighterFn()

				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.UpgradeServerCalled).To(Equal(1))

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateUpdating))
			})

			It("doesn't do anything if host has already been updated", func() {
				mockClient := &freighterclient.MockClient{}
				mockClient.UpgradeServerError = freighterclient.ErrAlreadyUpgraded
				test.StubDefaultFreighterFn(mockClient)
				defer test.UnstubFreighterFn()

				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.UpgradeServerCalled).To(Equal(1))

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("doesn't do anything if software update fails", func() {
				mockClient := &freighterclient.MockClient{}
				mockClient.UpgradeServerError = errors.New("host cannot be updated")
				test.StubDefaultFreighterFn(mockClient)
				defer test.UnstubFreighterFn()

				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.UpgradeServerCalled).To(Equal(1))

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusBadRequest,
				})
				hosts.HostbuilderClient = hb
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("resets the host to its original state", func() {
				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopped))
			})

			It("logs transitions for the host", func() {
				origState := hs.H.State
				doRequest(hs.H.Slug)

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateStarting, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, origState, host.StateStarting).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/start"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/stop", func() {
		var (
			hs  *test.HostStack
			tok *token.Token
		)

		doRequest := func(slug string, params *url.Values) {
			req, err := test.NewFormRequest("POST", "/api/v0/hosts/"+slug+"/stop", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+tok.Token)
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					State:            host.StateRunning,
					IPAddress:        sql.NullString{"127.0.0.1", true},
					FreighterdPort:   sql.NullInt64{65333, true},
					RegionIdentifier: sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())
			tok = t
		})

		It("should send the UUID headers", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: stopOkResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, nil)

			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())
		})

		It("sends the correct payload to hostbuilder", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: stopOkResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, nil)

			payload := &struct {
				ID        string `json:"id"`
				IPAddress string `json:"ip_address"`
				Region    string `json:"region_id"`
			}{}
			json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

			Expect(payload.ID).To(Equal(hs.H.ProviderIdentifier.String), "invalid id")
			Expect(payload.IPAddress).To(Equal(hs.H.IPAddress.String), "invalid ip_address")
			Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
		})

		Context("when host is already stopped", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: stopOkResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())

				doRequest(hs.H.Slug, nil)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("workspace is not running"))
			})

			It("allows host to be stopped if force=true", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: stopOkResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopping
				err := hs.H.Update(db)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("force", "true")
				doRequest(hs.H.Slug, params)

				Expect(res.Code).To(Equal(http.StatusAccepted))
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopping))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/stop"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})

		It("should not be accessible by a collaborator", func() {
			_, t := createCollaborator(hs)
			tok = t

			doRequest(hs.H.Slug, nil)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		Describe("when host has stopping containers", func() {
			var c1, c2 *container.Container
			BeforeEach(func() {
				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					State:  container.StateRunning,
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
				c1.TempState = sql.NullInt64{int64(container.StateStopping), true}
				err = c1.Update(db)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest(hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_container_state"))
				Expect(errDesc).To(Equal("containers(rails-box) are not stopped"))
			})

			Context("force is true", func() {
				var params *url.Values
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					replaceTransport(hb, &test.FakeRoundTripper{
						Message: stopOkResponse,
						Status:  http.StatusOK,
					})
					hosts.HostbuilderClient = hb

					hs.H.State = host.StateStopping
					err := hs.H.Update(db)
					Expect(err).To(BeNil())

					params = &url.Values{}
					params.Set("force", "true")
				})

				It("stops host", func() {
					doRequest(hs.H.Slug, params)

					Expect(res.Code).To(Equal(http.StatusAccepted))
					h, err := host.Find(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateStopping))
				})
			})
		})

		Describe("when host has starting containers", func() {
			var c1, c2 *container.Container
			BeforeEach(func() {
				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					State:  container.StateRunning,
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
				c1.TempState = sql.NullInt64{int64(container.StateStarting), true}
				err = c1.Update(db)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest(hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_container_state"))
				Expect(errDesc).To(Equal("containers(rails-box) are not stopped"))
			})

			Context("force is true", func() {
				var params *url.Values
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					replaceTransport(hb, &test.FakeRoundTripper{
						Message: stopOkResponse,
						Status:  http.StatusOK,
					})
					hosts.HostbuilderClient = hb

					hs.H.State = host.StateStopping
					err := hs.H.Update(db)
					Expect(err).To(BeNil())

					params = &url.Values{}
					params.Set("force", "true")
				})

				It("stops host", func() {
					doRequest(hs.H.Slug, params)

					Expect(res.Code).To(Equal(http.StatusAccepted))
					h, err := host.Find(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateStopping))
				})
			})
		})
	})

	Describe("POST /api/v0/hosts/{slug}/restart", func() {
		var hs *test.HostStack

		doRequest := func(slug, token string, params *url.Values) {
			req, err := test.NewFormRequest("POST", "/api/v0/hosts/"+slug+"/restart", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					State:            host.StateRunning,
					IPAddress:        sql.NullString{"127.0.0.1", true},
					RegionIdentifier: sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		It("should send the UUID headers", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, t.Token, nil)

			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())
		})

		It("sends the correct payload to hostbuilder", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			doRequest(hs.H.Slug, t.Token, nil)

			payload := &struct {
				ID        string `json:"id"`
				IPAddress string `json:"ip_address"`
				Region    string `json:"region_id"`
			}{}
			json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

			Expect(payload.ID).To(Equal(hs.H.ProviderIdentifier.String), "invalid id")
			Expect(payload.IPAddress).To(Equal(hs.H.IPAddress.String), "invalid ip_address")
			Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
		})

		Context("when host is not in the running state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())

				doRequest(hs.H.Slug, t.Token, nil)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("workspace is not running"))
			})

			It("allows host to be restarted if force=true", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				hs.H.State = host.StateRestarting
				err := hs.H.Update(db)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("force", "true")
				doRequest(hs.H.Slug, t.Token, params)

				Expect(res.Code).To(Equal(http.StatusOK))
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})
		})

		Context("when hostbuilder successfully restarts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb
			})

			It("updates the host state to running", func() {
				doRequest(hs.H.Slug, t.Token, nil)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes host restarting and restarted events", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, t.Token, nil)

				msgs := []pubsub.Message{}
				finished := make(chan struct{})
				go func() {
					msgs = append(msgs, <-ch, <-ch)
					close(finished)
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out while waiting for events")
				case <-finished:
					evt, ok := msgs[0].(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(evt.Name).To(Equal(controllers.EventHostRestarting))
					evtData, ok := evt.Data.(map[string]*serializers.Host)
					Expect(ok).To(BeTrue())
					Expect(evtData["host"].Slug).To(Equal(hs.H.Slug))
					Expect(evtData["host"].State).To(Equal(host.StateRestarting))

					h, err := host.Find(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(msgs[1]).To(Equal(&controllers.Event{
						Name: controllers.EventHostRestarted,
						Data: map[string]*serializers.Host{
							"host": serializers.SerializeHost(h),
						},
					}))
				}
			})

			It("logs transitions for the host", func() {
				origState := hs.H.State
				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateRestarting, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateRunning, host.StateRestarting).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("updates software on the host", func() {
				mockClient := &freighterclient.MockClient{}
				test.StubDefaultFreighterFn(mockClient)
				defer test.UnstubFreighterFn()

				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.UpgradeServerCalled).To(Equal(1))

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateUpdating))
			})

			It("doesn't do anything if host has already been updated", func() {
				mockClient := &freighterclient.MockClient{}
				mockClient.UpgradeServerError = freighterclient.ErrAlreadyUpgraded
				test.StubDefaultFreighterFn(mockClient)
				defer test.UnstubFreighterFn()

				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.UpgradeServerCalled).To(Equal(1))

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("doesn't do anything if software update fails", func() {
				mockClient := &freighterclient.MockClient{}
				mockClient.UpgradeServerError = errors.New("host cannot be updated")
				test.StubDefaultFreighterFn(mockClient)
				defer test.UnstubFreighterFn()

				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.UpgradeServerCalled).To(Equal(1))

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})
		})

		Context("when the host does not belong to the user", func() {
			var (
				anotherUser  *user.User
				anotherToken *token.Token
			)

			BeforeEach(func() {
				anotherUser = &user.User{
					Username: "the-others",
					Password: "winter iz coming",
					Email:    "the-others@the-north.gov",
				}
				err := test.WrapInTransaction(db, anotherUser.Insert)
				Expect(err).To(BeNil())

				anotherToken = &token.Token{
					UserID: anotherUser.ID,
					Token:  "another-users-token",
				}
				err = anotherToken.Insert(db)
				Expect(err).To(BeNil())
			})

			It("responds with 404", func() {
				doRequest(hs.H.Slug, anotherToken.Token, nil)

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("restarts the host when user is an admin", func() {
				_, err := db.Exec("UPDATE users SET admin = true WHERE id = $1", anotherUser.ID)
				Expect(err).To(BeNil())

				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug, anotherToken.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.State).To(Equal(host.StateRunning))
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			var origState string
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusBadRequest,
				})

				hosts.HostbuilderClient = hb

				origState = hs.H.State
				doRequest(hs.H.Slug, t.Token, nil)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("resets the host to its original state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("logs transitions back for the host", func() {
				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateRestarting, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, origState, host.StateRestarting).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("when host has running containers", func() {
			var c1, c2 *container.Container

			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					State:  container.StateRunning,
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					State:  container.StateRunning,
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
			})

			It("publishes container stopped events", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				Expect(c1.State).To(Equal(container.StateRunning))
				Expect(c2.State).To(Equal(container.StateRunning))

				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				// Collect the 2 expected container stop events.
				stopEvents := []*controllers.Event{}
				finished := make(chan struct{})
				go func() {
					for msg := range ch {
						if e := msg.(*controllers.Event); e.Name == controllers.EventContainerStopped {
							stopEvents = append(stopEvents, e)
						}
						if len(stopEvents) == 2 {
							close(finished)
							break
						}
					}
				}()

				var slugs []string
				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container stopped events")
				case <-finished:
					slugs = append(slugs, checkStoppedEvent(stopEvents[0]))
					slugs = append(slugs, checkStoppedEvent(stopEvents[1]))
				}
				// order of receiving events is non-deterministic, so check expected
				// slugs in sorted order
				sort.Strings(slugs)
				Expect(slugs[0]).To(Equal(c2.Slug)) // python-box
				Expect(slugs[1]).To(Equal(c1.Slug)) // rails-box
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/restart"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})

		It("should not be accessible by a collaborator", func() {
			_, t := createCollaborator(hs)

			doRequest(hs.H.Slug, t.Token, nil)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		Describe("when host has stopping containers", func() {
			var c1, c2 *container.Container
			BeforeEach(func() {
				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					State:  container.StateRunning,
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
				c1.TempState = sql.NullInt64{int64(container.StateStopping), true}
				err = c1.Update(db)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_container_state"))
				Expect(errDesc).To(Equal("containers(rails-box) are not stopped"))
			})

			Context("force is true", func() {
				var params *url.Values
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					replaceTransport(hb, &test.FakeRoundTripper{
						Message: stopOkResponse,
						Status:  http.StatusOK,
					})
					hosts.HostbuilderClient = hb

					hs.H.State = host.StateStopping
					err := hs.H.Update(db)
					Expect(err).To(BeNil())

					params = &url.Values{}
					params.Set("force", "true")
				})

				It("stops host", func() {
					doRequest(hs.H.Slug, t.Token, params)

					Expect(res.Code).To(Equal(http.StatusOK))
					h, err := host.Find(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateRunning))
				})

				It("does not publish container stopped events", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					doRequest(hs.H.Slug, t.Token, params)
					Expect(res.Code).To(Equal(http.StatusOK))

					stopEvents := []*controllers.Event{}
					finished := make(chan struct{})
					go func() {
						for msg := range ch {
							if e := msg.(*controllers.Event); e.Name == controllers.EventContainerStopped {
								stopEvents = append(stopEvents, e)
							}
						}
					}()

					go func() {
						time.After(50 * time.Millisecond)
						close(finished)
					}()

					var slugs []string
					select {
					case <-finished:
						slugs = append(slugs, checkStoppedEvent(stopEvents[0]))
					}
					Expect(stopEvents).To(HaveLen(1))
					Expect(slugs[0]).To(Equal(c2.Slug)) // python-box
				})
			})
		})

		Describe("when host has starting containers", func() {
			var c1, c2 *container.Container
			BeforeEach(func() {
				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					State:  container.StateRunning,
					UserID: hs.U.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
				c1.TempState = sql.NullInt64{int64(container.StateStarting), true}
				err = c1.Update(db)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest(hs.H.Slug, t.Token, nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_container_state"))
				Expect(errDesc).To(Equal("containers(rails-box) are not stopped"))
			})

			Context("force is true", func() {
				var params *url.Values
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					replaceTransport(hb, &test.FakeRoundTripper{
						Message: stopOkResponse,
						Status:  http.StatusOK,
					})
					hosts.HostbuilderClient = hb

					hs.H.State = host.StateStopping
					err := hs.H.Update(db)
					Expect(err).To(BeNil())

					params = &url.Values{}
					params.Set("force", "true")
				})

				It("stops host", func() {
					doRequest(hs.H.Slug, t.Token, params)

					Expect(res.Code).To(Equal(http.StatusOK))
					h, err := host.Find(db, hs.U, hs.H.Slug)
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal(host.StateRunning))
				})

				It("publishes container stopped events", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					doRequest(hs.H.Slug, t.Token, params)
					Expect(res.Code).To(Equal(http.StatusOK))

					stopEvents := []*controllers.Event{}
					finished := make(chan struct{})
					go func() {
						for msg := range ch {
							if e := msg.(*controllers.Event); e.Name == controllers.EventContainerStopped {
								stopEvents = append(stopEvents, e)
							}
						}
					}()

					go func() {
						time.After(50 * time.Millisecond)
						close(finished)
					}()

					var slugs []string
					select {
					case <-finished:
						Expect(stopEvents).To(HaveLen(2))
						slugs = append(slugs, checkStoppedEvent(stopEvents[0]))
						slugs = append(slugs, checkStoppedEvent(stopEvents[1]))
					}
					sort.Strings(slugs)
					Expect(slugs[0]).To(Equal(c2.Slug))
					Expect(slugs[1]).To(Equal(c1.Slug))
				})
			})
		})
	})

	Describe("POST /api/v0/hosts/{slug}/respec", func() {
		var (
			hs *test.HostStack
			rt *test.FakeRoundTripper
		)

		BeforeEach(func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt = &test.FakeRoundTripper{
				Message: okResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb

			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				PT: &providertemplate.ProviderTemplate{
					Name:     "default-1",
					Provider: "amazon",
					Config: hstore.Hstore{
						Map: map[string]sql.NullString{
							"instance_type": sql.NullString{"t2.micro", true},
							"storage_size":  sql.NullString{"20", true},
							"disable_eip":   sql.NullString{"false", true},
						},
					},
				},
			})
			Expect(err).To(BeNil())

			r := &respec.Respec{
				HostID:             hs.H.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			tx, err := db.Beginx()
			Expect(err).To(BeNil())
			Expect(r.Insert(tx)).To(BeNil())
			Expect(tx.Commit()).To(BeNil())
		})

		doRequest := func(slug string) {
			req, err := test.NewFormRequest("POST", "/api/v0/hosts/"+slug+"/respec", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.Token))
			handler.ServeHTTP(res, req)
		}

		It("responds with HTTP 400 Bad Request and does nothing if the host does not have a pending respec", func() {
			// Create a new host that has no pending respec.
			hs2, err := test.CreateHostStack(db, &test.HostStack{U: u})
			Expect(err).To(BeNil())

			doRequest(hs2.H.Slug)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})

		It("responds with HTTP 202 Accepted and respecs the host asynchronously", func() {
			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
			Expect(j.Host.State).To(Equal(host.StateRespeccing))
		})

		It("sends the new provider template config to Hostbuilder", func() {
			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			// Wait for Hostbuilder request to be received, since it's done
			// asynchronously from the HTTP request.
			Eventually(func() int {
				return len(rt.Requests)
			}).Should(Equal(1))

			req := rt.Requests[0]
			var config hostbuilderclient.Config
			err := json.Unmarshal([]byte(req.PostFormValue("config")), &config)
			Expect(err).To(BeNil())

			Expect(config["id"]).To(Equal(hs.H.ProviderIdentifier.String))
			Expect(config["instance_type"]).To(Equal("t2.micro"))
		})

		It("updates the host state to respeccing", func() {
			// Add some delay in hostbuilder response so that we can test the
			// transition state (i.e. "respeccing").
			rt.Wait = 100 * time.Millisecond

			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			h, err := host.FindByIdentifier(db, hs.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.State).To(Equal(host.StateRespeccing))
		})

		It("eventually updates the host state to running", func() {
			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			Eventually(func() (string, error) {
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				if err != nil {
					return "", err
				}
				return h.State, nil
			}).Should(Equal(host.StateRunning))
		})

		It("updates the host's provider template to its new template (i.e. the one that we respecced to)", func() {
			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			Eventually(func() (int64, error) {
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				if err != nil {
					return 0, err
				}
				return h.ProviderTemplateID, nil
			}).Should(Equal(hs.PT.ID))
		})

		It("updates the host's IP address from hostbuilder's response", func() {
			// Add some delay in hostbuilder response so that we can test the
			// IP address before hostbuilder is done.
			rt.Wait = 100 * time.Millisecond

			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			h, err := host.FindByIdentifier(db, hs.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.IPAddress.String).To(Equal(""))

			Eventually(func() (string, error) {
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				if err != nil {
					return "", err
				}
				return h.IPAddress.String, nil
			}).Should(Equal("127.0.0.1"))
		})

		It("changes the respec's state to processing", func() {
			// Add some delay in hostbuilder response so that we can test the
			// transition state.
			rt.Wait = 100 * time.Millisecond

			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			r := &respec.Respec{}
			err := db.Get(r, `SELECT * FROM respecs WHERE host_id = $1 LIMIT 1`, hs.H.ID)
			Expect(err).To(BeNil())

			Expect(r.State).To(Equal(respec.StateProcessing))
		})

		It("eventually changes the respec's state to completed", func() {
			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			Eventually(func() (string, error) {
				r := &respec.Respec{}
				err := db.Get(r, `SELECT * FROM respecs WHERE host_id = $1 LIMIT 1`, hs.H.ID)
				if err != nil {
					return "", err
				}
				return r.State, nil
			}).Should(Equal(respec.StateCompleted))
		})

		It("publishes a host respeccing event", func() {
			// Add some delay in hostbuilder response so that host does not get
			// return to "running" state too quickly.
			rt.Wait = 100 * time.Millisecond

			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			h, err := queryHost(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(h.State).To(Equal(host.StateRespeccing))

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timed out waiting for host respeccing event")
			case msg := <-ch:
				Expect(msg).To(Equal(&controllers.Event{
					Name: controllers.EventHostRespeccing,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(h),
					},
				}))
			}
		})

		It("eventually publishes a host respecced event", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest(hs.H.Slug)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			// 1st message is a respeccing event
			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventHostRespeccing))

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timed out waiting for host respecced event")
			case msg := <-ch:
				h, err := queryHost(db, hs.H.ID)
				Expect(err).To(BeNil())

				Expect(msg).To(Equal(&controllers.Event{
					Name: controllers.EventHostRespecced,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(h),
					},
				}))
			}
		})

		Context("when hostbuilder fails to create the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt = &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusInternalServerError,
				}
				replaceTransport(hb, rt)
				hosts.HostbuilderClient = hb
			})

			It("eventually updates the host state to 'respec failed'", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() (string, error) {
					h, err := host.FindByIdentifier(db, hs.H.Slug)
					if err != nil {
						return "", err
					}
					return h.State, nil
				}).Should(Equal(host.StateRespecFailed))
			})

			It("eventually changes the respec's state to failed", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() (string, error) {
					r := &respec.Respec{}
					err := db.Get(r, `SELECT * FROM respecs WHERE host_id = $1 LIMIT 1`, hs.H.ID)
					if err != nil {
						return "", err
					}
					return r.State, nil
				}).Should(Equal(respec.StateFailed))
			})

			It("publishes a host respec failed event with the host record and error", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				// 1st message is a respeccing event, ignore it.
				_ = <-ch

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for host respec failed event")
				case msg := <-ch:
					h, err := queryHost(db, hs.H.ID)
					Expect(err).To(BeNil())

					evt, ok := msg.(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(evt.Name).To(Equal(controllers.EventHostRespecFailed))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(evtData["host"]).To(Equal(serializers.SerializeHost(h)))
					Expect(evtData["error"]).To(ContainSubstring("some error occurred"))
				}
			})
		})
	})

	Describe("POST /api/v0/hosts/{slug}/update_software", func() {
		var (
			hs         *test.HostStack
			mockClient *freighterclient.MockClient
			tok        *token.Token
		)

		doRequest := func() {
			req, err := test.NewRequest("POST", "/api/v0/hosts/"+hs.H.Slug+"/update_software", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+tok.Token)
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error

			mockClient = &freighterclient.MockClient{}
			test.StubDefaultFreighterFn(mockClient)

			hs, err = test.CreateHostStack(db, &test.HostStack{U: u})
			Expect(err).To(BeNil())
			tok = t
		})

		AfterEach(func() {
			test.UnstubFreighterFn()
		})

		It("updates last_pinged_at", func() {
			doRequest()
			h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
			Expect(err).To(BeNil())
			Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
		})

		It("responds with HTTP 202 Accepted and publishes a host software updating event", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest()
			Expect(mockClient.UpgradeServerCalled).To(Equal(1))

			Expect(res.Code).To(Equal(http.StatusAccepted))

			h, err := queryHost(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(h.State).To(Equal(host.StateUpdating))

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timed out waiting for host software updating event")
			case msg := <-ch:
				Expect(msg).To(Equal(&controllers.Event{
					Name: controllers.EventHostSoftwareUpdating,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(h),
					},
				}))
			}
		})

		It("responds with HTTP 200 OK, the host, and publishes a host software updated event when host has already been updated", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			mockClient.UpgradeServerError = freighterclient.ErrAlreadyUpgraded

			doRequest()
			Expect(mockClient.UpgradeServerCalled).To(Equal(1))

			Expect(res.Code).To(Equal(http.StatusOK))
			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))

			h, err := queryHost(db, hs.H.ID)
			Expect(err).To(BeNil())

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timed out waiting for host software updated event")
			case msg := <-ch:
				Expect(msg).To(Equal(&controllers.Event{
					Name: controllers.EventHostSoftwareUpdated,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(h),
					},
				}))
			}
		})

		It("responds with HTTP 500 when software update fails", func() {
			mockClient.UpgradeServerError = errors.New("host cannot be updated")

			doRequest()
			Expect(res.Code).To(Equal(http.StatusInternalServerError))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
			Expect(errDesc).To(Equal("an error occurred while attempting to update your workspace"))

			Expect(mockClient.UpgradeServerCalled).To(Equal(1))
		})

		It("should not be accessible by a collaborator", func() {
			_, t := createCollaborator(hs)
			tok = t

			doRequest()
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("PUT /api/v0/hosts/:slug/versions", func() {
		var (
			hs     *test.HostStack
			params *url.Values
			err    error
			tok    *token.Token
		)

		doRequest := func() {
			req, err := test.NewAuthedRequest(t, "PUT", fmt.Sprintf("/api/v0/hosts/%s/versions", hs.H.Slug), params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+tok.Token)
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			tok = t
		})

		Context("when the user is not an admin", func() {
			It("responds with 404", func() {
				hs, err = test.CreateHostStack(db, &test.HostStack{U: u})
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("freighterd_version", "1.0.88-acecafe-42")
				params.Set("docker_version", "1.2.3-nitrous")

				doRequest()
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when valid params are received and user is an admin", func() {
			BeforeEach(func() {
				_, err = db.Exec("UPDATE users SET admin = true WHERE id = $1", u.ID)
				Expect(err).To(BeNil())

				hs, err = test.CreateHostStack(db, &test.HostStack{U: u})
				Expect(err).To(BeNil())

				params = &url.Values{}
				params.Set("freighterd_version", "1.0.88-acecafe-42")
				params.Set("docker_version", "1.2.3-nitrous")

				doRequest()
			})

			It("updates a host's versions record in the db", func() {
				h, err := queryHost(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(h.FreighterdVersion).To(Equal("1.0.88-acecafe-42"))
				Expect(h.DockerVersion).To(Equal("1.2.3-nitrous"))
			})

			It("responds with created and serialized host versions", func() {
				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Host.FreighterdVersion).To(Equal("1.0.88-acecafe-42"))
				Expect(j.Host.DockerVersion).To(Equal("1.2.3-nitrous"))
			})
		})

		It("should not be accessible by a collaborator", func() {
			hs, err = test.CreateHostStack(db, &test.HostStack{U: u})
			Expect(err).To(BeNil())
			_, t := createCollaborator(hs)
			tok = t

			doRequest()
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("GET /api/v0/hosts/{slug}/can_create_containers", func() {
		var hs *test.HostStack

		doRequest := func() {
			req, err := test.NewRequest("GET", "/api/v0/hosts/"+hs.H.Slug+"/can_create_containers", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					State:            host.StateRunning,
					IPAddress:        sql.NullString{"127.0.0.1", true},
					FreighterdPort:   sql.NullInt64{65333, true},
					RegionIdentifier: sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		It("should return true if the user can create containers", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				CanCreate bool `json:"can_create"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.CanCreate).To(BeTrue())
		})

		It("should return false if the user cannot create containers", func() {
			_, err := db.Exec("UPDATE hosts SET max_containers = 0 WHERE id = $1;", hs.H.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE users SET max_containers = 0 WHERE id = $1;", u.ID)
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				CanCreate bool `json:"can_create"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.CanCreate).To(BeFalse())
		})

	})

	Describe("GET /api/v0/hosts/:slug/apt_allowed", func() {
		doRequest := func(hostID int64, secret, hostSlug string) {
			req, err := test.NewRequest("GET", fmt.Sprintf("/api/v0/hosts/%s/apt_allowed", hostSlug), nil)
			Expect(err).To(BeNil())
			req.SetBasicAuth(strconv.FormatInt(hostID, 10), secret)
			handler.ServeHTTP(res, req)
		}

		It("returns 200 when basic auth user/password are set to a host's ID/secret", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug: "foo",
				},
			})
			Expect(err).To(BeNil())
			Expect(hs.H.Secret).ToNot(BeEmpty())

			doRequest(hs.H.ID, hs.H.Secret, strconv.FormatInt(hs.H.ID, 10))
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("returns 401 when basic auth password is not the host's secret", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug: "foo",
				},
			})
			Expect(err).To(BeNil())

			doRequest(hs.H.ID, "invalid-secret", strconv.FormatInt(hs.H.ID, 10))
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})

		It("returns 401 when basic auth user/password are a valid host ID/secret but for a different host", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug: "foo",
				},
			})
			Expect(err).To(BeNil())
			Expect(hs.H.Secret).ToNot(BeEmpty())

			doRequest(hs.H.ID, hs.H.Secret, "1234") // "1234" is ID of a different host.
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})

		It("returns 401 when host is terminated", func() {
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug: "foo",
				},
			})
			Expect(err).To(BeNil())
			Expect(hs.H.Secret).ToNot(BeEmpty())

			err = hs.H.Delete(db)
			Expect(err).To(BeNil())

			doRequest(hs.H.ID, hs.H.Secret, strconv.FormatInt(hs.H.ID, 10))
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})

		It("returns 401 when host does not exist", func() {
			doRequest(1234, "some-secret", "1234")
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
		})
	})
})

var (
	stopOkResponse = `{"state": "stopped", "ip_address": ""}`
	okResponse     = `{"freighterd_port":65333,"ip_address":"127.0.0.1"}`
	errorResponse  = `{"error":"some error occurred"}`
)

// replace the transport in hostbuilder client with a new fake transport
func replaceTransport(hb *hostbuilderclient.Hostbuilder, t http.RoundTripper) {
	hb.Client.Client.Transport = t
}
