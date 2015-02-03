package containers_test

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateContainer", func() {
	var (
		db *db.Database

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u      *user.User
		t      *token.Token
		hs     *HostStack
		params url.Values
		env    *middleware.Env
		h      *host.Host
	)

	mkParams := func() url.Values {
		params := url.Values{}
		params.Set("slug", "elixir-web-1")
		params.Set("name", "Elixir Web Server 1")
		params.Set("image", "elixir")
		params.Add("env[]", "LANG=C")
		params.Add("env[]", "NODE=nope")
		params.Add("port[]", "3000:8080")
		params.Add("port[]", "0.0.0.0::4552")
		params.Add("port[]", "0.0.0.0:8081:11211")
		return params
	}

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}

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
				Slug:          "foo",
				IPAddress:     sql.NullString{"127.0.0.1", true},
				MaxContainers: sql.NullInt64{2, true},
			},
			U: u,
		})
		Expect(err).To(BeNil())
		h = hs.H

		params = mkParams()

		env = &middleware.Env{
			DB:      db,
			Events:  pubsub.New(128),
			Tracker: analytics.NewMixpanelTracker(envvars.MixpanelToken),
			Logger:  logger,
			User:    u,
		}

		mockClient.CreatedContainer = &freighterclient.Container{
			ID:      "deadbeef",
			Name:    "elixir-web-1",
			Host:    "",
			Gateway: "",
			State:   "running",
			Image:   "elixir",
			ImageID: "abc123",
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		UnstubFreighterFn()
	})

	expectSerializedContainer := func(cn *serializers.Container, c *container.Container) {
		Expect(cn.ID).To(Equal(c.ID))
		Expect(cn.CID).To(Equal("deadbeef"))
		Expect(cn.UserID).To(Equal(u.ID))
		Expect(cn.HostID).To(Equal(hs.H.ID))
		Expect(cn.Image).To(Equal("elixir"))
		Expect(cn.Slug).To(Equal("elixir-web-1"))
		Expect(cn.IPAddress).To(Equal(""))
		Expect(cn.GatewayIP).To(Equal(""))
		Expect(cn.State).To(Equal(container.StateRunning.String()))
		Expect(cn.Name).To(Equal("Elixir Web Server 1"))
	}

	It("calls the Freighter client to create a container", func() {
		mockClient.CreateError = errors.New("create failed")

		_, err := containers.CreateContainer(h, params, env)
		Expect(err).To(MatchError(mockClient.CreateError))

		Expect(mockClient.CreateCalled).To(Equal(1))
		Expect(mockClient.ReceivedName).To(Equal("elixir-web-1"))
		Expect(mockClient.ReceivedImage).To(Equal("elixir"))
		conf := mockClient.ReceivedConfig
		Expect(conf.Env).To(ConsistOf("LANG=C", "NODE=nope"))
		Expect(conf.PortBindings).To(ConsistOf("3000:8080", "0.0.0.0::4552", "0.0.0.0:8081:11211"))
	})

	PIt("should not allow a user to create multiple containers at the same time", func() {
		var wg sync.WaitGroup
		var createResults []*containers.CreateResult
		var errGroup []error

		var fireAt time.Time

		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				p := mkParams()
				p.Set("slug", fmt.Sprintf("con1-%d", i))
				<-start
				Eventually(func() bool {
					return time.Now().After(fireAt)
				}, "3s", "1ms").Should(BeTrue())
				cr, err := containers.CreateContainer(h, p, env)
				if err != nil {
					errGroup = append(errGroup, err)
				} else {
					createResults = append(createResults, cr)
				}
				wg.Done()
			}(i)
		}
		fireAt = time.Now().Add(2 * time.Second)
		close(start)
		wg.Wait()

		containers, err := container.FindAllByHostID(db, h.ID)
		Expect(err).To(BeNil())
		Expect(containers).To(HaveLen(1))

		Expect(createResults).To(HaveLen(1))
		Expect(errGroup).To(HaveLen(1))
		Expect(errGroup[0]).To(Equal(httperror.New(http.StatusBadRequest, "multiple_container_creation_disallowed", "only one container can be created at any time")))
	})

	It("should not allow for creating containers over the host's allowance", func() {
		c := &container.Container{
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err := c.Insert(db)
		Expect(err).To(BeNil())

		h.MaxContainers = sql.NullInt64{1, true}

		res, err := containers.CreateContainer(h, params, env)
		Expect(res).To(BeNil())

		Expect(err).To(MatchError(httperror.New(http.StatusBadRequest, "max_containers_reached", "host has exceeded the plan's maximum containers")))
	})

	It("creates a new container record", func() {
		_, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		c, err := container.Find(db, u, "elixir-web-1")
		Expect(err).To(BeNil())
		Expect(c.Slug).To(Equal("elixir-web-1"))
		Expect(c.Image).To(Equal("elixir"))
		Expect(c.Name.String).To(Equal("Elixir Web Server 1"))
		Expect(mockClient.ReceivedSecret).To(Equal(c.Secret))
	})

	It("converts the 'slug' form value to lowercase", func() {
		params.Del("slug")
		params.Set("slug", "Elixir-WEB-1")

		_, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		Expect(mockClient.CreateCalled).To(Equal(1))
		Expect(mockClient.ReceivedName).To(Equal("elixir-web-1"))

		c, err := container.Find(db, u, "elixir-web-1")
		Expect(err).To(BeNil())
		Expect(c.Slug).To(Equal("elixir-web-1"))
	})

	It("responds with the created container", func() {
		res, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		c, err := container.Find(db, u, "elixir-web-1")
		Expect(err).To(BeNil())
		expectSerializedContainer(res.Container, c)
	})

	It("publishes a container created event with the created container", func(done Done) {
		ch := env.Events.Subscribe(u.Topic())
		defer env.Events.Unsubscribe(ch, u.Topic())

		_, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		var evt *controllers.Event
		for msg := range ch { // Ignore all events except container created.
			e, ok := msg.(*controllers.Event)
			if ok && e.Name == controllers.EventContainerCreated {
				evt = e
				break
			}
		}
		Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
		evtData, ok := evt.Data.(map[string]*serializers.Container)
		Expect(ok).To(BeTrue())
		c, err := container.Find(db, u, "elixir-web-1")
		Expect(err).To(BeNil())
		expectSerializedContainer(evtData["container"], c)

		close(done)
	})

	It("should allow for creating a container from a template", func() {
		t1 := &template.Template{
			Slug:        "my-template",
			Name:        sql.NullString{"Ruby on Rails", true},
			Description: sql.NullString{"A Rails stack template", true},
			Image:       "rails:4.2",
			Config: `{
				"privileged": true,
				"publish_all": false,
				"ports": ["public::2341", "public::8931"],
				"env": ["SHELL=powershell"],
				"volumes": ["shared:/tmp/shared", "database:/var/lib/pg"]
			}`,
			Type: template.TypeStack,
		}
		err := t1.Insert(db)
		Expect(err).To(BeNil())

		params.Set("template", "my-template")
		res, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		Expect(mockClient.CreateCalled).To(Equal(1))
		Expect(mockClient.ReceivedImage).To(Equal("rails:4.2"))
		Expect(mockClient.ReceivedConfig.Image).To(Equal("rails:4.2"))
		Expect(mockClient.ReceivedConfig.PortBindings).To(Equal([]string{"3000:8080", "0.0.0.0::4552", "0.0.0.0:8081:11211", "public::2341", "public::8931"}))
		Expect(mockClient.ReceivedConfig.Env).To(Equal([]string{"LANG=C", "NODE=nope", "SHELL=powershell"}))
		Expect(*mockClient.ReceivedConfig.PublishAll).To(BeFalse())
		Expect(*mockClient.ReceivedConfig.Privileged).To(BeTrue())

		c, err := container.Find(db, u, "elixir-web-1")
		Expect(err).To(BeNil())
		expectSerializedContainer(res.Container, c)
	})

	Context("when the Freighter client returns an error", func() {
		BeforeEach(func() {
			mockClient.CreateError = errors.New("only Chuck Norris is allowed")
		})

		It("does not create a new container record", func() {
			_, err := containers.CreateContainer(h, params, env)
			Expect(err).ToNot(BeNil())

			c, err := container.Find(db, u, "elixir-web-1")
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
		})

		It("responds with the error", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(res).To(BeNil())
			Expect(err).To(MatchError(mockClient.CreateError))
		})

		Context("when the error is a StartFailedError", func() {
			BeforeEach(func() {
				mockClient.CreateError = &freighterclient.StartFailedError{"start failed: could not bind port 22"}
			})

			It("returns with the container and the error", func() {
				res, err := containers.CreateContainer(h, params, env)
				Expect(err).To(BeNil())

				Expect(res.ErrorMessage).To(Equal("start_failed"))
				Expect(res.ErrorDescription).To(ContainSubstring("start failed: could not bind port 22"))

				c, err := container.Find(db, u, "elixir-web-1")
				Expect(err).To(BeNil())
				expectSerializedContainer(res.Container, c)
			})
		})

		Context("when the error is an InvalidParamError", func() {
			BeforeEach(func() {
				mockClient.CreateError = &freighterclient.InvalidParamError{"invalid name"}
			})

			It("returns an error", func() {
				res, err := containers.CreateContainer(h, params, env)
				Expect(res).To(BeNil())

				Expect(err).To(MatchError(httperror.New(http.StatusBadRequest, "validation_failed", "invalid name")))
			})
		})

		Context("when the error is an ErrImageNotFound", func() {
			BeforeEach(func() {
				mockClient.CreateError = freighterclient.ErrImageNotFound
			})

			It("returns an error", func() {
				res, err := containers.CreateContainer(h, params, env)
				Expect(res).To(BeNil())

				Expect(err).To(MatchError(containers.ErrImageNotFound))
			})
		})

		Context("when the error is ErrMissingCmd", func() {
			BeforeEach(func() {
				mockClient.CreatedContainer = nil
				mockClient.CreateError = freighterclient.ErrMissingCmd
			})

			It("returns an error", func() {
				res, err := containers.CreateContainer(h, params, env)
				Expect(res).To(BeNil())

				Expect(err).To(MatchError(containers.ErrMissingCmd))
			})
		})
	})

	Context("when name is already taken", func() {
		BeforeEach(func() {
			c := &container.Container{
				CID:    "facecafe",
				Slug:   "elixir-web-1",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("returns the error", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(res).To(BeNil())

			Expect(err).To(MatchError(containers.Validations["index_containers_on_slug"]))
		})
	})

	Context("when slug param is not given", func() {
		BeforeEach(func() {
			params.Del("slug")
		})

		It("returns the error", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(res).To(BeNil())

			Expect(err).To(MatchError(containers.ErrSlugRequired))
		})
	})

	Context("when image param is not given", func() {
		BeforeEach(func() {
			params.Del("image")

			mockClient.CreatedContainer = &freighterclient.Container{
				ID:    "deadbeef",
				Name:  "freighter.elixir-web-1",
				Host:  "1.2.3.4",
				State: "running",
				Image: controllers.DefaultImage,
			}
		})

		It("uses the default image", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(err).To(BeNil())
			Expect(res.Container.Image).To(Equal(controllers.DefaultImage))
		})
	})

	Context("with volumes param", func() {
		BeforeEach(func() {
			params.Add("volume[]", "music:/mpd/albums")
			params.Add("volume[]", "backups")
		})

		It("returns success if volume does exist", func() {
			v := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
			err := v.Insert(db)
			Expect(err).To(BeNil())
			v2 := &volume.Volume{HostID: hs.H.ID, Slug: "backups"}
			err = v2.Insert(db)
			Expect(err).To(BeNil())

			res, err := containers.CreateContainer(h, params, env)
			Expect(err).To(BeNil())

			conf := mockClient.ReceivedConfig
			Expect(conf.Mounts).To(HaveLen(2))
			Expect(conf.Mounts).To(ContainElement("music:/mpd/albums"))
			Expect(conf.Mounts).To(ContainElement("backups"))
			cv, err := cvolume.Find(db, &container.Container{ID: res.Container.ID}, v)
			Expect(err).To(BeNil())
			Expect(cv.VolumeID).To(Equal(v.ID))
			cv2, err := cvolume.Find(db, &container.Container{ID: res.Container.ID}, v2)
			Expect(err).To(BeNil())
			Expect(cv2.VolumeID).To(Equal(v2.ID))
		})

		It("returns error if volume does not exist", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(res).To(BeNil())

			Expect(err).To(MatchError(containers.ErrVolumeNotFound))
		})

	})
})
