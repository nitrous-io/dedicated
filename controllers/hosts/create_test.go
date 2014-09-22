package hosts_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("POST /api/v0/hosts", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs
		params          *url.Values

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	BeforeEach(func() {
		var err error
		hs, err = CreateHostStack(db, &HostStack{
			R: &region.Region{Slug: "use"},
			P: &plan.Plan{Slug: "large"},
		})
		Expect(err).To(BeNil())

		params = &url.Values{}
		params.Set("slug", "honeybadger")
		params.Set("size", hs.P.Slug)
		params.Set("region", hs.R.Slug)
		params.Set("name", "honeybadger don't care")
	})

	type overrides map[string]string
	doRequest := func(p *url.Values) *httptest.ResponseRecorder {
		req, err := NewRequest("POST", "/api/v0/hosts", p)
		Expect(err).To(BeNil())
		req.Header.Set("Authorization", "Bearer valid-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// tests for required params
	for _, param := range []string{"size", "region", "slug"} {
		p := param

		Context("when request does not specify a "+p, func() {
			It("responds with "+p+" required", func() {
				params.Del(p)
				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal(p + " is required"))
			})
		})
	}

	Context("when a matching plan cannot be found for the specified size", func() {
		It("responds with plan not found", func() {
			params.Set("size", "doubledouble")
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("no matching plan found for the given size"))
		})
	})

	Context("when an invalid region is specified", func() {
		It("responds with region not found", func() {
			params.Set("region", "antarctica")
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("no matching region found for the given name"))
		})
	})

	Context("when no provider template found for the given plan and region", func() {
		It("responds with provider template not found error", func() {
			var r2 *region.Region

			r2 = &region.Region{Slug: "usw", Name: "US West"}
			err := r2.Insert(db)
			Expect(err).To(BeNil())

			params.Set("region", r2.Slug)
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("no matching templates found for the given plan and regions combination"))
		})
	})

	Context("with valid parameters", func() {
		Context("when hostbuilder successfully creates a host", func() {
			var config hostbuilderclient.Config

			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				ch := func(path string, provider string, conf *hostbuilderclient.Config) (*hostbuilder.Response, error) {
					config = *conf

					return &hostbuilder.Response{
						ID:             "abcd1234",
						IPAddress:      "0.0.0.0",
						Provider:       hs.PT.Name,
						FreighterdPort: uint16(65333),
					}, nil
				}
				hb.OverrideRequestHandler(ch)

				hosts.HostbuilderClient = hb
			})

			It("creates a host record for the user", func() {
				doRequest(params)
				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))

				h := hosts[0]
				Expect(h.RegionID).To(Equal(hs.R.ID))
				Expect(h.PlanID).To(Equal(hs.P.ID))
				Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

				Expect(h.Slug).To(Equal(params.Get("slug")))
				Expect(h.Name.String).To(Equal(params.Get("name")))

				Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
				Expect(h.State).To(Equal(host.StateRunning))
				Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
				Expect(h.Size.String).To(Equal(hs.P.Slug))
				Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
				Expect(h.FreighterdPort.Int64).To(Equal(int64(65333)))

				Expect(config["name"]).To(Equal(params.Get("slug")))
			})

			It("creates a keypair associated with that host", func() {
				doRequest(params)
				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))

				kp, err := keypairs.Get(db, hosts[0].ID)
				Expect(err).To(BeNil())
				Expect(kp.HostID).To(Equal(hosts[0].ID))

				Expect(config["public_keys"].([]string)[0]).To(Equal(kp.PublicKey))
			})

			It("creates a shared secret for the host", func() {
				doRequest(params)
				hosts, _ := host.FindAllByUser(db, u)

				Expect(hosts[0].Secret).NotTo(BeEmpty())
				Expect(config["secret"]).To(Equal(hosts[0].Secret))
			})

			It("creates a cert associated with that host", func() {
				doRequest(params)
				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))
				crt, err := cert.Find(db, hosts[0].ID)
				Expect(err).To(BeNil())
				Expect(crt.HostID).To(Equal(hosts[0].ID))
				Expect(crt.ServerCert).To(ContainSubstring("BEGIN CERTIFICATE"))
				Expect(crt.ServerKey).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
			})

			It("responds with the created host and status created", func() {
				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusCreated))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				hosts, _ := host.FindAllByUser(db, u)
				Expect(j.Host.ID).To(Equal(hosts[0].ID))
				Expect(j.Host.UserID).To(Equal(u.ID))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.Slug).To(Equal(params.Get("slug")))
				Expect(j.Host.IPAddress).To(Equal("0.0.0.0"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(hs.P.Slug))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hostname("honeybadger")))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host created event with the created host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(params)

				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))
				h := hosts[0]
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreated,
					Data: serializers.SerializeHost(&h),
				}))
				close(done)
			}, 3) // increase timeout to allow goroutine to finish

			It("converts the 'slug' form value to lowercase", func() {
				params.Set("slug", "HoneyBADGER")
				doRequest(params)

				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))
				Expect(hosts[0].Slug).To(Equal("honeybadger"))
			})
		})

		Context("when trying to create a host with an existing slug", func() {
			BeforeEach(func() {
				_, err := CreateHostStack(db, &HostStack{
					H: &host.Host{Slug: "honeybadger"},
					U: u,
				})
				Expect(err).To(BeNil())

				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))
			})

			It("does not create a host", func() {
				doRequest(params)
				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))
			})

			It("responds with HTTP 400 Bad Request", func() {
				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("validation_failed"))
				errDesc, _ := j["error_description"].(string)
				Expect(errDesc).To(Equal("slug is taken"))
			})
		})

		Context("when hostbuilder fails to create the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb
			})

			It("creates a host in the orphan state", func() {
				doRequest(params)
				h, err := host.Find(db, u, params.Get("slug"))
				Expect(err).To(BeNil())
				Expect(h.Slug).To(Equal(params.Get("slug")))
				Expect(h.State).To(Equal(host.StateOrphan))
			})

			It("responds with a internal server error", func() {
				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})

	Context("when async param is true", func() {
		BeforeEach(func() {
			params.Set("async", "true")
		})

		It("returns 202 accepted and creates host asynchronously", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Host.State).To(Equal(host.StateBorn))

			// These fields are not yet set.
			Expect(j.Host.IPAddress).To(BeEmpty())
			Expect(j.Host.FreighterdPort).To(BeZero())

			h, err := host.Find(db, u, params.Get("slug"))
			Expect(err).To(BeNil())
			Expect(h.Slug).To(Equal(params.Get("slug")))
			Expect(h.State).To(Equal(host.StateBorn))
		})

		Context("when hostbuilder successfully creates a host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				ch := func(path string, provider string, conf *hostbuilderclient.Config) (*hostbuilder.Response, error) {
					return &hostbuilder.Response{
						ID:             "abcd1234",
						IPAddress:      "0.0.0.0",
						Provider:       hs.PT.Name,
						FreighterdPort: uint16(65333),
					}, nil
				}
				hb.OverrideRequestHandler(ch)

				hosts.HostbuilderClient = hb
			})

			It("updates the host record with the info from Hostbuilder", func() {
				doRequest(params)

				Eventually(func() (string, error) {
					h, err := host.Find(db, u, params.Get("slug"))
					return h.State, err
				}).Should(Equal(host.StateRunning))

				Eventually(func() (string, error) {
					h, err := host.Find(db, u, params.Get("slug"))
					return h.ProviderIdentifier.String, err
				}).Should(Equal("abcd1234"))
			})

			It("publishes a host created event with the created host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(params)

				msg := <-ch
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventHostCreated))

				h, ok := evt.Data.(*serializers.Host)
				Expect(ok).To(BeTrue())

				hRec, err := host.Find(db, u, params.Get("slug"))
				Expect(err).To(BeNil())
				Expect(h).To(Equal(serializers.SerializeHost(hRec)))

				close(done)
			})
		})

		Context("when hostbuilder fails to create the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb
			})

			It("updates the host's state to orphan", func() {
				doRequest(params)

				Eventually(func() (string, error) {
					h, err := host.Find(db, u, params.Get("slug"))
					return h.State, err
				}).Should(Equal(host.StateOrphan))
			})

			It("publishes a host create failed event with the host record", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(params)

				msg := <-ch
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventHostCreateFailed))

				h, ok := evt.Data.(*serializers.Host)
				Expect(ok).To(BeTrue())

				hRec, err := host.Find(db, u, params.Get("slug"))
				Expect(err).To(BeNil())
				Expect(h).To(Equal(serializers.SerializeHost(hRec)))

				close(done)
			})
		})
	})

	Context("oauth token failure modes", func() {
		BeforeEach(func() {
			authTokenInputs.Path = "/api/v0/hosts"
			authTokenInputs.Method = "POST"
		})

		ItRequiresAuthToken(&authTokenInputs)
	})
})
