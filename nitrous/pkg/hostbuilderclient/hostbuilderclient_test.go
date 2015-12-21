package hostbuilderclient_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/nitrous-io/dedicated/hostbuilder"
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

	prepareHandlers := func(method, path string, code int, body string, headers http.Header) {
		if headers == nil {
			headers = http.Header{}
		}
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyRequest(method, path),
			ghttp.RespondWith(code, body),
			ghttp.VerifyHeader(headers),
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
			header := http.Header{}
			header.Set("NITROUS", "IS_GOOD")
			prepareHandlers("POST", "/hosts", http.StatusOK, response, header)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			out := bytes.NewBuffer(nil)
			info, err := hb.Create("digitalocean", hostbuilderclient.Config{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			}, out, header, nil)

			Expect(out.String()).To(Equal(response), "response not the same as the hostbuilder stream")
			Expect(err).To(BeNil(), "unexpected error for POST /hosts")
			Expect(info).To(Equal(&hostbuilder.Response{
				Provider:       "digitalocean",
				ID:             "some_id",
				IPAddress:      "0.0.0.0",
				FreighterdPort: uint16(65333),
			}))
		})

		It("returns error when the build failed, including the original response", func() {
			response := `
			{"stream":"starting: create ssh key"}
			{"stream":"finished: create ssh key"}
			{"stream":"starting: create droplet"}
			{"stream":"failed: create droplet"}
			{"error":"failed to create droplet because of some reason", "provider": "digitalocean"}
			`
			header := http.Header{}
			header.Set("NITROUS", "IS_FAST")
			prepareHandlers("POST", "/hosts", http.StatusOK, response, header)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			info, err := hb.Create("digitalocean", hostbuilderclient.Config{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			}, ioutil.Discard, header, nil)
			Expect(err).ToNot(BeNil(), "failed to return error when the build failed")
			Expect(info).To(Equal(&hostbuilder.Response{
				Provider: "digitalocean",
				Error:    "failed to create droplet because of some reason",
			}))
		})
	})

	Describe("Update()", func() {
		It("streams back update output and returns the final host info", func() {
			response := `
			{"stream":"starting: renaming host"}
			{"stream":"finished: renaming host"}
			{"provider":"digitalocean","id":"some_id","ip_address":"0.0.0.0","freighterd_port":65333}
			`
			header := http.Header{}
			header.Set("NITROUS", "IS_PRETTY")
			prepareHandlers("POST", "/hosts/update", http.StatusOK, response, header)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			out := bytes.NewBuffer(nil)
			info, err := hb.Update("digitalocean", hostbuilderclient.Config{
				"name": "doublerainbow",
			}, out, header, nil)

			Expect(out.String()).To(Equal(response), "response not the same as the hostbuilder stream")
			Expect(err).To(BeNil())
			Expect(info).To(Equal(&hostbuilder.Response{
				Provider:       "digitalocean",
				ID:             "some_id",
				IPAddress:      "0.0.0.0",
				FreighterdPort: uint16(65333),
			}))
		})

		It("returns error when the update fails, including the original response", func() {
			response := `
			{"stream":"starting: renaming host"}
			{"stream":"failed: renaming host"}
			{"stream":"failed: renaming host"}
			{"error":"failed to rename host because reasons", "id": "i-232x"}
			`
			header := http.Header{}
			header.Set("NITROUS", "IS_FAST")
			prepareHandlers("POST", "/hosts/update", http.StatusOK, response, header)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			info, err := hb.Update("digitalocean", hostbuilderclient.Config{
				"name": "doublerainbow",
			}, ioutil.Discard, header, nil)
			Expect(err).ToNot(BeNil())
			Expect(info).To(Equal(&hostbuilder.Response{
				ID:    "i-232x",
				Error: "failed to rename host because reasons",
			}))
		})
	})

	Describe("Delete()", func() {
		var hb *hostbuilderclient.Hostbuilder

		doRequest := func() (*hostbuilder.Response, error) {
			header := http.Header{}
			header.Set("NITROUS", "IS_CLOUD")
			return hb.Delete("digitalocean", hostbuilderclient.Config{
				"id": "hostid",
			}, header, nil)
		}

		Context("when hostbuilder deletes the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_CLOUD")
				prepareHandlers("POST", "/hosts/delete", http.StatusOK, `{"status":"ok"}`, header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_CLOUD")
				prepareHandlers("POST", "/hosts/delete", http.StatusBadRequest, "some_error", http.Header{"NITROUS": []string{"IS_CLOUD"}})
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
			header := http.Header{}
			header.Set("NITROUS", "IS_IDE")
			return hb.Start("digitalocean", hostbuilderclient.Config{
				"id": "hostid",
			}, header, nil)
		}

		Context("when hostbuilder starts the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_IDE")
				prepareHandlers("POST", "/hosts/start", http.StatusOK, `{"status":"ok"}`, header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_IDE")
				prepareHandlers("POST", "/hosts/start", http.StatusBadRequest, "some_error", header)
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
			header := http.Header{}
			header.Set("NITROUS", "IS_SAAS")
			return hb.Stop("digitalocean", hostbuilderclient.Config{
				"id": "hostid",
			}, header, nil)
		}

		Context("when hostbuilder stops the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_SAAS")
				prepareHandlers("POST", "/hosts/stop", http.StatusOK, `{"status":"ok"}`, header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_SAAS")
				prepareHandlers("POST", "/hosts/stop", http.StatusBadRequest, "some_error", header)
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
			header := http.Header{}
			header.Set("NITROUS", "IS_AJAX")
			return hb.Restart("digitalocean", hostbuilderclient.Config{
				"id": "hostid",
			}, header, nil)
		}

		Context("when hostbuilder restarts the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_AJAX")
				prepareHandlers("POST", "/hosts/restart", http.StatusOK, `{"status":"ok"}`, header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_AJAX")
				prepareHandlers("POST", "/hosts/restart", http.StatusBadRequest, "some_error", header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err.Error()).To(ContainSubstring("some_error"))
			})
		})
	})

	Describe("SoftRestart()", func() {
		doRequest := func() (*hostbuilder.Response, error) {
			header := http.Header{}
			header.Set("NITROUS", "IS_AJAX")
			return hb.SoftRestart("digitalocean", hostbuilderclient.Config{
				"id": "hostid",
			}, header, nil)
		}

		Context("when hostbuilder restarts the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_AJAX")
				prepareHandlers("POST", "/hosts/soft_restart", http.StatusOK, `{"status":"ok"}`, header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				header := http.Header{}
				header.Set("NITROUS", "IS_AJAX")
				prepareHandlers("POST", "/hosts/soft_restart", http.StatusBadRequest, "some_error", header)
				hb = hostbuilderclient.NewHostbuilder(srv.URL())
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err.Error()).To(ContainSubstring("some_error"))
			})
		})
	})

	Describe("List()", func() {
		It("returns a slice of hosts", func() {
			prepareHandlers("GET", "/hosts", http.StatusOK, `{
				"hosts": [
					{
						"provider": "digitalocean",
						"id": "abcd",
						"ip_address": "1.2.3.4",
						"freighterd_port": 65333,
						"state": "Running"
					},
					{
						"provider": "digitalocean",
						"id": "bcde",
						"ip_address": "1.2.3.5",
						"freighterd_port": 65333,
						"state": "Running"
					}
				]
			}`, nil)
			hb = hostbuilderclient.NewHostbuilder(srv.URL())

			hosts, err := hb.List("digitalocean")
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(2))

			h1, h2 := hosts[0], hosts[1]
			Expect(h1.ID).To(Equal("abcd"))
			Expect(h1.IPAddress).To(Equal("1.2.3.4"))
			Expect(h1.FreighterdPort).To(Equal(uint16(65333)))
			Expect(h1.State).To(Equal("Running"))

			Expect(h2.ID).To(Equal("bcde"))
			Expect(h2.IPAddress).To(Equal("1.2.3.5"))
			Expect(h2.FreighterdPort).To(Equal(uint16(65333)))
			Expect(h2.State).To(Equal("Running"))
		})
	})
})
