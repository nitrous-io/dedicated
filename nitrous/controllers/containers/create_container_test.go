package containers_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
	"github.com/nitrous-io/nitrous-common/sshkey"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateContainer", func() {
	var (
		db *ndb.Database

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u      *user.User
		t      *token.Token
		hs     *test.HostStack
		params url.Values
		env    *middleware.Env
		h      *host.Host
	)

	mkParams := func() url.Values {
		params := url.Values{}
		params.Set("name", "elixir-web")
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
		test.StubDefaultFreighterFn(mockClient)

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

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
		err = t.Insert(db)
		Expect(err).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
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
			Ports: map[string][]string{
				"22":   []string{"65022"},
				"3000": []string{"63000"},
			},
			Env: []string{
				"NITROUS_PREVIEW_PORT=3000",
			},
		}

		_, err = db.Exec(`ALTER SEQUENCE containers_slug_seq RESTART WITH 10000;`)
		Expect(err).To(BeNil())
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
		Expect(cn.Image).To(Equal("elixir"))
		Expect(cn.Slug).To(HavePrefix("elixir-web"))
		Expect(slugbuilder.IsValid(cn.Slug)).To(BeTrue())
		Expect(cn.IPAddress).To(Equal(""))
		Expect(cn.GatewayIP).To(Equal(""))
		Expect(cn.State).To(Equal(container.StateRunning.String()))
		Expect(cn.Name).To(Equal("elixir-web"))
	}

	It("calls the Freighter client to create a container", func() {
		mockClient.CreateError = errors.New("create failed")

		_, err := containers.CreateContainer(h, params, env)
		Expect(err).To(MatchError(mockClient.CreateError))

		Expect(mockClient.CreateCalled).To(Equal(1))
		Expect(mockClient.ReceivedName).To(HavePrefix("elixir-web"))
		Expect(slugbuilder.IsValid(mockClient.ReceivedName)).To(BeTrue())
		Expect(mockClient.ReceivedImage).To(Equal("elixir"))
		conf := mockClient.ReceivedConfig
		Expect(conf.Env).To(ConsistOf("LANG=C", "NODE=nope"))
		Expect(conf.PortBindings).To(ConsistOf("3000:8080", "0.0.0.0::4552", "0.0.0.0:8081:11211"))
	})

	It("should not allow a user to create multiple containers at the same time", func() {
		tx, _ := db.Beginx()
		ndb.TransactionLock(tx, ndb.CONTAINER_CREATE_LOCK, int(u.ID))
		defer tx.Rollback()

		params.Set("slug", "container-unique")
		res, err := containers.CreateContainer(h, params, env)
		Expect(res).To(BeNil())
		Expect(err).To(Equal(httperror.New(http.StatusConflict, "conflict", "another container creation request is currently being processed")))
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

		Expect(err).To(MatchError(httperror.New(http.StatusBadRequest, "max_containers_reached", "workspace has exceeded the plan's maximum containers")))
	})

	It("creates a new container record", func() {
		_, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		cntrs, err := container.FindAllByHostID(db, h.ID)
		Expect(err).To(BeNil())
		Expect(cntrs).To(HaveLen(1))
		c := cntrs[0]
		Expect(c.Slug).To(HavePrefix("elixir-web"))
		Expect(slugbuilder.IsValid(c.Slug)).To(BeTrue())
		Expect(c.Image).To(Equal("elixir"))
		Expect(c.Name.String).To(Equal("elixir-web"))

		var portMappings map[string]string
		err = json.Unmarshal([]byte(c.PortMappings), &portMappings)
		Expect(err).To(BeNil())
		Expect(portMappings["preview"]).To(Equal("63000"))
		Expect(portMappings["22"]).To(Equal("65022"))

		Expect(mockClient.ReceivedSecret).To(Equal(c.Secret))
	})

	It("responds with the created container", func() {
		res, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		cntrs, err := container.FindAllByHostID(db, h.ID)
		Expect(err).To(BeNil())
		Expect(cntrs).To(HaveLen(1))
		c := cntrs[0]

		expectSerializedContainer(res.Container, c)
	})

	It("updates the user to set first_time as FALSE", func() {
		_, err := db.Exec("UPDATE users SET first_time = TRUE WHERE id = $1", env.User.ID)
		Expect(err).To(BeNil())

		_, err = containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		u, err := user.FindByID(db, env.User.ID)
		Expect(u.FirstTime).To(BeFalse())
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
		evtData, ok := evt.Data.(map[string]interface{})
		Expect(ok).To(BeTrue())
		cn, ok := evtData["container"].(*serializers.Container)
		Expect(ok).To(BeTrue())
		cntrs, err := container.FindAllByHostID(db, h.ID)
		Expect(err).To(BeNil())
		Expect(cntrs).To(HaveLen(1))
		c := cntrs[0]

		expectSerializedContainer(cn, c)

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
				"env": ["SHELL=powershell"]
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
		Expect(mockClient.ReceivedConfig.PortBindings).To(ConsistOf([]string{"3000:8080", "0.0.0.0::4552", "0.0.0.0:8081:11211", "public::2341", "public::8931"}))
		Expect(mockClient.ReceivedConfig.Env).To(ConsistOf([]string{"LANG=C", "NODE=nope", "SHELL=powershell"}))
		Expect(*mockClient.ReceivedConfig.PublishAll).To(BeFalse())
		Expect(*mockClient.ReceivedConfig.Privileged).To(BeTrue())

		cntrs, err := container.FindAllByHostID(db, h.ID)
		Expect(err).To(BeNil())
		Expect(cntrs).To(HaveLen(1))
		c := cntrs[0]

		expectSerializedContainer(res.Container, c)
	})

	It("should override template config with user-requested settings", func() {
		t1 := &template.Template{
			Slug:        "my-template",
			Name:        sql.NullString{"Ruby on Rails", true},
			Description: sql.NullString{"A Rails stack template", true},
			Image:       "rails:4.2",
			Config: `{
				"privileged": true,
				"publish_all": false
			}`,
			Type: template.TypeStack,
		}
		err := t1.Insert(db)
		Expect(err).To(BeNil())

		params.Set("template", "my-template")
		params.Set("privileged", "false")
		params.Set("publish_all", "true")
		_, err = containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		Expect(mockClient.CreateCalled).To(Equal(1))
		Expect(*mockClient.ReceivedConfig.PublishAll).To(BeTrue())
		Expect(*mockClient.ReceivedConfig.Privileged).To(BeFalse())
	})

	Context("when the Freighter client returns an error", func() {
		BeforeEach(func() {
			mockClient.CreateError = errors.New("only Chuck Norris is allowed")
		})

		It("does not create a new container record", func() {
			_, err := containers.CreateContainer(h, params, env)
			Expect(err).ToNot(BeNil())

			cntrs, err := container.FindAllByHostID(db, h.ID)
			Expect(err).To(BeNil())
			Expect(cntrs).To(HaveLen(0))
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

				cntrs, err := container.FindAllByHostID(db, h.ID)
				Expect(err).To(BeNil())
				Expect(cntrs).To(HaveLen(1))
				c := cntrs[0]

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

	Context("when name param is not given", func() {
		BeforeEach(func() {
			params.Del("name")
		})

		It("returns the error", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(res).To(BeNil())

			Expect(err).To(MatchError(containers.ErrNameRequired))
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

	Context("when assign pre-created volumes", func() {
		BeforeEach(func() {
			params.Add("volume[]", "music:/mpd/albums")
			params.Add("volume[]", "backups")
		})

		It("returns success if volume does exist and response doesnt include volumes", func() {
			v := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
			Expect(v.Insert(db)).To(BeNil())

			v2 := &volume.Volume{HostID: hs.H.ID, Slug: "backups"}
			Expect(v2.Insert(db)).To(BeNil())

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

		It("creates new volumes and associate old ones when container has a mix of built-in and manual volumes", func() {
			for _, slug := range []string{"music", "backups"} {
				Expect((&volume.Volume{HostID: hs.H.ID, Slug: slug}).Insert(db)).To(BeNil())
			}
			mockClient.CreatedContainer = &freighterclient.Container{
				ID:      "deadbeef",
				Name:    "elixir-web-1",
				State:   "running",
				Image:   "elixir",
				ImageID: "abc123",
				Volumes: map[string]string{
					"/var/lib/docker":  "hexvarlibdocker",
					"/var/lib/extra":   "hexvarlibextra",
					"/mpd/albums":      "music",
					"/volumes/backups": "backups",
				},
			}

			res, err := containers.CreateContainer(h, params, env)
			Expect(err).To(BeNil())

			vols, _ := cvolume.FindAll(db, &container.Container{ID: res.Container.ID})
			Expect(vols).To(HaveLen(4), "missing volumes")

			var slugs []string
			for _, v := range vols {
				slugs = append(slugs, v.Slug)
			}
			Expect(slugs).To(ConsistOf("music", "backups", "hexvarlibdocker", "hexvarlibextra"))
		})
	})

	It("creates new volumes when the container requires built-in volumes", func() {
		mockClient.CreatedContainer = &freighterclient.Container{
			ID:      "deadbeef",
			Name:    "elixir-web-1",
			State:   "running",
			Image:   "elixir",
			ImageID: "abc123",
			Volumes: map[string]string{
				"/var/lib/docker": "hexvarlibdocker",
				"/var/lib/extra":  "hexvarlibextra",
			},
		}

		res, err := containers.CreateContainer(h, params, env)
		Expect(err).To(BeNil())

		vols, _ := cvolume.FindAll(db, &container.Container{ID: res.Container.ID})
		Expect(vols).To(HaveLen(2), "missing generated volumes")
		var nameSlugs [][]string
		for _, v := range vols {
			Expect(v.HostID).To(Equal(res.Container.HostID))
			nameSlugs = append(nameSlugs, []string{v.Name, v.Slug})
		}

		Expect(nameSlugs).To(ConsistOf(
			[]string{fmt.Sprintf("%s:%s", res.Container.Slug, "/var/lib/docker"), "hexvarlibdocker"},
			[]string{fmt.Sprintf("%s:%s", res.Container.Slug, "/var/lib/extra"), "hexvarlibextra"},
		))
	})

	Context("when a user has connected to GitHub", func() {
		type addGitHubKeyFunc func(env *middleware.Env, h *host.Host, cn *container.Container, publicKey string) error

		var (
			key0                   string
			fn0                    string
			addGitHubKeyFuncCalled int
			origAddGitHubKeyFunc   addGitHubKeyFunc
		)

		BeforeEach(func() {
			up, err := userprofile.Find(db, u.ID)
			Expect(err).NotTo(HaveOccurred())
			up.GitHubUserName = sql.NullString{"github-user", true}
			up.GitHubOAuthTokenJson = sql.NullString{"{}", true}
			err = up.UpdateGithubInfo(db)
			Expect(err).NotTo(HaveOccurred())

			mockClient.CreatedContainer = &freighterclient.Container{
				ID:      "deadbeef",
				Name:    "elixir-web-1",
				State:   "running",
				Image:   "elixir",
				ImageID: "abc123",
			}

			key0 = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDATOQiODD9fe8IrqVuQKCN/8PyqCXOYrREqpmsi4Jx3m4VdwP0IwrmOylvkDYGG7HVOHKvlncwrIkDJKXlyplaqs6O5D+daY1JLuosqckGPzBlOFchdUjuqiDxdLPBEfOYr2ANZjfLjAyuGqsNAXwhn5+LHKXiXg7Le6feuEsZhP0JyAnEg9diLcmyv8f2rOPqM3L2SmZ38GbWJm1rO7+LVUmroNNa1X1UJHyQFyNOEtXHbYuoZ0xEui6UDJU/DAoVxSfTRzg9XKUWArTsKeGRmJoQBmRRtoFVe8kbs1qnJQCnHnkyH6BhoqFZ/wl7YNRvsGAta1Q1qiqLFSOYZvbT"
			fn0, err = sshkey.Md5Fingerprint(key0)
			mockClient.ListContainerKeysReturn = []*freighterclient.ContainerKey{
				&freighterclient.ContainerKey{Key: key0, Fingerprint: fn0},
			}

			addGitHubKeyFuncCalled = 0
			origAddGitHubKeyFunc = github.AddKeyToGitHub
			github.AddKeyToGitHub = func(env *middleware.Env, h *host.Host, cn *container.Container, publicKey string) error {
				addGitHubKeyFuncCalled++
				return nil
			}
		})

		AfterEach(func() {
			github.AddKeyToGitHub = origAddGitHubKeyFunc
		})

		It("fetches a container key and update the record", func() {
			res, err := containers.CreateContainer(h, params, env)
			Expect(err).NotTo(HaveOccurred())

			c, err := container.Find(db, u, res.Container.Slug)
			Expect(err).NotTo(HaveOccurred())

			Expect(c.PublicKeyFingerprint.String).To(Equal(fn0))
		})

		It("adds public key to github", func() {
			_, err := containers.CreateContainer(h, params, env)
			Expect(err).NotTo(HaveOccurred())

			Expect(addGitHubKeyFuncCalled).To(Equal(1))
		})
	})
})
