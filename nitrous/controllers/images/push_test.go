package images_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/flusher"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("controllers/images", func() {
	var (
		db            *db.Database
		svr           *server.Server
		fakeFreighter *freighterServer
		ctx           *imgTestCtx
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, &logger.Logger{Level: logger.LogLevelQuiet}, nil)

		fakeFreighter = newFreighterServer("scratch", "latest")
		ctx = newImgTestCtx(fakeFreighter, db)
		fakeFreighter.start(db, ctx)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		fakeFreighter.Close()
	})

	Describe("POST /api/v0/hosts/{slug}/images/{image}/push", func() {
		It("should block if the user has billing issues", func() {
			u, h := ctx.user, ctx.host
			MarkDelinquentUser(db, u)
			ctx.insertImage(fakeFreighter, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			VerifyDelinquentUserResponse(res)
		})

		It("calls images/push on the freighter server", func() {
			u, h := ctx.user, ctx.host
			ctx.insertImage(fakeFreighter, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(fakeFreighter.pushCalled).To(BeTrue())
		})

		It("requires repository param", func() {
			u, h := ctx.user, ctx.host
			ctx.insertImage(fakeFreighter, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{})
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("repository is required"))
			Expect(fakeFreighter.pushCalled).To(BeFalse())
		})

		It("returns 404 if the image could not be located", func() {
			u, h := ctx.user, ctx.host
			ctx.insertImage(fakeFreighter, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratchy:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("image_not_found"))
			Expect(fakeFreighter.pushCalled).To(BeFalse())
		})

		It("returns 401 if freighterclient returns an ErrImagePushAuthRequired", func() {
			u, h := ctx.user, ctx.host
			ctx.insertImage(fakeFreighter, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			fakeFreighter.pushUnAuthed = true

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("registry_auth_failed"))
			Expect(errDesc).To(Equal(freighterclient.ErrImagePushAuthRequired.Error()))
			Expect(fakeFreighter.pushCalled).To(BeTrue())
		})

		Context("event publishing", func() {
			It("publishes an image pushing event with progress data", func() {
				u, h := ctx.user, ctx.host
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())
				ctx.insertImage(fakeFreighter, db)

				ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})

				timeout := time.After(100 * time.Millisecond)
				select {
				case ev := <-ch:
					// check if the first event we got is image pushing event
					e := ev.(*controllers.Event)
					Expect(e.Name).To(Equal(controllers.EventImagePushing))

					i, err := image.Find(db, u, h, "scratch:latest")
					Expect(err).To(BeNil())
					data := e.Data.(*controllers.ProgressEvent)
					Expect(data.ID).To(Equal(i.ID), "emitted image pushing with invalid image")
					Expect(data.Progress).To(Equal(`{"status":"Pushing..."}`), "failed to include progress data")
				case <-timeout:
					Fail("expected EventImagePushing but timed out")
				}
			})

			It("publishes an image pushed event with the image and the target repository", func() {
				u, h := ctx.user, ctx.host
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())
				ctx.insertImage(fakeFreighter, db)

				res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
				Expect(res.Code).To(Equal(http.StatusOK))

				var e *controllers.Event
				timeout := time.After(1000 * time.Millisecond)
				for {
					if e != nil && e.Name == controllers.EventImagePushed {
						break
					}
					select {
					case ev := <-ch:
						e = ev.(*controllers.Event)
					case <-timeout:
						Fail("expected EventImagePushed but timed out")
					}
				}
				evtData, ok := e.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				img, ok := evtData["image"].(*serializers.Image)
				Expect(ok).To(BeTrue())
				i, err := image.Find(db, u, h, "scratch:latest")
				Expect(err).To(BeNil())
				Expect(img).To(Equal(serializers.SerializeImage(i)))
				Expect(evtData["repository"]).To(Equal("nitrousio/devenv"))
			})

			It("publishes an image pushed failed event with the image, target repository and the error", func() {
				fakeFreighter.pushError = true

				u, h := ctx.user, ctx.host
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())
				ctx.insertImage(fakeFreighter, db)

				res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("internal_server_error"))

				var e *controllers.Event
				timeout := time.After(1000 * time.Millisecond)
				for {
					if e != nil && e.Name == controllers.EventImagePushFailed {
						break
					}
					select {
					case ev := <-ch:
						e = ev.(*controllers.Event)
					case <-timeout:
						Fail("expected EventImagePushFailed but timed out")
					}
				}
				evtData, ok := e.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				img, ok := evtData["image"].(*serializers.Image)
				Expect(ok).To(BeTrue())
				i, err := image.Find(db, u, h, "scratch:latest")
				Expect(err).To(BeNil())
				Expect(img).To(Equal(serializers.SerializeImage(i)))
				Expect(evtData["repository"]).To(Equal("nitrousio/devenv"))
				Expect(evtData["error"]).NotTo(BeEmpty())
			})

		})

		It("returns a 202 Accepted when the async param is sent", func() {
			ctx.insertImage(fakeFreighter, db)

			params := &url.Values{}
			params.Set("async", "true")
			params.Set("repository", "nitrousio/devenv")
			res := ctx.doPush(svr, "foo", "scratch:latest", params)
			Expect(res.Code).To(Equal(http.StatusAccepted))
		})
	})
})

func (c *imgTestCtx) doPush(svr *server.Server, host, slug string, params *url.Values) *httptest.ResponseRecorder {
	req, err := NewAuthedFormRequest(c.token, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/%v/push", host, slug), params)
	Expect(err).To(BeNil(), "failed to POST images/push")
	res := httptest.NewRecorder()
	svr.Router().ServeHTTP(res, req)
	return res
}

func (s *freighterServer) pushImage(w http.ResponseWriter, r *http.Request) {
	s.pushCalled = true

	if s.pushUnAuthed {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"RJ? Is that you?"}`)
		return
	}

	if s.pushError {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"error":"internal server error. dont know why"}`)
		return
	}

	// If image exists, return HTTP 304 Not Modified.
	if r.PostFormValue("update") == "false" && s.existingImage != nil {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	stream := flusher.NewWriteFlusher(w)
	fmt.Fprintln(stream, `{"status":"Pushing..."}`)
	fmt.Fprintln(stream, `{"status":"Pushing", "progress":"1/? (n/a)", "progressDetail":{"current":1}}`)
	fmt.Fprintln(stream, `{"status":"Pushing", "progress":"2/? (n/a)", "progressDetail":{"current":1}}`)
}
