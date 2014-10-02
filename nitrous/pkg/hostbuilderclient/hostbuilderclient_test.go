package hostbuilderclient_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
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
		It("streams back build output and returns the final host info", func() {
			response := `
			{"stream":"starting: create ssh key"}
			{"stream":"finished: create ssh key"}
			{"stream":"starting: create droplet"}
			{"stream":"finished: create droplet"}
			{"provider":"digitalocean","id":"some_id","ip_address":"0.0.0.0","freighterd_port":65333}
			`
			prepareHandlers("POST", "/hosts", http.StatusOK, response)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			out := bytes.NewBuffer(nil)
			info, err := hb.Create("digitalocean", &hostbuilderclient.Config{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			}, out)

			Expect(out.String()).To(Equal(response), "response not the same as the hostbuilder stream")
			Expect(err).To(BeNil(), "unexpected error for POST /hosts")
			Expect(info).To(Equal(&hostbuilder.Response{
				Provider:       "digitalocean",
				ID:             "some_id",
				IPAddress:      "0.0.0.0",
				FreighterdPort: uint16(65333),
			}))
		})

		It("returns error when the build failed", func() {
			response := `
			{"stream":"starting: create ssh key"}
			{"stream":"finished: create ssh key"}
			{"stream":"starting: create droplet"}
			{"stream":"failed: create droplet"}
			{"error":"failed to create droplet because of some reason"}
			`
			prepareHandlers("POST", "/hosts", http.StatusOK, response)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			_, err := hb.Create("digitalocean", &hostbuilderclient.Config{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			}, ioutil.Discard)
			Expect(err).ToNot(BeNil(), "failed to return error when the build failed")
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
				Expect(err.Error()).To(ContainSubstring("some_error"))
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
				Expect(err.Error()).To(ContainSubstring("some_error"))
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
				Expect(err.Error()).To(ContainSubstring("some_error"))
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
				Expect(err.Error()).To(ContainSubstring("some_error"))
			})
		})
	})
})
