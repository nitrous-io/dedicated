package images_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/images"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/flusher"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	cert.KeyLen = 512
}

var _ = Describe("controllers/images/pull", func() {
	var (
		db  *db.Database
		env *middleware.Env
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil(), "failed to setup test database")

		env = &middleware.Env{
			Events:  pubsub.New(128),
			Tracker: analytics.NewMixpanelTracker(envvars.MixpanelToken),
			DB:      db,
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	It("creates an image record", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		rI, err := ctx.doPull(env, url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		}, false, nil)
		Expect(err).To(BeNil())

		u, h := ctx.user, ctx.host
		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to store image in db")
		Expect(i.UserID).To(Equal(u.ID))
		Expect(i.HostID).To(Equal(h.ID))
		Expect(i.FreighterID).To(Equal("123asd"))
		Expect(i.Name).To(Equal("scratcher v1 test"))
		Expect(i.Slug).To(Equal("scratch"))
		Expect(i.Tag).To(Equal("latest"))
		Expect(i.CreatedAt).NotTo(BeNil())
		Expect(i.UpdatedAt).NotTo(BeNil())
		Expect(rI.ID).To(Equal(i.ID))
	})

	It("uses the default image if repository is not given", func() {
		fs := newFreighterServer("nitrousio/ubuntu", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		_, err := ctx.doPull(env, url.Values{}, false, nil)
		Expect(err).To(BeNil())

		u, h := ctx.user, ctx.host
		i, err := image.Find(db, u, h, "nitrousio/ubuntu:latest")
		Expect(err).To(BeNil(), "failed to store image in db")
		Expect(i.Slug).To(Equal("nitrousio/ubuntu"))
		Expect(i.Tag).To(Equal("latest"))
	})

	It("removes the image and emits images removed event when freighter failed to pull image", func() {
		fs := newFreighterServer("scratch", "latest")
		fs.pullError = true
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		u, h := ctx.user, ctx.host
		ch := env.Events.Subscribe(u.Topic())
		defer env.Events.Unsubscribe(ch, u.Topic())

		_, err := ctx.doPull(env, url.Values{"repository": []string{"scratch"}}, false, nil)
		Expect(err).ToNot(BeNil())

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(Equal(sql.ErrNoRows), "expect the image to not exist in db, but got %v", i)

		ech := make(chan *controllers.Event, 1)

		go func() {
			var e *controllers.Event
			for {
				e = (<-ch).(*controllers.Event)
				if e.Name == controllers.EventImageRemoved {
					ech <- e
					break
				}
			}
		}()

		select {
		case <-time.After(50 * time.Millisecond):
			Fail("timeout while getting image removed event")
		case e := <-ech:
			evtData, ok := e.Data.(map[string]*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(evtData["image"].Repo).To(Equal("scratch:latest"), "failed to serialize image response")
		}
	})

	It("writes with progress data on status", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		var wr bytes.Buffer
		_, err := ctx.doPull(env, url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		}, false, &wr)
		Expect(err).To(BeNil())

		Expect(wr.String()).To(ContainSubstring(`{"status":"Pulling..."}`), "failed to include progress data")
	})

	It("publishes an image pulled event with the pulled image", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		u, h := ctx.user, ctx.host
		ch := env.Events.Subscribe(u.Topic())
		defer env.Events.Unsubscribe(ch, u.Topic())

		_, err := ctx.doPull(env, url.Values{
			"repository": []string{"scratch"},
		}, false, nil)

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to find image after pull")

		var e *controllers.Event
		timeout := time.After(1000 * time.Millisecond)
		for {
			if e != nil && e.Name == controllers.EventImagePulled {
				break
			}
			select {
			case ev := <-ch:
				e = ev.(*controllers.Event)
			case <-timeout:
				Fail("expected EventImagePulled but timed out")
			}
		}
		evtData, ok := e.Data.(map[string]*serializers.Image)
		Expect(ok).To(BeTrue())
		img := evtData["image"]
		Expect(img.ID).To(Equal(i.ID), "failed to serialize correct image")
		Expect(img.FreighterID).To(Equal("123asd"), "failed to serialize freighter response")
	})

	It("returns an error and emits image pull failed event", func() {
		fs := newFreighterServer("scratch", "latest")
		fs.pullError = true
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		ch := env.Events.Subscribe(ctx.user.Topic())
		defer env.Events.Unsubscribe(ch, ctx.user.Topic())

		_, err := ctx.doPull(env, url.Values{
			"repository": []string{"scratch"},
		}, false, nil)

		Expect(err).ToNot(BeNil())

		ech := make(chan *controllers.Event, 1)

		go func() {
			var e *controllers.Event
			for {
				e = (<-ch).(*controllers.Event)
				if e.Name == controllers.EventImagePullFailed {
					ech <- e
					break
				}
			}
		}()

		select {
		case <-time.After(50 * time.Millisecond):
			Expect(true).To(BeFalse(), "timeout while getting image pull failed event")
		case e := <-ech:
			evtData, ok := e.Data.(map[string]*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(evtData["image"].Slug).To(Equal("scratch"), "failed to serialize freighter response")
		}
	})

	Context("when image already exists", func() {
		It("updates the existing image record with the new info from Freighter", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)
			fs.imageUpdated = true
			fs.start(db, ctx)
			defer fs.Close()

			_, err := ctx.doPull(env, url.Values{
				"repository": []string{"scratch"},
				"update":     []string{"true"},
			}, false, nil)

			i, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find stored image after pull")
			Expect(i.FreighterID).To(Equal("123asd"))
		})

		It("does not update the existing image record if update=false", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)

			fs.imageUpdated = false
			fs.start(db, ctx)
			defer fs.Close()

			_, err := ctx.doPull(env, url.Values{
				"repository": []string{"scratch"},
				"update":     []string{"false"},
			}, false, nil)

			i2, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil())
			Expect(i2.FreighterID).To(Equal("cafebabe"))
		})

		It("does not remove the image when freighter fails to pull image", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)

			fs.imageUpdated = true
			fs.pullError = true
			fs.start(db, ctx)
			defer fs.Close()

			_, err := ctx.doPull(env, url.Values{
				"repository": []string{"scratch"},
			}, false, nil)

			_, err = image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil(), "expect the old image to not be deleted")
		})

		It("does not emit images removed event when freighter fails to pull image", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)

			fs.imageUpdated = true
			fs.pullError = true
			fs.start(db, ctx)
			defer fs.Close()

			ch := env.Events.Subscribe(ctx.user.Topic())
			defer env.Events.Unsubscribe(ch, ctx.user.Topic())

			_, err := ctx.doPull(env, url.Values{
				"repository": []string{"scratch"},
			}, false, nil)
			close(ch)

			_, err = image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil(), "expect the old image to not be deleted")

			failed := make(chan *controllers.Event, 1)
			finished := make(chan struct{}, 1)

			go func() {
				var e *controllers.Event
				var ok bool
				for {
					e, ok = (<-ch).(*controllers.Event)
					if !ok {
						close(finished)
						break
					}
					if e.Name == controllers.EventImageRemoved {
						failed <- e
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("timeout while getting image image removed event")
			case fe := <-failed:
				Expect(fe).To(BeNil(), "should not receive image removed event")
			case <-finished:
				// finished without receiving image removed
			}
		})
	})

	It("should update the progress publisher's ID and Slug", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		ctx.pp = &controllers.ProgressPublisher{}

		_, err := ctx.doPull(env, url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		}, false, nil)
		Expect(err).To(BeNil())

		i, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
		Expect(err).To(BeNil())

		Expect(ctx.pp.ID).To(Equal(i.ID))
		Expect(ctx.pp.Slug).To(Equal(i.Slug))
	})
})

