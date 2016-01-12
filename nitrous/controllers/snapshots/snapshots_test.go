package snapshots_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
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
	RunSpecs(t, "controllers/snapshots")
}

var _ = Describe("controllers/snapshots", func() {
	var (
		db  *ndb.Database
		svr *server.Server

		u      *user.User
		o1, o2 *org.Org
		hs     *test.HostStack
		i1     *image.Image

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = test.NewTestServer("", db, nil)

		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		t := &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		Expect(t.Insert(db)).To(BeNil())

		o1 = &org.Org{
			Name:  "ze robot labs",
			Email: "admin@zerobotlabs.com",
			Slug:  "zerobotlabs",
		}
		Expect(o1.Insert(db)).To(BeNil())

		o2 = &org.Org{
			Name:  "Path",
			Email: "admin@path.com",
			Slug:  "path",
		}
		Expect(o2.Insert(db)).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
			U: u,
			O: o1,
		})
		Expect(err).To(BeNil())

		i1 = &image.Image{
			UserID:        u.ID,
			HostID:        hs.H.ID,
			FreighterID:   "cafebabe",
			Name:          "nestorbot",
			Slug:          "nestorbot",
			Tag:           "latest",
			OriginalImage: sql.NullString{"quay.io/nitrousio/golang:latest", true},
		}
		Expect(i1.Insert(db)).To(BeNil())

		mockClient = &freighterclient.MockClient{}
		test.StubDefaultFreighterFn(mockClient)

		mockClient.InspectImageReturn = &freighterclient.Image{
			ID:          "deadbeef",
			Name:        "zerobotlabs/nestorbot",
			Tag:         "latest",
			VirtualSize: 8888,
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		test.UnstubFreighterFn()
	})

	Describe("GET /api/v0/orgs/{org_slug}/snapshots", func() {
		doRequest := func(orgSlug string) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/orgs/%s/snapshots", orgSlug)
			req, err := test.NewRequest("GET", url, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with HTTP 404 Not Found when org does not exist", func() {
			res := doRequest("non-existent-org")
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
			Expect(errDesc).To(Equal("org does not exist"))
		})

		It("responds with HTTP 403 Forbidden when user does not belong in the org", func() {
			res := doRequest(o1.Slug)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not a member of this org"))
		})

		Context("when user is a member of the org", func() {
			BeforeEach(func() {
				// Add user to both orgs.
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())

				// Add snapshots to one of the orgs.
				s := &snapshot.Snapshot{
					ImageID:       i1.ID,
					UserID:        u.ID,
					OrgID:         sql.NullInt64{o1.ID, true},
					Slug:          "zerobotlabs/nestorbot",
					Namespace:     "zerobotlabs",
					Name:          "nestorbot",
					Tag:           "latest",
					DisplayName:   "Nestorbot dev",
					OriginalImage: "golang:latest",
				}
				Expect(s.Insert(db)).To(BeNil())
			})

			It("responds with no snapshots if org does not have any snapshots", func() {
				res := doRequest(o2.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Snapshots []*serializers.Snapshot `json:"snapshots"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Snapshots).To(HaveLen(0))
			})

			It("responds with snapshots belonging to the org", func() {
				res := doRequest(o1.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Snapshots []*serializers.Snapshot `json:"snapshots"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Snapshots).To(HaveLen(1))

				s := j.Snapshots[0]
				Expect(s.UserID).To(Equal(u.ID))
				Expect(o1.ID).To(Equal(o1.ID))
				Expect(s.DisplayName).To(Equal("Nestorbot dev"))
			})
		})
	})

	Describe("POST /api/v0/orgs/{org_slug}/snapshots/publish", func() {
		doRequest := func(orgSlug string, params *url.Values) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/orgs/%s/snapshots/publish", orgSlug)
			req, err := test.NewFormRequest("POST", url, params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with HTTP 404 Not Found when org does not exist", func() {
			params := &url.Values{}
			params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
			params.Set("image_id", strconv.FormatInt(i1.ID, 10))

			res := doRequest("non-existent-org", params)
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
			Expect(errDesc).To(Equal("org does not exist"))
		})

		It("responds with HTTP 403 Forbidden when user does not belong in the org", func() {
			params := &url.Values{}
			params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
			params.Set("image_id", strconv.FormatInt(i1.ID, 10))

			res := doRequest(o1.Slug, params)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not a member of this org"))
		})

		It("responds with HTTP 403 Forbidden when user is a limited member of the org", func() {
			_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleLimited)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
			params.Set("image_id", strconv.FormatInt(i1.ID, 10))

			res := doRequest(o1.Slug, params)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not allowed to publish snapshots"))
		})

		Context("when user is member of the org", func() {
			BeforeEach(func() {
				// Add user to both orgs.
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 404 Not Found when host_id is that of a non-existent host", func() {
				params := &url.Values{}
				params.Set("host_id", "9999")
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("workstation not found"))
			})

			It("responds with HTTP 404 Not Found when host_id is that of a host that does not belong to the user", func() {
				hs2, err := test.CreateHostStack(db, &test.HostStack{})
				Expect(err).To(BeNil())
				Expect(hs2.U.ID).NotTo(Equal(u.ID))

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs2.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("workstation not found"))
			})

			It("responds with HTTP 400 Bad Request when host_id is that of a host that does not belong to the org", func() {
				hs2, err := test.CreateHostStack(db, &test.HostStack{U: u, O: o2})
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs2.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("bad_request"))
				Expect(errDesc).To(Equal("workstation does not belong to this org"))
			})

			It("responds with HTTP 404 Not Found when image_id is that of a non-existent image", func() {
				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", "9999")

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("snapshot not found"))
			})

			It("responds with HTTP 404 Not Found when image_id is that of an image that does not belong to the user", func() {
				hs2, err := test.CreateHostStack(db, &test.HostStack{})
				Expect(err).To(BeNil())
				Expect(hs2.U.ID).NotTo(Equal(u.ID))
				i2 := &image.Image{
					UserID:      hs2.U.ID,
					HostID:      hs2.H.ID,
					FreighterID: "cafedead",
					Name:        "another-image",
					Slug:        "another-image",
					Tag:         "latest",
				}
				Expect(i2.Insert(db)).To(BeNil())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i2.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("snapshot not found"))
			})

			It("responds with HTTP 409 Conflict when image has already been published", func() {
				// Create existing snapshot.
				s := &snapshot.Snapshot{
					ImageID:       i1.ID,
					UserID:        u.ID,
					OrgID:         sql.NullInt64{o1.ID, true},
					Slug:          "zerobotlabs/nestorbot",
					Namespace:     "zerobotlabs",
					Name:          "nestorbot",
					Tag:           "latest",
					DisplayName:   "Nestorbot dev",
					OriginalImage: "golang:latest",
				}
				Expect(s.Insert(db)).To(BeNil())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusConflict))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("conflict"))
				Expect(errDesc).To(Equal("snapshot already published"))
			})

			It("should not allow image to be published concurrently", func() {
				tx, _ := db.Beginx()
				ndb.TransactionLock(tx, ndb.IMAGE_PUBLISH_LOCK, int(i1.ID))
				defer tx.Rollback()

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusConflict))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("conflict"))
				Expect(errDesc).To(Equal("snapshot is currently being published"))
			})
		})

		Context("when params are valid", func() {
			BeforeEach(func() {
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 503 Service Unavailable when there is an error logging in to the Docker registry", func() {
				mockClient.RegistryLoginError = freighterclient.ErrInvalidParams

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("unavailable"))
				Expect(errDesc).To(Equal("the snapshot service is temporarily unavailable"))

				Expect(mockClient.RegistryLoginCalled).To(Equal(1))
			})

			It("creates a docker_registry token and uses it to login to the Docker registry", func() {
				_, err := token.FindByType(db, u.ID, token.TypeDockerRegistry)
				Expect(err).To(Equal(sql.ErrNoRows))

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				tok, err := token.FindByType(db, u.ID, token.TypeDockerRegistry)
				Expect(err).To(BeNil())
				Expect(tok.UserID).To(Equal(u.ID))
				Expect(tok.Type).To(Equal(token.TypeDockerRegistry))
				Expect(tok.Token).NotTo(BeEmpty())

				Expect(mockClient.RegistryLoginCalled).To(Equal(1))
				Expect(mockClient.ReceivedUsername).To(Equal(u.Username))
				Expect(mockClient.ReceivedPassword).To(Equal(tok.Token))
				Expect(mockClient.ReceivedEmail).To(Equal(u.Email))
				Expect(mockClient.ReceivedRegistry).To(Equal(envvars.DockerRegistryHostname))
			})

			It("uses an existing docker_registry token to login to the Docker registry", func() {
				// Insert existing token.
				tok := &token.Token{
					UserID: u.ID,
					Type:   token.TypeDockerRegistry,
				}
				Expect(tok.Insert(db)).To(BeNil())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Expect(mockClient.RegistryLoginCalled).To(Equal(1))
				Expect(mockClient.ReceivedUsername).To(Equal(u.Username))
				Expect(mockClient.ReceivedPassword).To(Equal(tok.Token))
				Expect(mockClient.ReceivedEmail).To(Equal(u.Email))
				Expect(mockClient.ReceivedRegistry).To(Equal(envvars.DockerRegistryHostname))
			})

			It("pushes the image to the Docker registry", func() {
				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() int {
					return mockClient.PushImageCalled
				}).Should(Equal(1))

				Expect(mockClient.ReceivedImage).To(Equal(i1.FreighterID))

				expectedTargetRepo := fmt.Sprintf("%s/%s/nestorbot-%d:latest",
					envvars.DockerRegistryHostname, o1.Slug, i1.ID)
				Expect(mockClient.ReceivedTag).To(Equal(expectedTargetRepo))
			})

			It("converts an image name that contains spaces to a legit slug", func() {
				// This is to handle images which have slugs containing spaces,
				// e.g. "Laravel 12/7/2015 4:18:41 PM".
				i1.Slug = "Laravel 12/7/2015 4:18:41 PM"
				Expect(i1.Update(db)).To(BeNil())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() int {
					return mockClient.PushImageCalled
				}).Should(Equal(1))

				Expect(mockClient.ReceivedImage).To(Equal(i1.FreighterID))

				expectedTargetRepo := fmt.Sprintf("%s/%s/laravel-12-7-2015-4-18-41-pm-%d:latest",
					envvars.DockerRegistryHostname, o1.Slug, i1.ID)
				Expect(mockClient.ReceivedTag).To(Equal(expectedTargetRepo))
			})

			It("publishes a snapshot publishing event", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evt := (<-ch).(*controllers.Event)
				Expect(evt.Name).To(Equal(controllers.EventSnapshotPublishing))
				evtData, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())

				orgSlug, ok := evtData["org_slug"].(string)
				Expect(ok).To(BeTrue())
				Expect(orgSlug).To(Equal(o1.Slug))

				hostID, ok := evtData["host_id"].(int64)
				Expect(ok).To(BeTrue())
				Expect(hostID).To(Equal(hs.H.ID))

				imageID, ok := evtData["image_id"].(int64)
				Expect(ok).To(BeTrue())
				Expect(imageID).To(Equal(i1.ID))
			})

			It("publishes a snapshot publish failed event when there is an error pushing the image", func() {
				mockClient.PushImageError = errors.New("test error")

				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventSnapshotPublishFailed {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for snapshot publish failed event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventSnapshotPublishFailed))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					errMsg, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(errMsg).To(Equal("test error"))

					orgSlug, ok := evtData["org_slug"].(string)
					Expect(ok).To(BeTrue())
					Expect(orgSlug).To(Equal(o1.Slug))

					hostID, ok := evtData["host_id"].(int64)
					Expect(ok).To(BeTrue())
					Expect(hostID).To(Equal(hs.H.ID))

					imageID, ok := evtData["image_id"].(int64)
					Expect(ok).To(BeTrue())
					Expect(imageID).To(Equal(i1.ID))
				}
			})

			It("creates a snapshot record and publishes a snapshot published event to all members of the org", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				// Create another user as a member of the same org.
				u2 := &user.User{
					Username: "otheruser",
					Password: "123456",
					Email:    "otheruser@example.com",
				}
				Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				otherMemberCh := svr.Events.Subscribe(u2.Topic())
				defer svr.Events.Unsubscribe(otherMemberCh, u2.Topic())

				params := &url.Values{}
				params.Set("host_id", strconv.FormatInt(hs.H.ID, 10))
				params.Set("image_id", strconv.FormatInt(i1.ID, 10))

				res := doRequest(o1.Slug, params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventSnapshotPublished {
							evtCh <- evt
							break
						}
					}
				}()

				var snap *snapshot.SnapshotWithOwner

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for snapshot published event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventSnapshotPublished))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					snapshots, err := snapshot.FindAllByOrgID(db, u.ID, o1.ID)
					Expect(err).To(BeNil())

					for _, s := range snapshots {
						if s.ImageID == i1.ID {
							snap = s
						}
					}
					Expect(snap).NotTo(BeNil())
					Expect(snap.UserID).To(Equal(u.ID))
					Expect(snap.OrgID.Int64).To(Equal(o1.ID))
					Expect(snap.Slug).To(Equal(fmt.Sprintf("%s/%s-%d", o1.Slug, i1.Slug, i1.ID)))
					Expect(snap.Namespace).To(Equal(o1.Slug))
					Expect(snap.Name).To(Equal(fmt.Sprintf("%s-%d", i1.Slug, i1.ID)))
					Expect(snap.Tag).To(Equal("latest"))
					Expect(snap.DisplayName).To(Equal(i1.Name))
					Expect(snap.OriginalImage).To(Equal(i1.OriginalImage.String))
					Expect(snap.Size.Int64).To(Equal(mockClient.InspectImageReturn.VirtualSize))

					Expect(snap.OwnerUsername.String).To(Equal(u.Username))
					Expect(snap.OwnerName.String).To(Equal(u.Name))
					Expect(snap.OwnerEmail.String).To(Equal(u.Email))

					serialized, ok := evtData["snapshot"].(*serializers.Snapshot)
					Expect(ok).To(BeTrue())
					Expect(serialized).To(Equal(serializers.SerializeSnapshotWithOwner(snap)))

					orgSlug, ok := evtData["org_slug"].(string)
					Expect(ok).To(BeTrue())
					Expect(orgSlug).To(Equal(o1.Slug))

					hostID, ok := evtData["host_id"].(int64)
					Expect(ok).To(BeTrue())
					Expect(hostID).To(Equal(hs.H.ID))

					imageID, ok := evtData["image_id"].(int64)
					Expect(ok).To(BeTrue())
					Expect(imageID).To(Equal(i1.ID))
				}

				evt := (<-otherMemberCh).(*controllers.Event)
				Expect(evt.Name).To(Equal(controllers.EventSnapshotPublished))
				evtData, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())

				serialized, ok := evtData["snapshot"].(*serializers.Snapshot)
				Expect(ok).To(BeTrue())
				Expect(serialized).To(Equal(serializers.SerializeSnapshotWithOwner(snap)))
			})
		})
	})

	Describe("DELETE /orgs/{org_slug}/snapshots/{snapshot_id}", func() {
		var (
			snap1, snap2 *snapshot.Snapshot
			u2           *user.User
		)

		BeforeEach(func() {
			snap1 = &snapshot.Snapshot{
				ImageID:       i1.ID,
				UserID:        u.ID,
				OrgID:         sql.NullInt64{o1.ID, true},
				Slug:          "zerobotlabs/nestorbot",
				Namespace:     "zerobotlabs",
				Name:          "nestorbot",
				Tag:           "latest",
				DisplayName:   "Nestorbot dev",
				OriginalImage: "golang:latest",
			}
			Expect(snap1.Insert(db)).To(BeNil())

			// Create snapshot belonging to another user.
			u2 = &user.User{
				Username: "otheruser",
				Password: "123456",
				Email:    "otheruser@example.com",
			}
			Expect(test.WrapInTransaction(db, u2.Insert)).To(BeNil())

			hs2, err := test.CreateHostStack(db, &test.HostStack{U: u2})
			Expect(err).To(BeNil())
			i2 := &image.Image{
				UserID:      u2.ID,
				HostID:      hs2.H.ID,
				FreighterID: "cafedead",
				Name:        "another-image",
				Slug:        "another-image",
				Tag:         "latest",
			}
			Expect(i2.Insert(db)).To(BeNil())

			snap2 = &snapshot.Snapshot{
				ImageID:       i2.ID,
				UserID:        u2.ID,
				OrgID:         sql.NullInt64{o1.ID, true},
				Slug:          "zerobotlabs/relax-rb",
				Namespace:     "zerobotlabs",
				Name:          "relax-rb",
				Tag:           "latest",
				DisplayName:   "Relax ruby client",
				OriginalImage: "ruby:latest",
			}
			Expect(snap2.Insert(db)).To(BeNil())
		})

		doRequest := func(orgSlug string, snapshotID int64) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/orgs/%s/snapshots/%d", orgSlug, snapshotID)
			req, err := test.NewRequest("DELETE", url, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with HTTP 404 Not Found when org does not exist", func() {
			res := doRequest("non-existent-org", 9000)
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
			Expect(errDesc).To(Equal("org does not exist"))
		})

		It("responds with HTTP 403 Forbidden when user does not belong in the org", func() {
			res := doRequest(o1.Slug, 9000)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not a member of this org"))
		})

		It("responds with HTTP 403 Forbidden when user is a limited member of the org", func() {
			_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleLimited)
			Expect(err).To(BeNil())

			res := doRequest(o1.Slug, snap2.ID)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not allowed to delete this snapshot"))
		})

		It("responds with HTTP 403 Forbidden when user is a regular member of the org", func() {
			_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
			Expect(err).To(BeNil())

			res := doRequest(o1.Slug, snap2.ID)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you are not allowed to delete this snapshot"))
		})

		Context("when user is owner of the snapshot", func() {
			It("allows user to delete it even if he is a limited member of the org", func() {
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleLimited)
				Expect(err).To(BeNil())

				res := doRequest(o1.Slug, snap1.ID)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = snapshot.FindByID(db, snap1.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
			})

			It("allows user to delete it even if he is a regular member of the org", func() {
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				res := doRequest(o1.Slug, snap1.ID)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = snapshot.FindByID(db, snap1.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
			})

			It("publishes a snapshot deleted event to all members of the org", func() {
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				// Add other user to the same org.
				_, err = orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				otherMemberCh := svr.Events.Subscribe(u2.Topic())
				defer svr.Events.Unsubscribe(otherMemberCh, u2.Topic())

				swo, err := snapshot.FindByID(db, snap1.ID)
				Expect(err).To(BeNil())

				res := doRequest(o1.Slug, snap1.ID)
				Expect(res.Code).To(Equal(http.StatusOK))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventSnapshotDeleted {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for snapshot deleted event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventSnapshotDeleted))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					serialized, ok := evtData["snapshot"].(*serializers.Snapshot)
					Expect(ok).To(BeTrue())
					Expect(serialized).To(Equal(serializers.SerializeSnapshotWithOwner(swo)))
				}

				evt := (<-otherMemberCh).(*controllers.Event)
				Expect(evt.Name).To(Equal(controllers.EventSnapshotDeleted))
				evtData, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())

				serialized, ok := evtData["snapshot"].(*serializers.Snapshot)
				Expect(ok).To(BeTrue())
				Expect(serialized).To(Equal(serializers.SerializeSnapshotWithOwner(swo)))
			})
		})

		Context("when user is an admin member of the org", func() {
			BeforeEach(func() {
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleAdmin)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 404 Not Found when snapshot does not exist", func() {
				res := doRequest(o1.Slug, 9000)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("snapshot not found"))
			})

			It("allows user to delete a snapshot they don't own", func() {
				res := doRequest(o1.Slug, snap2.ID)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err := snapshot.FindByID(db, snap2.ID)
				Expect(err).To(Equal(sql.ErrNoRows))
			})

			It("publishes a snapshot deleted event to all members of the org", func() {
				// Add other user to the same org.
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u2.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				otherMemberCh := svr.Events.Subscribe(u2.Topic())
				defer svr.Events.Unsubscribe(otherMemberCh, u2.Topic())

				swo, err := snapshot.FindByID(db, snap2.ID)
				Expect(err).To(BeNil())

				res := doRequest(o1.Slug, snap2.ID)
				Expect(res.Code).To(Equal(http.StatusOK))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventSnapshotDeleted {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for snapshot deleted event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventSnapshotDeleted))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					serialized, ok := evtData["snapshot"].(*serializers.Snapshot)
					Expect(ok).To(BeTrue())
					Expect(serialized).To(Equal(serializers.SerializeSnapshotWithOwner(swo)))
				}

				evt := (<-otherMemberCh).(*controllers.Event)
				Expect(evt.Name).To(Equal(controllers.EventSnapshotDeleted))
				evtData, ok := evt.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())

				serialized, ok := evtData["snapshot"].(*serializers.Snapshot)
				Expect(ok).To(BeTrue())
				Expect(serialized).To(Equal(serializers.SerializeSnapshotWithOwner(swo)))
			})
		})
	})
})
