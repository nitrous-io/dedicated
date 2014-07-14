package hostbuilderclient_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/hostbuilderclient")
}

var _ = Describe("pkg/hostbuilderclient", func() {
	var (
		hb  *hostbuilderclient.Hostbuilder
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
		doRequest := func() (*hostbuilder.Response, error) {
			return hb.Create("digitalocean", &hostbuilderclient.Config{
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
					FreighterdPort: 65333,
				}
				payload, err := json.Marshal(m)
				Expect(err).To(BeNil())
				prepareHandlers("POST", "/hosts", http.StatusCreated, string(payload))
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns the info of the created host", func() {
				info, err := doRequest()

				Expect(err).To(BeNil())
				Expect(info.Provider).To(Equal("digitalocean"))
				Expect(info.ID).To(Equal("some_id"))
				Expect(info.IPAddress).To(Equal("0.0.0.0"))
				Expect(info.FreighterdPort).To(Equal(uint16(65333)))
			})
		})

		Context("when hostbuilder fails to create a host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts", http.StatusBadRequest, "some_error")
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Delete()", func() {
		var hb *hostbuilderclient.Hostbuilder

		doRequest := func() (*hostbuilder.Response, error) {
			return hb.Delete("digitalocean", &hostbuilderclient.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder deletes the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/delete", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/delete", http.StatusBadRequest, "some_error")
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Start()", func() {
		var hb *hostbuilderclient.Hostbuilder

		doRequest := func() (*hostbuilder.Response, error) {
			return hb.Start("digitalocean", &hostbuilderclient.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder starts the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/start", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/start", http.StatusBadRequest, "some_error")
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Stop()", func() {
		doRequest := func() (*hostbuilder.Response, error) {
			return hb.Stop("digitalocean", &hostbuilderclient.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder stops the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/stop", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/stop", http.StatusBadRequest, "some_error")
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Restart()", func() {
		doRequest := func() (*hostbuilder.Response, error) {
			return hb.Restart("digitalocean", &hostbuilderclient.Config{
				"id": "hostid",
			})
		}

		Context("when hostbuilder restarts the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/restart", http.StatusOK, `{"status":"ok"}`)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				prepareHandlers("POST", "/hosts/restart", http.StatusBadRequest, "some_error")
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("OverrideRequestHandler()", func() {
		It("overrides the default request handler", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")

			ch := func(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
				return nil, errors.New("this is a custom handler")
			}
			hb.OverrideRequestHandler(ch)

			o, err := hb.Create("provider", &hostbuilderclient.Config{})
			Expect(o).To(BeNil())
			Expect(err).To(MatchError("this is a custom handler"))
		})
	})
})
