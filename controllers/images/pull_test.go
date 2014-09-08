package images_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/controllers/images"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/flusher"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"
	"github.com/nitrous-io/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("controllers/images/pull", func() {
	var (
		db  *db.Database
		svr *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil(), "failed to setup test database")
		svr = server.NewServer("", db, &logger.Logger{Level: logger.LogLevelQuiet})
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	It("creates an image record", func() {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()

		ctx := newPullTestCtx(fs, db)
		ctx.do(svr, "foo", &url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		})

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
	})

	It("removes the image when freighter failed to pull image", func() {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()
		fs.pullError = true

		ctx := newPullTestCtx(fs, db)
		ctx.do(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		u, h := ctx.user, ctx.host
		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(Equal(sql.ErrNoRows), "expect the image to not exist in db, but got %v", i)
	})

	It("responds with progress data and image data on success", func() {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()

		ctx := newPullTestCtx(fs, db)
		res := ctx.do(svr, "foo", &url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		})

		j := struct {
			Image serializers.Image `json:"image"`
		}{}

		// includes progress data
		Expect(res.Body).To(ContainSubstring(`{"status":"Pulling..."}`), "failed to include progress data")

		// keep parsing and ignore the progress data to get the final image
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			json.Unmarshal(scanner.Bytes(), &j)
			if j.Image.ID > 0 {
				break
			}
		}
		Expect(j.Image.FreighterID).To(Equal("123asd"), "missing final image in response")
	})

	It("publishes an image.pulling event with the pulled image", func(done Done) {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()

		ctx := newPullTestCtx(fs, db)
		u, h := ctx.user, ctx.host
		ch := svr.Events.Subscribe(u.Topic())
		defer svr.Events.Unsubscribe(ch, u.Topic())

		ctx.do(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to find image after pull")

		// check if the first event we got is image.pulling event
		e := (<-ch).(*controllers.Event)
		data := e.Data.(*images.PullEvent)
		Expect(e.Name).To(Equal("image.pulling"), "expect image.pulling events but got another")
		Expect(data.ID).To(Equal(i.ID), "emitted image.pulling with invalid image")
		Expect(data.Progress).To(Equal(`{"status":"Pulling..."}`), "failed to include progress data")

		close(done)
	})

	It("publishes an image.pulled event with the pulled image", func(done Done) {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()

		ctx := newPullTestCtx(fs, db)
		u, h := ctx.user, ctx.host
		ch := svr.Events.Subscribe(u.Topic())
		defer svr.Events.Unsubscribe(ch, u.Topic())

		ctx.do(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to find image after pull")

		var e *controllers.Event
		for {
			e = (<-ch).(*controllers.Event)
			if e.Name == "image.pulled" {
				break
			}
		}
		img := e.Data.(*serializers.Image)
		Expect(img.ID).To(Equal(i.ID), "failed to serialize correct image")
		Expect(img.FreighterID).To(Equal("123asd"), "failed to serialize freighter response")
		close(done)
	})

	It("requires repository param", func() {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()

		ctx := newPullTestCtx(fs, db)
		res := ctx.do(svr, "foo", nil)

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("repository is required"))
	})

	It("returns 500 on freighterclient error and emits image.pull_failed event", func() {
		fs := newFreighterServer("scratch", "latest")
		defer fs.Close()
		fs.pullError = true

		ctx := newPullTestCtx(fs, db)
		ch := svr.Events.Subscribe(ctx.user.Topic())
		defer svr.Events.Unsubscribe(ch, ctx.user.Topic())

		res := ctx.do(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		errType, _ := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("internal_server_error"))

		ech := make(chan *controllers.Event, 1)

		go func() {
			var e *controllers.Event
			for {
				e = (<-ch).(*controllers.Event)
				if e.Name == "image.pull_failed" {
					ech <- e
					break
				}
			}
		}()

		select {
		case <-time.After(50 * time.Millisecond):
			Expect(true).To(BeFalse(), "timeout while getting image.pull_failed event")
		case e := <-ech:
			img := e.Data.(*serializers.Image)
			Expect(img.Slug).To(Equal("scratch"), "failed to serialize freighter response")
		}
	})

	Context("when image already exists", func() {
		It("updates the existing image record with the new info from Freighter", func() {
			fs := newFreighterServer("scratch", "latest")
			defer fs.Close()

			ctx := newPullTestCtx(fs, db)
			u, h := ctx.user, ctx.host
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

			ctx.do(svr, "foo", &url.Values{"repository": []string{"scratch"}})

			i, err = image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find stored image after pull")
			Expect(i.FreighterID).To(Equal("123asd"))
		})

		It("does not removes the image when freighter failed to pull image", func() {
			fs := newFreighterServer("scratch", "latest")
			defer fs.Close()
			fs.pullError = true

			ctx := newPullTestCtx(fs, db)
			u, h := ctx.user, ctx.host
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

			ctx.do(svr, "foo", &url.Values{"repository": []string{"scratch"}})

			_, err = image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "expect the old image to not be deleted")
		})
	})

	Context("when we want to pull asynchronously", func() {
		It("returns 202 Accepted with the image stored in db", func() {
			fs := newFreighterServer("scratch", "latest")
			defer fs.Close()

			ctx := newPullTestCtx(fs, db)

			params := &url.Values{}
			params.Set("async", "true")
			params.Set("repository", "scratch")
			res := ctx.do(svr, "foo", params)

			Expect(res.Code).To(Equal(http.StatusAccepted))

			j := struct {
				Image serializers.Image `json:"image"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			// These fields are not yet set.
			Expect(j.Image.FreighterID).To(BeEmpty())
			Expect(j.Image.BuiltAt).To(BeNil())
		})
	})
})

type pullTestCtx struct {
	user  *user.User
	token *token.Token
	host  *host.Host
}

func (c *pullTestCtx) do(svr *server.Server, host string, params *url.Values) *httptest.ResponseRecorder {
	req, err := test.NewAuthedFormRequest(c.token, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/pull", host), params)
	Expect(err).To(BeNil(), "failed to POST images/pull")
	res := httptest.NewRecorder()
	svr.Router().ServeHTTP(res, req)
	return res
}

func newPullTestCtx(fs *freighterServer, db *db.Database) *pullTestCtx {
	fsHost, fsPort := fs.address()

	var err error
	u := &user.User{
		Username: "foo",
		Password: "123456",
		Email:    "foo@example.com",
	}

	err = u.Insert(db)
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

	return &pullTestCtx{
		user:  u,
		token: t,
		host:  hs.H,
	}
}

// freighter test server
type freighterServer struct {
	*httptest.Server
	image string
	tag   string

	pullError bool
}

func (s *freighterServer) inspectImage(w http.ResponseWriter, r *http.Request) {
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

func (s *freighterServer) pullImage(w http.ResponseWriter, r *http.Request) {
	if s.pullError {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"error":"internal server error. dont know why"}`)
		return
	}
	stream := flusher.NewWriteFlusher(w)
	fmt.Fprintln(stream, `{"status":"Pulling..."}`)
	fmt.Fprintln(stream, `{"status":"Pulling","progress":"1 B/ 100 B"}`)
	fmt.Fprintln(stream, `{"status":"Pulling","progress":"2 B/ 100 B"}`)
}

func (s *freighterServer) address() (string, int64) {
	addr := s.Listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)
	portInt, _ := strconv.ParseInt(port, 10, 64)
	return host, portInt
}

func newFreighterServer(image, tag string) *freighterServer {
	s := &freighterServer{image: image, tag: tag}
	r := mux.NewRouter()
	r.HandleFunc("/images/pull", s.pullImage).Methods("POST")
	r.HandleFunc("/images/inspect", s.inspectImage).Methods("GET")
	s.Server = httptest.NewServer(r)
	return s
}
