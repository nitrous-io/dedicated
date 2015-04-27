package volumes_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/volumes")
}

var _ = Describe("controllers/volumes", func() {
	var (
		db              *db.Database
		svr             *server.Server
		res             *httptest.ResponseRecorder
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
		svr = server.NewServer("", db, logger, nil)
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

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
				Slug:      "myhost",
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

	makeVolumes := func(slugs ...string) {
		for _, s := range slugs {
			v := volume.Volume{HostID: hs.H.ID, Slug: s}
			err := v.Insert(db)
			Expect(err).To(BeNil())
		}
	}

	Describe("GET /api/v0/hosts/:slug/volumes", func() {
		doRequest := func() {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%s/volumes", hs.H.Slug), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		type vols struct {
			Volumes []struct {
				Slug string `json:"slug"`
			} `json:"volumes"`
		}

		var j *vols

		BeforeEach(func() {
			j = &vols{}
		})

		It("should allow the user even if it has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		Context("host has volumes", func() {
			BeforeEach(func() {
				makeVolumes("shared", "music", "db")
			})

			It("returns volumes in json", func() {
				doRequest()
				Expect(mockClient.ListVolumesCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusOK))

				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Volumes).To(HaveLen(3))
				slugs := []string{}
				for _, v := range j.Volumes {
					slugs = append(slugs, v.Slug)
				}
				// TODO on Gomega upgrade, can use ConsistOf([]string{"db", "music", shared"})
				Expect(slugs).To(ContainElement("db"))
				Expect(slugs).To(ContainElement("music"))
				Expect(slugs).To(ContainElement("shared"))
			})
		})

		Context("host has no volumes", func() {
			It("returns json", func() {
				doRequest()
				Expect(mockClient.ListVolumesCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusOK))

				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Volumes).To(HaveLen(0))
			})
		})

		Context("error encountered", func() {
			BeforeEach(func() {
				db.Close() // simulate error
			})

			It("returns error", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				slug, _ := ErrorSlug(res.Body)
				Expect(slug).To(ContainSubstring("internal_server_error"))
			})
		})
	})

	Describe("POST /api/v0/hosts/:slug/volumes", func() {
		var (
			previousCreateVolumeFn func(*host.Host, url.Values, *middleware.Env) (*serializers.Volume, error)
			createVolumeResult     *serializers.Volume
			createVolumeError      error

			createVolumeOptions url.Values
		)

		doRequest := func(vol string) {
			params := &url.Values{"slug": []string{vol}, "name": []string{"some label"}}
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%s/volumes", hs.H.Slug), params)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			previousCreateVolumeFn = volumes.CreateVolumeFn
			volumes.CreateVolumeFn = func(h *host.Host, options url.Values, env *middleware.Env) (*serializers.Volume, error) {
				createVolumeOptions = options

				return createVolumeResult, createVolumeError
			}
		})

		AfterEach(func() {
			createVolumeOptions = url.Values{}
			createVolumeError = nil
			createVolumeResult = nil
			volumes.CreateVolumeFn = previousCreateVolumeFn
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("sluge2")
			VerifyDelinquentUserResponse(res)
		})

		It("send down the options", func() {
			createVolumeResult = &serializers.Volume{
				Name: "v1",
			}
			doRequest("sluge2")
			Expect(createVolumeOptions.Get("slug")).To(Equal("sluge2"))
		})

		It("should return an error", func() {
			createVolumeError = errors.New("FAT32 is unavailable")
			doRequest("sluge2")
			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("DELETE /api/v0/hosts/:slug/volumes/:vslug", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedRequest(t, "DELETE", fmt.Sprintf("/api/v0/hosts/%s/volumes/%s", hs.H.Slug, vol), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			makeVolumes("deprecated", "conditioner")
			v, err := volume.Find(db, hs.H.ID, "deprecated")
			Expect(err).To(BeNil())
			Expect(v).NotTo(BeNil())
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("deprecated")
			VerifyDelinquentUserResponse(res)
		})

		Context("successful destroy", func() {
			It("removes volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Volume *serializers.Volume
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				v, err := volume.Find(db, hs.H.ID, "deprecated")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(v).To(BeNil())

				v, err = volume.Find(db, hs.H.ID, "conditioner")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(v.Slug).To(Equal("conditioner"))
			})

			It("publishes a volume destroyed event with the destroyed volume", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest("deprecated")

				msg := <-ch
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventVolumeDestroyed))

				evtData, ok := evt.Data.(map[string]*serializers.Volume)
				Expect(ok).To(BeTrue())
				Expect(evtData["volume"].Slug).To(Equal("deprecated"))
				close(done)
			})
		})

		Context("when volume is a system volume", func() {
			BeforeEach(func() {
				v := volume.Volume{HostID: hs.H.ID, Slug: "shared", System: true}
				err := v.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns 403 without destroying volume", func() {
				doRequest("shared")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when volume is not found", func() {
			It("returns 404 without calling freighterclient", func() {
				doRequest("nosuch")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when Freighter client returns error", func() {
			BeforeEach(func() {
				mockClient.RemoveVolumeError = fmt.Errorf("volume too loud")
			})

			It("returns 500 without destroying volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				slug, _ := ErrorSlug(res.Body)
				Expect(slug).To(Equal("internal_server_error"))
				v, err := volume.Find(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(v.Slug).To(Equal("deprecated"))
			})
		})

		Context("when volumes is being used by a container", func() {
			BeforeEach(func() {
				v, err := volume.Find(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				c := &container.Container{
					UserID: u.ID,
					HostID: hs.H.ID,
					Slug:   "go-123456",
					Name:   sql.NullString{"go", true},
					Image:  "go-image",
				}
				err = c.Insert(db)
				Expect(err).To(BeNil())
				cv := cvolume.ContainerVolume{ContainerID: c.ID, VolumeID: v.ID}
				err = cv.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns 403 without destroying volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(0))
				v, err := volume.Find(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})
		})
	})

	Describe("GET /api/v0/hosts/:slug/volumes/:vslug", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%s/volumes/%s", hs.H.Slug, vol), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("music")
			VerifyDelinquentUserResponse(res)
		})

		Context("when volume exist", func() {
			It("returns volume in JSON", func() {
				v := volume.Volume{HostID: hs.H.ID, Slug: "music", Name: "my music volume"}
				err := v.Insert(db)
				Expect(err).To(BeNil())

				doRequest("music")
				var j struct {
					Volume *serializers.Volume `json:"volume"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Volume.ID).To(Equal(v.ID))
				Expect(j.Volume.Name).To(Equal("my music volume"))
				Expect(j.Volume.Slug).To(Equal("music"))
				Expect(j.Volume.HostID).To(Equal(hs.H.ID))
				Expect(j.Volume.CreatedAt.IsZero()).NotTo(BeTrue())
				Expect(j.Volume.UpdatedAt.IsZero()).NotTo(BeTrue())
			})
		})

		Context("when volume is not found", func() {
			It("returns 404 Not Found", func() {
				doRequest("nosuch")
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})
	})

	Describe("GET /api/v0/hosts/:slug/volumes/:vslug/info", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%s/volumes/%s/info", hs.H.Slug, vol), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			makeVolumes("music")
			v, err := volume.Find(db, hs.H.ID, "music")
			Expect(err).To(BeNil())
			Expect(v).NotTo(BeNil())
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, u)
			doRequest("music")
			VerifyDelinquentUserResponse(res)
		})

		Context("successful get", func() {
			BeforeEach(func() {
				mockClient.VolumeInfoReturn = &freighterclient.Volume{
					Size: 666,
				}
			})

			It("returns info", func() {
				doRequest("music")
				Expect(mockClient.VolumeInfoCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Vol struct {
						Size uint64 `json:"size"`
					} `json:"info"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Vol.Size).To(Equal(uint64(666)))
			})
		})

		Context("when volume is not found", func() {
			It("returns 404 without calling freighterclient", func() {
				doRequest("nosuch")
				Expect(mockClient.VolumeInfoCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when Freighter client returns error", func() {
			BeforeEach(func() {
				mockClient.VolumeInfoError = fmt.Errorf("volume is deafening")
			})

			It("returns 500", func() {
				doRequest("music")
				Expect(mockClient.VolumeInfoCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				slug, _ := ErrorSlug(res.Body)
				Expect(slug).To(Equal("internal_server_error"))
			})
		})
	})
})