// freighter test server
type freighterServer struct {
	*httptest.Server
	image string
	tag   string

	pullError     bool
	existingImage *freighterclient.Image
	imageUpdated  bool

	pushError    bool
	pushUnAuthed bool
	pushCalled   bool
}

func (s *freighterServer) inspectImage(w http.ResponseWriter, r *http.Request) {
	if s.existingImage != nil && !s.imageUpdated {
		json.NewEncoder(w).Encode(map[string]freighterclient.Image{
			"image": *s.existingImage,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]freighterclient.Image{
		"image": {
			ID:          "123asd",
			Name:        s.image,
			Tag:         s.tag,
			CreatedAt:   time.Now().Unix(),
			ActualSize:  42,
			VirtualSize: 666,
		},
	})
}

type imgTestCtx struct {
	user  *user.User
	token *token.Token
	host  *host.Host
	pp    *controllers.ProgressPublisher
}

func (s *freighterServer) address() (string, int64) {
	addr := s.Listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)
	portInt, _ := strconv.ParseInt(port, 10, 64)
	return host, portInt
}

func (s *freighterServer) start(db *db.Database, ctx *imgTestCtx) {
	crt, err := cert.Find(db, ctx.host.ID)
	Expect(err).To(BeNil(), "no cert found for host")
	s.Server.TLS, err = test.TLSConfig(ctx.host.Hostname(), []byte(crt.ServerCert), []byte(crt.ServerKey))
	Expect(err).To(BeNil(), "bad tls config")
	s.StartTLS()
}

