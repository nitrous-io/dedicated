package volumes_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateVolume", func() {
	var (
		db *db.Database

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u   *user.User
		t   *token.Token
		hs  *HostStack
		env *middleware.Env
		h   *host.Host
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

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
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
			U: u,
		})
		Expect(err).To(BeNil())
		h = hs.H

		env = &middleware.Env{
			DB:      db,
			Events:  pubsub.New(128),
			Tracker: analytics.NewMixpanelTracker(envvars.MixpanelToken),
			User:    u,
		}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		UnstubFreighterFn()
	})

	Context("successful create", func() {
		It("creates the volume after calling freighter", func() {
			_, err := volumes.CreateVolume(h, url.Values{"slug": []string{"metric"}, "name": []string{"some label"}}, env)
			Expect(mockClient.CreateVolumeCalled).To(Equal(1))
			Expect(err).To(BeNil())

			vols, err := volume.FindAll(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(vols).To(HaveLen(1))
			Expect(vols[0].Slug).To(Equal("metric"))
			Expect(vols[0].Name).To(Equal("some label"))
		})

		It("publishes a volume created event with the created volume", func(done Done) {
			ch := env.Events.Subscribe(u.Topic())
			defer env.Events.Unsubscribe(ch, u.Topic())

			_, err := volumes.CreateVolume(h, url.Values{"slug": []string{"metric"}, "name": []string{"some label"}}, env)
			Expect(err).To(BeNil())

			v, err := volume.Find(db, hs.H.ID, "metric")
			Expect(err).To(BeNil())
			Expect(<-ch).To(Equal(&controllers.Event{
				Name: controllers.EventVolumeCreated,
				Data: map[string]*serializers.Volume{
					"volume": serializers.SerializeVolume(v),
				},
			}))
			close(done)
		})

		It("converts the 'slug' form value to lowercase", func() {
			_, err := volumes.CreateVolume(h, url.Values{"slug": []string{"TitleCaseSlug"}, "name": []string{"some label"}}, env)
			Expect(err).To(BeNil())

			Expect(mockClient.CreateVolumeCalled).To(Equal(1))
			Expect(mockClient.CreateVolumeSlug).To(Equal("titlecaseslug"))

			vols, err := volume.FindAll(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(vols).To(HaveLen(1))
			Expect(vols[0].Slug).To(Equal("titlecaseslug"))
		})
	})

	Context("when slug already exists", func() {
		BeforeEach(func() {
			v := &volume.Volume{HostID: hs.H.ID, Slug: "taken"}
			err := v.Insert(db)
			Expect(err).To(BeNil())
			Expect(v.ID).NotTo(BeZero())
		})

		It("returns an error without calling freighterclient", func() {
			res, err := volumes.CreateVolume(h, url.Values{"slug": []string{"taken"}, "name": []string{"some label"}}, env)
			Expect(res).To(BeNil())

			Expect(mockClient.CreateVolumeCalled).To(Equal(0))
			Expect(err).To(MatchError(httperror.New(http.StatusBadRequest, "validation_failed", "volume already exists on this host")))
		})
	})

	Context("when Freighter client returns error", func() {
		BeforeEach(func() {
			mockClient.CreateVolumeError = fmt.Errorf("volume too loud")
		})

		It("responds with 500 and no volume is added to db", func() {
			res, err := volumes.CreateVolume(h, url.Values{"slug": []string{"slug"}, "name": []string{"some label"}}, env)
			Expect(res).To(BeNil())
			Expect(err).To(MatchError(mockClient.CreateVolumeError))

			Expect(mockClient.CreateVolumeCalled).To(Equal(1))
			vols, err := volume.FindAll(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(vols).To(HaveLen(0))
		})
	})

	Context("when host is unavailable", func() {
		BeforeEach(func() {
			hs.H.State = host.StateStopped
			err := hs.H.Update(db)
			Expect(err).To(BeNil())
			UnstubFreighterFn()
		})

		It("responds with 503 and no volume is added to db", func() {
			res, err := volumes.CreateVolume(h, url.Values{"slug": []string{"slug"}, "name": []string{"some label"}}, env)
			Expect(res).To(BeNil())
			Expect(err).To(Equal(httperror.New(http.StatusServiceUnavailable, "service_unavailable", "host is not running")))
		})
	})

})
