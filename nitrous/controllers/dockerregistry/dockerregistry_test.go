package dockerregistry_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/dockerregistry")
}

var _ = Describe("controllers/dockerregistry", func() {
	var (
		db  *ndb.Database
		svr *server.Server

		snap1, snap2 *snapshot.Snapshot
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, nil, nil)

		u := &user.User{
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

		o1 := &org.Org{
			Name:  "ze robot labs",
			Email: "admin@zerobotlabs.com",
			Slug:  "zerobotlabs",
		}
		Expect(o1.Insert(db)).To(BeNil())

		o2 := &org.Org{
			Name:  "Path",
			Email: "admin@path.com",
			Slug:  "path",
		}
		Expect(o2.Insert(db)).To(BeNil())

		hs1, err := test.CreateHostStack(db, &test.HostStack{
			U: u,
			O: o1,
		})
		Expect(err).To(BeNil())

		hs2, err := test.CreateHostStack(db, &test.HostStack{
			U: u,
			O: o2,
		})
		Expect(err).To(BeNil())

		img1 := &image.Image{
			UserID:      u.ID,
			HostID:      hs1.H.ID,
			FreighterID: "cafebabe",
			Name:        "nestorbot",
			Slug:        "nestorbot",
			Tag:         "latest",
		}
		Expect(img1.Insert(db)).To(BeNil())

		img2 := &image.Image{
			UserID:      u.ID,
			HostID:      hs2.H.ID,
			FreighterID: "deadbeef",
			Name:        "car-os",
			Slug:        "car-os",
			Tag:         "latest",
		}
		Expect(img2.Insert(db)).To(BeNil())

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

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	doRequest := func(token, jsonStr string) *httptest.ResponseRecorder {
		url := "/api/v0/docker/registry/notifications"
		req, err := test.NewBodyRequest("POST", url, jsonStr)
		Expect(err).To(BeNil())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		svr.Router().ServeHTTP(res, req)
		return res
	}

	It("requires a bearer token", func() {
		res := doRequest("", pushEventPayload)
		Expect(res.Code).To(Equal(http.StatusUnauthorized))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_token"))
		Expect(errDesc).To(Equal("auth token is required"))
	})

	It("requires a valid bearer token that matches the DOCKER_REGISTRY_WEBHOOK_TOKEN env var", func() {
		res := doRequest("invalid-token", pushEventPayload)
		Expect(res.Code).To(Equal(http.StatusUnauthorized))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_token"))
		Expect(errDesc).To(Equal("auth token is invalid"))
	})

	Context("push event", func() {
		It("updates a snapshot's size", func() {
			// Size is initially 0.
			Expect(snap1.Size.Int64).To(Equal(int64(0)))

			res := doRequest(envvars.DockerRegistryWebhookToken, pushEventPayload)
			Expect(res.Code).To(Equal(http.StatusOK))

			s, err := snapshot.FindByID(db, snap1.ID)
			Expect(err).To(BeNil())

			Expect(s.Slug).To(Equal(snap1.Slug))
			Expect(s.Size.Int64).To(Equal(int64(1234)))
		})

		It("responds with HTTTP 200 OK and does nothing snapshot does not exist", func() {
			res := doRequest(envvars.DockerRegistryWebhookToken, `{
				"events": [
					{
						"id": "abcd-abcd-abcd-abcd-0",
						"timestamp": "2006-01-02T15:04:05Z",
						"action": "push",
						"target": {
							"repository": "zerobotlabs/does-not-exist",
							"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
							"size": 1234,
							"digest": "sha256:0123456789abcdef0",
							"length": 1234,
							"url": "https://registry.nitrous.io/v2/zerobotlabs/does-not-exist/manifests/sha256:1041b6b4ca60d2"
						},
						"request": {
							"id": "abcdabcd",
							"addr": "registryclient.local",
							"host": "registry.nitrous.io",
							"method": "PUT",
							"useragent": "docker/1.9.1 go/go1.4.2"
						},
						"actor": {
							"name": "aroonthampy"
						},
						"source": {
							"addr": "registry.nitrous.io:5000",
							"instanceID": "abcd"
						}
					}
				]
			}`)
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Context("pull event", func() {
		It("increments a snapshot's number of downloads by 1", func() {
			prevDownloads := snap1.Downloads

			res := doRequest(envvars.DockerRegistryWebhookToken, pullEventPayload)
			Expect(res.Code).To(Equal(http.StatusOK))

			s, err := snapshot.FindByID(db, snap1.ID)
			Expect(err).To(BeNil())

			Expect(s.Slug).To(Equal(snap1.Slug))
			Expect(s.Downloads).To(Equal(prevDownloads + 1))
		})

		It("responds with HTTTP 200 OK and does nothing snapshot does not exist", func() {
			res := doRequest(envvars.DockerRegistryWebhookToken, `{
				"events": [
					{
						"id": "abcd-abcd-abcd-abcd-0",
						"timestamp": "2006-01-02T15:04:05Z",
						"action": "pull",
						"target": {
							"repository": "zerobotlabs/does-not-exist",
							"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
							"size": 1234,
							"digest": "sha256:0123456789abcdef0",
							"length": 1234,
							"url": "https://registry.nitrous.io/v2/zerobotlabs/does-not-exist/manifests/sha256:1041b6b4ca60d2"
						},
						"request": {
							"id": "abcdabcd",
							"addr": "registryclient.local",
							"host": "registry.nitrous.io",
							"method": "PUT",
							"useragent": "docker/1.9.1 go/go1.4.2"
						},
						"actor": {
							"name": "aroonthampy"
						},
						"source": {
							"addr": "registry.nitrous.io:5000",
							"instanceID": "abcd"
						}
					}
				]
			}`)
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Context("combination of multiple events", func() {
		It("processes all the events", func() {
			// Size is initially 0.
			Expect(snap1.Size.Int64).To(Equal(int64(0)))

			prevDownloads := snap2.Downloads

			res := doRequest(envvars.DockerRegistryWebhookToken, `{
				"events": [
					{
						"id": "abcd-abcd-abcd-abcd-0",
						"timestamp": "2006-01-02T15:04:05Z",
						"action": "push",
						"target": {
							"repository": "zerobotlabs/nestorbot",
							"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
							"size": 1234,
							"digest": "sha256:0123456789abcdef0",
							"length": 1234,
							"url": "https://registry.nitrous.io/v2/zerobotlabs/nestorbot/manifests/sha256:1041b6b4ca60d1"
						},
						"request": {
							"id": "abcdabcd",
							"addr": "registryclient.local",
							"host": "registry.nitrous.io",
							"method": "PUT",
							"useragent": "docker/1.9.1 go/go1.4.2"
						},
						"actor": {
							"name": "aroonthampy"
						},
						"source": {
							"addr": "registry.nitrous.io:5000",
							"instanceID": "abcd"
						}
					},
					{
						"id": "abcd-abcd-abcd-abcd-1",
						"timestamp": "2006-01-02T15:04:05Z",
						"action": "pull",
						"target": {
							"repository": "path/car-os",
							"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
							"size": 123871,
							"digest": "sha256:0123456789abcdef1",
							"length": 123871,
							"url": "https://registry.nitrous.io/v2/path/car-os/manifests/sha256:1041b6b4ca60d2"
						},
						"request": {
							"id": "abcdabce",
							"addr": "registryclient.local",
							"host": "registry.nitrous.io",
							"method": "PUT",
							"useragent": "docker/1.9.1 go/go1.4.2"
						},
						"actor": {
							"name": "elonmusk"
						},
						"source": {
							"addr": "registry.nitrous.io:5000",
							"instanceID": "abce"
						}
					}
				]
			}`)
			Expect(res.Code).To(Equal(http.StatusOK))

			s1, err := snapshot.FindByID(db, snap1.ID)
			Expect(err).To(BeNil())

			Expect(s1.Slug).To(Equal(snap1.Slug))
			Expect(s1.Size.Int64).To(Equal(int64(1234)))

			s2, err := snapshot.FindByID(db, snap2.ID)
			Expect(err).To(BeNil())

			Expect(s2.Slug).To(Equal(snap2.Slug))
			Expect(s2.Downloads).To(Equal(prevDownloads + 1))
		})
	})

	Context("unsupported event", func() {
		It("responds with HTTP 200 OK and does nothing", func() {
			// Send a "delete" action, which is unsupported now.
			res := doRequest(envvars.DockerRegistryWebhookToken, deleteEventPayload)
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})
})

const pushEventPayload = `{
	"events": [
		{
			"id": "abcd-abcd-abcd-abcd-0",
			"timestamp": "2006-01-02T15:04:05Z",
			"action": "push",
			"target": {
				"repository": "zerobotlabs/nestorbot",
				"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
				"size": 1234,
				"digest": "sha256:0123456789abcdef0",
				"length": 1234,
				"url": "https://registry.nitrous.io/v2/zerobotlabs/nestorbot/manifests/sha256:1041b6b4ca60d2"
			},
			"request": {
				"id": "abcdabcd",
				"addr": "registryclient.local",
				"host": "registry.nitrous.io",
				"method": "PUT",
				"useragent": "docker/1.9.1 go/go1.4.2"
			},
			"actor": {
				"name": "aroonthampy"
			},
			"source": {
				"addr": "registry.nitrous.io:5000",
				"instanceID": "abcd"
			}
		}
	]
}`
const pullEventPayload = `{
	"events": [
		{
			"id": "abcd-abcd-abcd-abcd-0",
			"timestamp": "2006-01-02T15:04:05Z",
			"action": "pull",
			"target": {
				"repository": "zerobotlabs/nestorbot",
				"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
				"size": 1234,
				"digest": "sha256:0123456789abcdef0",
				"length": 1234,
				"url": "https://registry.nitrous.io/v2/zerobotlabs/nestorbot/manifests/sha256:1041b6b4ca60d2"
			},
			"request": {
				"id": "abcdabcd",
				"addr": "registryclient.local",
				"host": "registry.nitrous.io",
				"method": "PUT",
				"useragent": "docker/1.9.1 go/go1.4.2"
			},
			"actor": {
				"name": "aroonthampy"
			},
			"source": {
				"addr": "registry.nitrous.io:5000",
				"instanceID": "abcd"
			}
		}
	]
}`
const deleteEventPayload = `{
	"events": [
		{
			"id": "abcd-abcd-abcd-abcd-0",
			"timestamp": "2006-01-02T15:04:05Z",
			"action": "delete",
			"target": {
				"repository": "zerobotlabs/nestorbot",
				"mediaType": "application/vnd.docker.distribution.manifest.v1+json",
				"size": 1234,
				"digest": "sha256:0123456789abcdef0",
				"length": 1234,
				"url": "https://registry.nitrous.io/v2/zerobotlabs/nestorbot/manifests/sha256:1041b6b4ca60d2"
			},
			"request": {
				"id": "abcdabcd",
				"addr": "registryclient.local",
				"host": "registry.nitrous.io",
				"method": "PUT",
				"useragent": "docker/1.9.1 go/go1.4.2"
			},
			"actor": {
				"name": "aroonthampy"
			},
			"source": {
				"addr": "registry.nitrous.io:5000",
				"instanceID": "abcd"
			}
		}
	]
}`