func newFreighterServer(image, tag string) *freighterServer {
	s := &freighterServer{image: image, tag: tag, imageUpdated: true}
	r := mux.NewRouter()
	r.HandleFunc("/images/pull", s.pullImage).Methods("POST")
	r.HandleFunc("/images/push", s.pushImage).Methods("POST")
	r.HandleFunc("/images/inspect", s.inspectImage).Methods("GET")
	s.Server = httptest.NewUnstartedServer(r)
	return s
}

// insert a dummy image to be used in case the image exists in db
func (c *imgTestCtx) insertImage(fs *freighterServer, db *db.Database) {
	fs.existingImage = &freighterclient.Image{ID: "cafebabe", Name: "scratch", Tag: "latest"}

	u, h := c.user, c.host
	i := &image.Image{
		UserID:      u.ID,
		HostID:      h.ID,
		FreighterID: "cafebabe",
		Name:        "scratch v1",
		Slug:        "scratch",
		Tag:         "latest",
	}
	err := i.Insert(db)
	Expect(err).To(BeNil(), "failed to insert test image")
}

func newImgTestCtx(fs *freighterServer, db *db.Database) *imgTestCtx {
	fsHost, fsPort := fs.address()

	var err error
	u := &user.User{
		Username: "foo",
		Password: "123456",
		Email:    "foo@example.com",
	}

	err = test.WrapInTransaction(db, u.Insert)
	Expect(err).To(BeNil(), "failed to setup test user")

	t := &token.Token{
		UserID: u.ID,
		Token:  "valid-token",
	}
	err = t.Insert(db)
	Expect(err).To(BeNil(), "failed to setup test user's token")

	hs, err := test.CreateHostStack(db, &test.HostStack{
		H: &host.Host{
			Slug:           "foo",
			IPAddress:      sql.NullString{fsHost, true},
			FreighterdPort: sql.NullInt64{fsPort, true},
		},
		U: u,
	})
	Expect(err).To(BeNil(), "failed to prepare host stack")

	hs, err = test.GenerateHostCert(db, hs)
	Expect(err).To(BeNil(), "failed to generate host cert")

	return &imgTestCtx{
		user:  u,
		token: t,
		host:  hs.H,
	}
}

func (c *imgTestCtx) doPull(env *middleware.Env, params url.Values, async bool, writer io.Writer) (*image.Image, error) {
	if env.User == nil {
		env.User = c.user
	}
	if writer == nil {
		writer = ioutil.Discard
	}
	return images.PullImage(c.host, params, env, writer, c.pp, async)
}

func (s *freighterServer) pullImage(w http.ResponseWriter, r *http.Request) {
	if s.pullError {
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
	fmt.Fprintln(stream, `{"status":"Pulling..."}`)
	fmt.Fprintln(stream, `{"status":"Pulling","progress":"1 B/ 100 B"}`)
	fmt.Fprintln(stream, `{"status":"Pulling","progress":"2 B/ 100 B"}`)
}
