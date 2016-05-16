package containers_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/quickstart"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
	"github.com/nitrous-io/nitrous-common/sshkey"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateAtomic", func() {
	var (
		db  *ndb.Database
		svr *server.Server

		u  *user.User
		t  *token.Token
		ft *test.FakeTracker
		hs *test.HostStack

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		Expect(t.Insert(db)).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:          "foo",
				IPAddress:     sql.NullString{"127.0.0.1", true},
				MaxContainers: sql.NullInt64{1, true},
			},
			U: u,
		})
		Expect(err).To(BeNil())

		mockClient = &freighterclient.MockClient{}
		test.StubDefaultFreighterFn(mockClient)

		mockClient.CreatedContainer = &freighterclient.Container{
			ID:      "deadbeef",
			Name:    "rails-project",
			Host:    "",
			Gateway: "",
			State:   "running",
			Image:   "rails",
			ImageID: "abc123",
			Ports: map[string][]string{
				"22":   []string{"65022"},
				"3000": []string{"63000"},
			},
			Env: []string{
				"NITROUS_PREVIEW_PORT=3000",
			},
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		test.UnstubFreighterFn()
	})

	expectSerializedContainer := func(cn *serializers.Container, c *container.Container) {
		Expect(cn.ID).To(Equal(c.ID))
		Expect(cn.CID).To(Equal("deadbeef"))
		Expect(cn.UserID).To(Equal(u.ID))
		Expect(cn.HostID).To(Equal(hs.H.ID))
		Expect(cn.Name).To(Equal("rails-project"))
		Expect(cn.Image).To(Equal("rails"))
		Expect(cn.Slug).To(HavePrefix("rails-project-"))
		Expect(slugbuilder.IsValid(cn.Slug)).To(BeTrue())
		Expect(cn.IPAddress).To(Equal(""))
		Expect(cn.GatewayIP).To(Equal(""))
		Expect(cn.State).To(Equal(container.StateRunning))
	}

	Describe("POST /api/v0/containers/create_atomic", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			url := "/api/v0/containers/create_atomic"
			req, err := test.NewFormRequest("POST", url, params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("responds with HTTP 404 Not Found if host with the given slug does not exist", func() {
			params := &url.Values{}
			params.Set("host", "non-existent-host")
			params.Set("name", "")
			params.Set("image", "rails")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("responds with HTTP 400 Bad Request if name is not provided", func() {
			params := &url.Values{}
			params.Set("host", hs.H.Slug)
			params.Set("name", "")
			params.Set("image", "rails")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("name is required"))
		})

		It("responds with HTTP 400 Bad Request if image, template and snapshot_id are not provided", func() {
			params := &url.Values{}
			params.Set("host", hs.H.Slug)
			params.Set("name", "rails-project")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal(`either "image", "template" or "snapshot_id" must be specified`))
		})

		It("should not allow containers to be created concurrently", func() {
			tx, _ := db.Beginx()
			ndb.TransactionLock(tx, ndb.CONTAINER_CREATE_LOCK, int(u.ID))
			defer tx.Rollback()

			params := &url.Values{}
			params.Set("host", hs.H.Slug)
			params.Set("name", "rails-project")
			params.Set("image", "rails")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusConflict))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("conflict"))
			Expect(errDesc).To(Equal("another container creation request is currently being processed"))
		})

		It("should not allow container to be created if user has more containers than their host allows", func() {
			Expect(hs.H.MaxContainers.Int64).To(Equal(int64(1)))

			c := &container.Container{
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
			}
			Expect(c.Insert(db)).To(BeNil())

			params := &url.Values{}
			params.Set("host", hs.H.Slug)
			params.Set("name", "rails-project")
			params.Set("image", "rails")

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you have reached the maximum number of projects allowed by your plan"))
		})

		Context("when image param is given", func() {
			It("pulls the image if skip_pull is false", func() {
				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() int {
					return mockClient.PullImageCalled
				}).Should(Equal(1))

				Expect(mockClient.ReceivedImage).To(Equal("rails"))
			})

			It("does not pull the image if skip_pull is true", func() {
				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")
				params.Set("skip_pull", "true")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() int {
					return mockClient.CreateCalled
				}).Should(Equal(1))

				Expect(mockClient.PullImageCalled).To(Equal(0))
			})

			It("asks freighter to create the container", func() {
				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				Eventually(func() int {
					return mockClient.CreateCalled
				}).Should(Equal(1))

				Expect(mockClient.ReceivedName).To(HavePrefix("rails-project-"))
				Expect(mockClient.ReceivedImage).To(Equal("rails"))
			})

			It("responds with HTTP 202 Accepted and serializes the container and the request_id", func() {
				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				var j struct {
					Container *serializers.Container `json:"container"`
					RequestID string                 `json:"request_id"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.ID).NotTo(BeZero())

				cn, err := container.Find(db, u, strconv.FormatInt(j.Container.ID, 10))
				Expect(err).To(BeNil())
				Expect(cn.HostID).To(Equal(hs.H.ID))
				Expect(cn.Name.String).To(Equal("rails-project"))
				Expect(cn.Image).To(Equal("rails"))

				Expect(j.RequestID).To(Equal(res.Header().Get(constants.NitrousUUIDHeader)))
			})

			It("publishes a container creating event with a unique request_id", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreating {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container creating event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreating))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					requestID, ok := evtData["request_id"].(string)
					Expect(ok).To(BeTrue())
					Expect(requestID).To(Equal(res.Header().Get(constants.NitrousUUIDHeader)))
				}
			})

			It("publishes a container created event with the created container", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					cn, ok := evtData["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))

					expectSerializedContainer(cn, cntrs[0])

					requestID, ok := evtData["request_id"].(string)
					Expect(ok).To(BeTrue())
					Expect(requestID).To(Equal(res.Header().Get(constants.NitrousUUIDHeader)))

					h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
					Expect(err).To(BeNil())
					Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
				}
			})

			It("creates container volume associations and mount binds when using existing volumes", func() {
				v1 := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
				Expect(v1.Insert(db)).To(BeNil())

				v2 := &volume.Volume{HostID: hs.H.ID, Slug: "backups"}
				Expect(v2.Insert(db)).To(BeNil())

				mockClient.CreatedContainer.Volumes = map[string]string{
					"/mpd/albums":      "music",
					"/volumes/backups": "backups",
				}

				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")
				params.Add("volume[]", "music:/mpd/albums")
				params.Add("volume[]", "backups")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					cn, ok := evtData["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))

					expectSerializedContainer(cn, cntrs[0])

					conf := mockClient.ReceivedConfig
					Expect(conf.Mounts).To(HaveLen(2))
					Expect(conf.Mounts).To(ContainElement("music:/mpd/albums"))
					Expect(conf.Mounts).To(ContainElement("backups"))

					cv1, err := cvolume.Find(db, cntrs[0], v1)
					Expect(err).To(BeNil())
					Expect(cv1.VolumeID).To(Equal(v1.ID))

					cv2, err := cvolume.Find(db, cntrs[0], v2)
					Expect(err).To(BeNil())
					Expect(cv2.VolumeID).To(Equal(v2.ID))

					h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
					Expect(err).To(BeNil())
					Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
				}
			})

			It("responds with HTTP 400 Bad Request when trying to use a volume that doesn't exist", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")
				params.Add("volume[]", "music:/mpd/albums")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("volume not found"))
			})

			It("creates new volumes and container volume associations when the Docker image specifies volumes", func() {

				mockClient.CreatedContainer.Volumes = map[string]string{
					"/var/lib/postgresql/data": "data",
				}

				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					cn, ok := evtData["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))

					expectSerializedContainer(cn, cntrs[0])

					conf := mockClient.ReceivedConfig
					Expect(conf.Mounts).To(HaveLen(0))

					vols, err := cvolume.FindAll(db, cntrs[0])
					Expect(err).To(BeNil())
					Expect(vols).To(HaveLen(1))

					vol := vols[0]
					Expect(vol.Slug).To(Equal("data"))
					Expect(vol.Name).To(Equal(fmt.Sprintf("%s:%s", cn.Slug, "/var/lib/postgresql/data")))

					cvol, err := cvolume.Find(db, cntrs[0], vol)
					Expect(err).To(BeNil())
					Expect(cvol.VolumeID).To(Equal(vol.ID))

					h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
					Expect(err).To(BeNil())
					Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
				}
			})
		})

		Context("when template param is given", func() {
			var tmpl *template.Template

			BeforeEach(func() {
				tmpl = &template.Template{
					Slug:        "golang-template",
					Name:        sql.NullString{"Golang", true},
					Description: sql.NullString{"A Golang stack template", true},
					Image:       "golang:latest",
					Config: `{
						"privileged": true,
						"publish_all": false,
						"ports": ["public::2341", "public::8931"],
						"env": ["SHELL=powershell"]
					}`,
					Type: template.TypeStack,
				}
				Expect(tmpl.Insert(db)).To(BeNil())
			})

			It("creates a container from the template", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "golang-project")
				params.Set("template", tmpl.Slug)

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					cn, ok := evtData["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))

					Expect(cn.ID).To(Equal(cntrs[0].ID))
					Expect(cn.UserID).To(Equal(u.ID))
					Expect(cn.HostID).To(Equal(hs.H.ID))
					Expect(cn.SnapshotID).To(Equal(int64(0)))
					Expect(cn.Name).To(Equal("golang-project"))
					Expect(cn.Image).To(Equal(tmpl.Image))

					Expect(mockClient.CreateCalled).To(Equal(1))
					Expect(mockClient.ReceivedImage).To(Equal(tmpl.Image))
					Expect(mockClient.ReceivedConfig.PortBindings).To(ConsistOf([]string{"public::2341", "public::8931"}))
					Expect(mockClient.ReceivedConfig.Env).To(ConsistOf([]string{"SHELL=powershell"}))
					Expect(*mockClient.ReceivedConfig.Privileged).To(BeTrue())
					Expect(*mockClient.ReceivedConfig.PublishAll).To(BeFalse())

					h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
					Expect(err).To(BeNil())
					Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
				}
			})

			It("allows request params to override the template's config", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "golang-project")
				params.Set("template", tmpl.Slug)
				params.Set("privileged", "false")
				params.Set("publish_all", "true")
				params.Add("port[]", "public::6060/tcp")
				params.Add("port[]", "public::8888/tcp")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					cn, ok := evtData["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))

					Expect(cn.ID).To(Equal(cntrs[0].ID))
					Expect(cn.UserID).To(Equal(u.ID))
					Expect(cn.HostID).To(Equal(hs.H.ID))
					Expect(cn.Name).To(Equal("golang-project"))
					Expect(cn.Image).To(Equal(tmpl.Image))

					Expect(mockClient.CreateCalled).To(Equal(1))
					Expect(mockClient.ReceivedConfig.PortBindings).To(ConsistOf([]string{"public::2341", "public::8931", "public::6060/tcp", "public::8888/tcp"}))
					Expect(*mockClient.ReceivedConfig.Privileged).To(BeFalse())
					Expect(*mockClient.ReceivedConfig.PublishAll).To(BeTrue())

					h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
					Expect(err).To(BeNil())
					Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
				}
			})
		})

		Context("when snapshot_id param is given", func() {
			var (
				snap1, snap2 *snapshot.Snapshot
				o1, o2       *org.Org
			)

			BeforeEach(func() {
				img1 := &image.Image{
					UserID:      u.ID,
					HostID:      hs.H.ID,
					FreighterID: "cafebabe",
					Name:        "nestorbot",
					Slug:        "nestorbot",
					Tag:         "latest",
				}
				Expect(img1.Insert(db)).To(BeNil())

				o1 = &org.Org{
					Name:  "ze robot labs",
					Email: "admin@zerobotlabs.com",
					Slug:  "zerobotlabs",
				}
				Expect(o1.Insert(db)).To(BeNil())

				snap1 = &snapshot.Snapshot{
					ImageID:       img1.ID,
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

				// Create another snapshot belonging to another org.
				o2 = &org.Org{
					Name:  "Path",
					Email: "admin@path.com",
					Slug:  "path",
				}
				Expect(o2.Insert(db)).To(BeNil())

				hs2, err := test.CreateHostStack(db, &test.HostStack{U: u, O: o2})
				Expect(err).To(BeNil())

				img2 := &image.Image{
					UserID:      u.ID,
					HostID:      hs2.H.ID,
					FreighterID: "deadbabe",
					Name:        "car-os",
					Slug:        "car-os",
					Tag:         "latest",
				}
				Expect(img2.Insert(db)).To(BeNil())

				snap2 = &snapshot.Snapshot{
					ImageID:       img2.ID,
					UserID:        u.ID,
					OrgID:         sql.NullInt64{o2.ID, true},
					Slug:          "path/car-os",
					Namespace:     "path",
					Name:          "car-os",
					Tag:           "latest",
					DisplayName:   "Car OS",
					OriginalImage: "rust:latest",
				}
				Expect(snap2.Insert(db)).To(BeNil())
			})

			It("responds with HTTP 400 Bad Request when snapshot does not exist", func() {
				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("snapshot_id", "9000")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("snapshot is invalid"))
			})

			It("responds with HTTP 403 Forbidden when snapshot does not belong to an org", func() {
				img3 := &image.Image{
					UserID:      u.ID,
					HostID:      hs.H.ID,
					FreighterID: "babecafe",
					Name:        "foo",
					Slug:        "foo",
					Tag:         "latest",
				}
				Expect(img3.Insert(db)).To(BeNil())
				snap3 := &snapshot.Snapshot{
					ImageID:       img3.ID,
					UserID:        u.ID,
					OrgID:         sql.NullInt64{},
					Slug:          "personal-project",
					Namespace:     "",
					Name:          "Personal Project",
					Tag:           "latest",
					DisplayName:   "Personal Project backup",
					OriginalImage: "rails:latest",
				}
				Expect(snap3.Insert(db)).To(BeNil())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("snapshot_id", strconv.Itoa(int(snap3.ID)))

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("forbidden"))
				Expect(errDesc).To(Equal("you are not allowed to use this snapshot"))
			})

			It("responds with HTTP 403 Forbidden when user not a member of the snapshot's org", func() {
				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("snapshot_id", strconv.Itoa(int(snap2.ID)))

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("forbidden"))
				Expect(errDesc).To(Equal("you are not allowed to use this snapshot because you're not a member of the organization"))
			})

			It("responds with HTTP 403 Forbidden when user is a limited member of the snapshot's org", func() {
				_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleLimited)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("snapshot_id", strconv.Itoa(int(snap1.ID)))

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("forbidden"))
				Expect(errDesc).To(Equal("you are not allowed to use this snapshot"))
			})

			Context("when user is a member of the snapshot's org", func() {
				BeforeEach(func() {
					_, err := orgmembership.AddMember(db, o1.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
					Expect(err).To(BeNil())
				})

				It("logs in to the private Docker registyr", func() {
					params := &url.Values{}
					params.Set("host", hs.H.Slug)
					params.Set("name", "Nestorbot")
					params.Set("snapshot_id", strconv.Itoa(int(snap1.ID)))

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusAccepted))

					Eventually(func() int {
						return mockClient.RegistryLoginCalled
					}).Should(Equal(1))

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

				It("creates a container from the snapshot", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					params := &url.Values{}
					params.Set("host", hs.H.Slug)
					params.Set("name", "Nestorbot")
					params.Set("snapshot_id", strconv.Itoa(int(snap1.ID)))

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusAccepted))

					evtCh := make(chan *controllers.Event)
					go func() {
						for msg := range ch { // Ignore events we don't care about.
							evt := msg.(*controllers.Event)
							if evt.Name == controllers.EventContainerCreated {
								evtCh <- evt
								break
							}
						}
					}()

					select {
					case <-time.After(50 * time.Millisecond):
						Fail("Timed out waiting for container created event")
					case evt := <-evtCh:
						Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
						evtData, ok := evt.Data.(map[string]interface{})
						Expect(ok).To(BeTrue())

						cn, ok := evtData["container"].(*serializers.Container)
						Expect(ok).To(BeTrue())

						cntrs, err := container.FindAllByHostID(db, hs.H.ID)
						Expect(err).To(BeNil())
						Expect(cntrs).To(HaveLen(1))

						Expect(cntrs[0].SnapshotID.Int64).To(Equal(snap1.ID))
						Expect(cntrs[0].UserID).To(Equal(u.ID))
						Expect(cntrs[0].HostID).To(Equal(hs.H.ID))
						Expect(cntrs[0].Name.String).To(Equal("Nestorbot"))

						Expect(cn.ID).To(Equal(cntrs[0].ID))
						Expect(cn.UserID).To(Equal(u.ID))
						Expect(cn.HostID).To(Equal(hs.H.ID))
						Expect(cn.SnapshotID).To(Equal(snap1.ID))
						Expect(cn.Name).To(Equal("Nestorbot"))

						expectedImage := fmt.Sprintf("%s/%s", envvars.DockerRegistryHostname, snap1.Repository())
						Expect(cn.Image).To(Equal(expectedImage))

						Expect(mockClient.CreateCalled).To(Equal(1))
						Expect(mockClient.ReceivedImage).To(Equal(expectedImage))

						h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
						Expect(err).To(BeNil())
						Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
					}
				})
			})
		})

		Context("when repo param is given", func() {
			var tmpl *template.Template

			BeforeEach(func() {
				tmpl = &template.Template{
					Slug:        "rails-template",
					Name:        sql.NullString{"rails", true},
					Description: sql.NullString{"A rails stack template", true},
					Image:       "golang:latest",
					Config: `{
						"privileged": true,
						"publish_all": false,
						"ports": ["public::2341", "public::8931"],
						"env": ["SHELL=powershell"]
					}`,
					Type: template.TypeStack,
				}
				Expect(tmpl.Insert(db)).To(BeNil())
			})

			It("persists quickstart in the db", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "discourse-project")
				params.Set("template", tmpl.Slug)
				params.Set("repo", "https://github.com/yeouchien/discourse")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					cn, ok := evtData["container"].(*serializers.Container)
					Expect(ok).To(BeTrue())

					q := &quickstart.Quickstart{}
					err := db.Get(q, `SELECT * FROM quickstarts LIMIT 1`)
					Expect(err).To(BeNil())

					Expect(q.ID).NotTo(BeZero())
					Expect(q.UserID).To(Equal(u.ID))
					Expect(q.ContainerID).To(Equal(cn.ID))
					Expect(q.TemplateID).To(Equal(tmpl.ID))
					Expect(q.CreatedAt).NotTo(BeNil())
					Expect(q.UpdatedAt).NotTo(BeNil())

					t, ok := svr.Tracker.(*test.FakeTracker)
					Expect(ok).To(BeTrue())
					Expect(t.Events[u.ID]).To(Equal([]string{"Quickstart Created"}))
					Expect(t.TrackEventProperties[u.ID]).To(Equal([]map[string]interface{}{
						{
							"template_slug": tmpl.Slug,
							"repo_url":      "https://github.com/yeouchien/discourse",
						},
					}))
				}
			})
		})

		Context("when there is an error from freighter", func() {
			BeforeEach(func() {
				mockClient.CreateError = errors.New("you are not worthy")
			})

			It("publishes a container create failed event and set state to create_failed", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreateFailed {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container create failed event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreateFailed))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())

					requestID, ok := evtData["request_id"].(string)
					Expect(ok).To(BeTrue())
					Expect(requestID).To(Equal(res.Header().Get(constants.NitrousUUIDHeader)))

					name, ok := evtData["name"].(string)
					Expect(ok).To(BeTrue())
					Expect(name).To(Equal("rails-project"))

					hostSlug, ok := evtData["host_slug"].(string)
					Expect(ok).To(BeTrue())
					Expect(hostSlug).To(Equal(hs.H.Slug))

					errStr, ok := evtData["error"].(string)
					Expect(ok).To(BeTrue())
					Expect(errStr).To(Equal("you are not worthy"))

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))
					Expect(cntrs[0].State).To(Equal(container.StateCreateFailed))
				}
			})

			Context("when the error occurs when trying to pull the image", func() {
				BeforeEach(func() {
					mockClient.PullImageError = errors.New("never gonna pull you down")
				})

				It("does not try to create the container", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					params := &url.Values{}
					params.Set("host", hs.H.Slug)
					params.Set("name", "rails-project")
					params.Set("image", "rails")

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusAccepted))

					evtCh := make(chan *controllers.Event)
					go func() {
						for msg := range ch { // Ignore events we don't care about.
							evt := msg.(*controllers.Event)
							if evt.Name == controllers.EventContainerCreateFailed {
								evtCh <- evt
								break
							}
						}
					}()

					select {
					case <-time.After(50 * time.Millisecond):
						Fail("Timed out waiting for container create failed event")
					case evt := <-evtCh:
						Expect(evt.Name).To(Equal(controllers.EventContainerCreateFailed))

						Expect(mockClient.PullImageCalled).To(Equal(1))
						Expect(mockClient.CreateCalled).To(Equal(0))
					}
				})
			})

			Context("when the error is a StartFailedError", func() {
				BeforeEach(func() {
					mockClient.CreateError = &freighterclient.StartFailedError{"start failed: could not bind port 22"}
				})

				It("still creates the container as if no error had occurred", func() {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					params := &url.Values{}
					params.Set("host", hs.H.Slug)
					params.Set("name", "rails-project")
					params.Set("image", "rails")

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusAccepted))

					evtCh := make(chan *controllers.Event)
					go func() {
						for msg := range ch { // Ignore events we don't care about.
							evt := msg.(*controllers.Event)
							if evt.Name == controllers.EventContainerCreated {
								evtCh <- evt
								break
							}
						}
					}()

					select {
					case <-time.After(50 * time.Millisecond):
						Fail("Timed out waiting for container created event")
					case evt := <-evtCh:
						Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
						evtData, ok := evt.Data.(map[string]interface{})
						Expect(ok).To(BeTrue())

						cn, ok := evtData["container"].(*serializers.Container)
						Expect(ok).To(BeTrue())

						cntrs, err := container.FindAllByHostID(db, hs.H.ID)
						Expect(err).To(BeNil())
						Expect(cntrs).To(HaveLen(1))

						expectSerializedContainer(cn, cntrs[0])

						h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
						Expect(err).To(BeNil())
						Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
					}
				})
			})
		})

		Context("when user has connected to GitHub", func() {
			type addGitHubKeyFunc func(env *middleware.Env, h *host.Host, cn *container.Container, publicKey string) error

			var (
				pubkey                 string
				fp                     string
				addGitHubKeyFuncCalled int
				addedKey               string
				origAddGitHubKeyFunc   addGitHubKeyFunc
			)

			BeforeEach(func() {
				up, err := userprofile.Find(db, u.ID)
				Expect(err).NotTo(HaveOccurred())
				up.GitHubUserName = sql.NullString{"github-user", true}
				up.GitHubOAuthTokenJson = sql.NullString{"{}", true}
				err = up.UpdateGithubInfo(db)
				Expect(err).NotTo(HaveOccurred())

				pubkey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDATOQiODD9fe8IrqVuQKCN/8PyqCXOYrREqpmsi4Jx3m4VdwP0IwrmOylvkDYGG7HVOHKvlncwrIkDJKXlyplaqs6O5D+daY1JLuosqckGPzBlOFchdUjuqiDxdLPBEfOYr2ANZjfLjAyuGqsNAXwhn5+LHKXiXg7Le6feuEsZhP0JyAnEg9diLcmyv8f2rOPqM3L2SmZ38GbWJm1rO7+LVUmroNNa1X1UJHyQFyNOEtXHbYuoZ0xEui6UDJU/DAoVxSfTRzg9XKUWArTsKeGRmJoQBmRRtoFVe8kbs1qnJQCnHnkyH6BhoqFZ/wl7YNRvsGAta1Q1qiqLFSOYZvbT"
				fp, err = sshkey.Md5Fingerprint(pubkey)
				mockClient.ListContainerKeysReturn = []*freighterclient.ContainerKey{
					&freighterclient.ContainerKey{Key: pubkey, Fingerprint: fp},
				}

				addGitHubKeyFuncCalled = 0
				origAddGitHubKeyFunc = github.AddKeyToGitHub
				github.AddKeyToGitHub = func(env *middleware.Env, h *host.Host, cn *container.Container, publicKey string) error {
					addGitHubKeyFuncCalled++
					addedKey = publicKey
					return nil
				}
			})

			AfterEach(func() {
				github.AddKeyToGitHub = origAddGitHubKeyFunc
			})

			It("fetches the container's public key, adds it to GitHub, and updates the container record with the public key's fingerprint", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				params := &url.Values{}
				params.Set("host", hs.H.Slug)
				params.Set("name", "rails-project")
				params.Set("image", "rails")

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				evtCh := make(chan *controllers.Event)
				go func() {
					for msg := range ch { // Ignore events we don't care about.
						evt := msg.(*controllers.Event)
						if evt.Name == controllers.EventContainerCreated {
							evtCh <- evt
							break
						}
					}
				}()

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for container created event")
				case evt := <-evtCh:
					Expect(evt.Name).To(Equal(controllers.EventContainerCreated))

					Eventually(func() int {
						return addGitHubKeyFuncCalled
					}).Should(Equal(1))

					Expect(addedKey).To(Equal(pubkey))

					cntrs, err := container.FindAllByHostID(db, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(cntrs).To(HaveLen(1))

					cn := cntrs[0]
					Expect(cn.PublicKeyFingerprint.String).To(Equal(fp))

					h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
					Expect(err).To(BeNil())
					Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
				}
			})
		})
	})
})
