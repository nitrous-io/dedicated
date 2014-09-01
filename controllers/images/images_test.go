package images_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/images")
}

var _ = Describe("controllers/images", func() {
	var (
		db              *db.Database
		svr             *server.Server
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
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

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
		UnstubFreighterFn()
	})

	Describe("GET /api/v0/hosts/{slug}/images", func() {
		doRequest := func(host string) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%v/images", host), nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		var (
			img1 *freighterclient.Image
			img2 *freighterclient.Image

			i1 *image.Image
			i2 *image.Image
		)

		BeforeEach(func() {
			img1 = &freighterclient.Image{
				ID:          "1a",
				Name:        "source1",
				Tag:         "sourcetag1",
				CreatedAt:   time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC).Unix(),
				ActualSize:  13,
				VirtualSize: 29,
			}
			img2 = &freighterclient.Image{
				ID:          "2a",
				Name:        "source2",
				Tag:         "sourcetag2",
				CreatedAt:   time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC).Unix(),
				ActualSize:  14,
				VirtualSize: 30,
			}
			mockClient.ListImagesArray = []*freighterclient.Image{img1, img2}

			i1 = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "1a",
				Name:        "source1",
				Tag:         "sourcetag1",
			}
			err := i1.Insert(db)
			Expect(err).To(BeNil())

			i2 = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "2a",
				Name:        "source2",
				Tag:         "sourcetag2",
			}
			err = i2.Insert(db)
			Expect(err).To(BeNil())
		})

		It("responds with the images from Freighter augmented with data from the database", func() {
			res := doRequest("foo")

			Expect(mockClient.ListImagesCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string][]serializers.Image
			data := res.Body.Bytes()
			err := json.Unmarshal(data, &j)
			Expect(err).To(BeNil())
			images, ok := j["images"]
			Expect(ok).To(BeTrue())
			Expect(images).To(HaveLen(2))

			Expect(images[0].ID).To(Equal("1a"))
			Expect(images[0].Slug).To(Equal("source1"))
			Expect(images[0].Tag).To(Equal("sourcetag1"))
			Expect(images[0].ActualSize).To(Equal(int64(13)))
			Expect(images[0].VirtualSize).To(Equal(int64(29)))
			createdAt := time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			Expect(images[0].CreatedAt).To(Equal(&createdAt))
			Expect(images[0].UpdatedAt).NotTo(BeNil())

			Expect(images[1].ID).To(Equal("2a"))
			Expect(images[1].Slug).To(Equal("source2"))
			Expect(images[1].Tag).To(Equal("sourcetag2"))
			Expect(images[1].ActualSize).To(Equal(int64(14)))
			Expect(images[1].VirtualSize).To(Equal(int64(30)))
			createdAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
			Expect(images[1].CreatedAt).To(Equal(&createdAt))
			Expect(images[1].CreatedAt).NotTo(BeNil())
			Expect(images[1].UpdatedAt).NotTo(BeNil())
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.ListImagesError = errors.New("you are not Chuck Norris")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				res := doRequest("foo")

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("internal_server_error"))
			})
		})

		Context("when host has no images", func() {
			BeforeEach(func() {
				mockClient.ListImagesArray = []*freighterclient.Image{}
			})

			It("responds with an empty array", func() {
				res := doRequest("foo")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}

				data := res.Body.Bytes()
				err := json.Unmarshal(data, &j)
				Expect(err).To(BeNil())
				images, ok := j["images"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(images).To(BeEmpty())
			})
		})

		Context("when host does not exist for user", func() {
			It("returns HTTP 404 Not Found", func() {
				res := doRequest("no-such-host")

				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(mockClient.ListImagesCalled).To(Equal(0))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("not_found"))

				errDesc, _ := j["error_description"].(string)
				Expect(errDesc).To(Equal("host does not exist"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/images"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/image", func() {
		doRequest := func(host string, params *url.Values, files map[string]string) *httptest.ResponseRecorder {
			url := fmt.Sprintf("/api/v0/hosts/%v/images", host)
			req, err := NewFileRequest("POST", url, params, files)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			req.Header.Set("Authorization", "Bearer "+t.Token)
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("creates an image record", func() {
			mockClient.BuildImageReturn = &freighterclient.Image{
				ID:   "cafebabe",
				Name: "scratcher",
				Tag:  "mytag",
			}
			params := &url.Values{}
			params.Add("slug", "scratcher")
			params.Add("name", "scratcher v1 test")
			res := doRequest("foo", params, map[string]string{"file": "FROM fake"})
			Expect(res.Code).To(Equal(http.StatusOK))

			i, err := image.Find(db, u, hs.H, "scratcher:mytag")
			Expect(err).To(BeNil())
			Expect(i.UserID).To(Equal(u.ID))
			Expect(i.HostID).To(Equal(hs.H.ID))
			Expect(i.FreighterID).To(Equal("cafebabe"))
			Expect(i.Name).To(Equal("scratcher v1 test"))
			Expect(i.Slug).To(Equal("scratcher"))
			Expect(i.Tag).To(Equal("mytag"))
			Expect(i.CreatedAt).NotTo(BeNil())
			Expect(i.UpdatedAt).NotTo(BeNil())
		})

		It("responds with image", func() {
			mockClient.BuildImageReturn = &freighterclient.Image{
				ID:   "cafebabe",
				Name: "scratcher",
				Tag:  "mytag",
			}
			params := &url.Values{}
			params.Add("slug", "scratcher")
			res := doRequest("foo", params, map[string]string{"file": "FROM fake"})
			j := struct {
				Image serializers.Image `json:"image"`
			}{}
			sent, err := ioutil.ReadAll(mockClient.BuildImageReader)
			Expect(err).To(BeNil())
			Expect(string(sent)).To(Equal("FROM fake"))
			Expect(mockClient.BuildImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Image.Slug).To(Equal("scratcher"))
		})

		It("publishes an image.built event with the built image", func(done Done) {
			mockClient.BuildImageReturn = &freighterclient.Image{
				ID:   "cafebabe",
				Name: "scratcher",
				Tag:  "mytag",
			}

			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			params := &url.Values{}
			params.Add("slug", "scratcher")
			doRequest("foo", params, map[string]string{"file": "FROM fake"})

			i, err := image.Find(db, u, hs.H, "scratcher:mytag")
			Expect(err).To(BeNil())

			Expect(<-ch).To(Equal(&controllers.Event{
				Name: "image.built",
				Data: serializers.SerializeImage(i),
			}))
			close(done)
		})

		It("requires slug", func() {
			res := doRequest("foo", &url.Values{}, map[string]string{"file": "filecontent"})
			Expect(mockClient.BuildImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("slug is required"))
		})

		It("requires file", func() {
			params := &url.Values{}
			params.Add("slug", "scratcher")
			res := doRequest("foo", params, map[string]string{})
			Expect(mockClient.BuildImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("file is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.BuildImageError = &freighterclient.StatusError{418}
			params := &url.Values{}
			params.Add("slug", "scratcher")
			res := doRequest("foo", params, map[string]string{"file": "FROM fake"})
			Expect(mockClient.BuildImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}/image/{image}", func() {
		BeforeEach(func() {
			i := &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Slug:        "nitrousio/elixir",
				Tag:         "latest",
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(host, image string) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "DELETE", fmt.Sprintf("/api/v0/hosts/%v/images/%v", host, image), nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("calls RemoveImage", func() {
			doRequest("foo", "nitrousio/elixir")
			Expect(mockClient.RemoveImageCalled).To(Equal(1))
		})

		It("deletes image record from the database", func() {
			_, err := image.Find(db, hs.U, hs.H, "nitrousio/elixir")
			Expect(err).To(BeNil())

			res := doRequest("foo", "nitrousio/elixir")
			Expect(res.Code).To(Equal(http.StatusOK))

			_, err = image.Find(db, hs.U, hs.H, "nitrousio/elixir")
			Expect(err).To(MatchError(sql.ErrNoRows))
		})

		It("publishes an image.removed event with the image ID/slug", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			res := doRequest("foo", "nitrousio/elixir")
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(<-ch).To(Equal(&controllers.Event{
				Name: "image.removed",
				Data: "nitrousio/elixir:latest",
			}))
			close(done)
		})

		Context("when trying to delete an image that does not exist in the database", func() {
			It("returns HTTP 400", func() {
				res := doRequest("foo", "nonexistent")
				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("image does not exist"))
			})
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/pull", func() {
		doRequest := func(host string, params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/pull", host), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("creates an image record", func() {
			mockClient.PullImageReturn = &freighterclient.Image{
				ID:   "pulledbeef",
				Name: "scratch",
				Tag:  "latest",
			}
			doRequest("foo", &url.Values{
				"repository": []string{"scratch"},
				"name":       []string{"scratcher v1 test"},
			})

			i, err := image.Find(db, u, hs.H, "scratch:latest")
			Expect(err).To(BeNil())
			Expect(i.UserID).To(Equal(u.ID))
			Expect(i.HostID).To(Equal(hs.H.ID))
			Expect(i.FreighterID).To(Equal("pulledbeef"))
			Expect(i.Name).To(Equal("scratcher v1 test"))
			Expect(i.Slug).To(Equal("scratch"))
			Expect(i.Tag).To(Equal("latest"))
			Expect(i.CreatedAt).NotTo(BeNil())
			Expect(i.UpdatedAt).NotTo(BeNil())
		})

		Context("when image already exists", func() {
			BeforeEach(func() {
				i := &image.Image{
					UserID:      hs.U.ID,
					HostID:      hs.H.ID,
					FreighterID: "cafebabe",
					Name:        "scratch v1",
					Slug:        "scratch",
					Tag:         "latest",
				}
				err := i.Insert(db)
				Expect(err).To(BeNil())
			})

			It("updates the existing image record with the new info from Freighter", func() {
				mockClient.PullImageReturn = &freighterclient.Image{
					ID:   "cafedead",
					Name: "scratch",
					Tag:  "latest",
				}
				doRequest("foo", &url.Values{"repository": []string{"scratch"}})

				i, err := image.Find(db, u, hs.H, "scratch:latest")
				Expect(err).To(BeNil())
				Expect(i.FreighterID).To(Equal("cafedead"))
			})
		})

		It("responds with image on success", func() {
			mockClient.PullImageReturn = &freighterclient.Image{
				ID:   "pulledbeef",
				Name: "scratch",
				Tag:  "latest",
			}
			res := doRequest("foo", &url.Values{"repository": []string{"scratch"}})

			Expect(mockClient.PullImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
			j := struct {
				Image serializers.Image `json:"image"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Image.ID).To(Equal("pulledbeef"))
			Expect(j.Image.Slug).To(Equal("scratch"))
			Expect(j.Image.Tag).To(Equal("latest"))
		})

		It("publishes an image.pulled event with the pulled image", func(done Done) {
			mockClient.PullImageReturn = &freighterclient.Image{
				ID:   "pulledbeef",
				Name: "scratch",
				Tag:  "latest",
			}

			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("foo", &url.Values{"repository": []string{"scratch"}})

			i, err := image.Find(db, u, hs.H, "scratch:latest")
			Expect(err).To(BeNil())

			Expect(<-ch).To(Equal(&controllers.Event{
				Name: "image.pulled",
				Data: serializers.SerializeImage(i),
			}))
			close(done)
		})

		It("requires repository param", func() {
			res := doRequest("foo", nil)
			Expect(mockClient.PullImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("repository is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.PullImageError = &freighterclient.StatusError{418}
			res := doRequest("foo", &url.Values{"repository": []string{"scratch"}})
			Expect(mockClient.PullImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/{image}/push", func() {
		doRequest := func(host, image string, params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/%s/push", host, image), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("calls PushImage", func() {
			res := doRequest("foo", "nitrousio/devenv:latest", &url.Values{"destination": []string{"myuser/scratch"}})
			Expect(mockClient.PushImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("publishes an image.pushed event with the image ID/slug and destination", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("foo", "nitrousio/devenv:latest", &url.Values{"destination": []string{"myuser/scratch"}})

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal("image.pushed"))
			Expect(evt.Data).To(BeEquivalentTo(struct {
				Image       string `json:"image"`
				Destination string `json:"destination"`
			}{
				Image:       "nitrousio/devenv:latest",
				Destination: "myuser/scratch",
			}))
			close(done)
		})

		It("requires destination param", func() {
			res := doRequest("foo", "devenv", nil)
			Expect(mockClient.PushImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("destination is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.PushImageError = &freighterclient.StatusError{418}
			res := doRequest("foo", "devenv", &url.Values{"destination": []string{"scratch"}})
			Expect(mockClient.PushImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/{image}/tag", func() {
		BeforeEach(func() {
			i := &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Name:        "scratch v1",
				Slug:        "nitrousio/scratch",
				Tag:         "latest",
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(host, image string, params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/%s/tag", host, image), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("creates an image record", func() {
			i, err := image.Find(db, u, hs.H, "staging")
			Expect(err).To(MatchError(sql.ErrNoRows))

			res := doRequest("foo", "nitrousio/scratch", &url.Values{
				"tag":  []string{"staging"},
				"name": []string{"staging v1 from scratch"},
			})
			Expect(res.Code).To(Equal(http.StatusOK))

			i, err = image.Find(db, u, hs.H, "staging")
			Expect(err).To(BeNil())
			Expect(i.FreighterID).To(Equal("cafebabe"))
			Expect(i.Name).To(Equal("staging v1 from scratch"))
			Expect(i.Slug).To(Equal("staging"))
			Expect(i.Tag).To(Equal("latest"))
		})

		Context("when image already exists", func() {
			BeforeEach(func() {
				i := &image.Image{
					UserID:      hs.U.ID,
					HostID:      hs.H.ID,
					FreighterID: "deadbeef",
					Name:        "staging image",
					Slug:        "staging", // We'll attempt to tag a new image with this image's repo.
					Tag:         "latest",
				}
				err := i.Insert(db)
				Expect(err).To(BeNil())
			})

			It("updates the existing image record with the source image", func() {
				res := doRequest("foo", "nitrousio/scratch", &url.Values{
					"tag":  []string{"staging"},
					"name": []string{"staging v1 from scratch"},
				})
				Expect(res.Code).To(Equal(http.StatusOK))

				i, err := image.Find(db, u, hs.H, "staging")
				Expect(err).To(BeNil())
				Expect(i.FreighterID).To(Equal("cafebabe"))
				Expect(i.Name).To(Equal("staging v1 from scratch"))
				Expect(i.Slug).To(Equal("staging"))
				Expect(i.Tag).To(Equal("latest"))
			})
		})

		It("calls TagImage", func() {
			res := doRequest("foo", "nitrousio/scratch", &url.Values{"tag": []string{"staging"}})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.ReceivedImage).To(Equal("nitrousio/scratch:latest"))
			Expect(mockClient.ReceivedTag).To(Equal("staging"))
			Expect(mockClient.TagImageForce).To(BeFalse())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("publishes an image.tagged event with the image ID/name and tag", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("foo", "nitrousio/scratch:latest", &url.Values{"tag": []string{"staging"}})

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal("image.tagged"))
			img, ok := evt.Data.(*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(img.ID).To(Equal("cafebabe"))
			Expect(img.Slug).To(Equal("staging"))
			Expect(img.Tag).To(Equal("latest"))
			close(done)
		})

		It("sends force param", func() {
			res := doRequest("foo", "nitrousio/scratch", &url.Values{"tag": []string{"staging"}, "force": []string{"1"}})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.TagImageForce).To(BeTrue())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("requires tag param", func() {
			res := doRequest("foo", "dontcare", nil)
			Expect(mockClient.TagImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("tag is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.TagImageError = &freighterclient.StatusError{418}
			res := doRequest("foo", "nitrousio/scratch", &url.Values{"tag": []string{"staging"}})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})

		Context("when trying to tag an image that does not exist in the database", func() {
			It("returns HTTP 400", func() {
				res := doRequest("foo", "nonexistent", &url.Values{"tag": []string{"staging"}})
				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("image does not exist"))
			})
		})
	})
})
