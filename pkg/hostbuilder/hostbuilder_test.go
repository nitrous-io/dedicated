package hostbuilder_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/nitrous-io/nitrous/pkg/hostbuilder"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/hostbuilder")
}

var _ = Describe("pkg/hostbuilder", func() {
	var (
		hb  *hostbuilder.Hostbuilder
		srv *ghttp.Server
	)

	prepareHandlers := func(method, path string, code int, body string) {
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyRequest(method, path),
			ghttp.RespondWith(code, body),
		))
	}

	BeforeEach(func() {
		srv = ghttp.NewServer()
	})

	AfterEach(func() {
		srv.Close()
	})

	Describe("Create()", func() {
		doRequest := func() (hostbuilder.Output, error) {
			return hb.Create("digitalocean", &hostbuilder.Config{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			})
		}

		Context("when hostbuilder creates a host", func() {
			BeforeEach(func() {
				m := struct {
					Provider       string `json:"provider"`
					ID             string `json:"id"`
					IPAddress      string `json:"ip_address"`
					FreighterdPort uint16 `json:"freighterd_port"`
				}{
					Provider:       "digitalocean",
					ID:             "some_id",
					IPAddress:      "0.0.0.0",
					FreighterdPort: 1234,
				}
				payload, err := json.Marshal(m)
				Expect(err).To(BeNil())
				prepareHandlers("POST", "/hosts", http.StatusCreated, string(payload))
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns the info of the created host", func() {
				info, err := doRequest()

				Expect(err).To(BeNil())
				Expect(info["provider"]).To(Equal("digitalocean"))
				Expect(info["id"]).To(Equal("some_id"))
				Expect(info["ip_address"]).To(Equal("0.0.0.0"))
				Expect(info["freighterd_port"]).To(Equal(float64(1234)))
			})
		})

		Context("when hostbuilder fails to create a host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts", http.StatusBadRequest, "some_error")
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Delete()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.Output, error) {
			return hb.Delete("digitalocean", &hostbuilder.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder deletes the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/delete", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/delete", http.StatusBadRequest, "some_error")
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Start()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.Output, error) {
			return hb.Start("digitalocean", &hostbuilder.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder starts the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/start", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/start", http.StatusBadRequest, "some_error")
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Stop()", func() {
		doRequest := func() (hostbuilder.Output, error) {
			return hb.Stop("digitalocean", &hostbuilder.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder stops the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/stop", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/stop", http.StatusBadRequest, "some_error")
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Restart()", func() {
		doRequest := func() (hostbuilder.Output, error) {
			return hb.Restart("digitalocean", &hostbuilder.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder restarts the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/restart", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/restart", http.StatusBadRequest, "some_error")
				hb = hostbuilder.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("OverrideRequestHandler()", func() {
		It("overrides the default request handler", func() {
			hb := hostbuilder.NewHostbuilder("www.example.com")

			ch := func(path string, provider string, config *hostbuilder.Config) (hostbuilder.Output, error) {
				return nil, errors.New("this is a custom handler")
			}
			hb.OverrideRequestHandler(ch)

			o, err := hb.Create("provider", &hostbuilder.Config{})
			Expect(o).To(BeNil())
			Expect(err).To(MatchError("this is a custom handler"))
		})
	})
})
