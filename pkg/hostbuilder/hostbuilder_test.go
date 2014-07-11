package hostbuilder_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nitrous-io/nitrous/pkg/hostbuilder"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostbuilder")
}

func createServer(fn func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(fn))
}

func mockOKServer(path, method string) *httptest.Server {
	return createServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal(path))
		Expect(r.Method).To(Equal(method))

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
}

func mockErrorServer(path, method string) *httptest.Server {
	return createServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal(path))
		Expect(r.Method).To(Equal(method))

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("some_error"))
	})
}

var _ = Describe("hostbuilder", func() {
	var hb *hostbuilder.Hostbuilder

	Describe("Create()", func() {
		doRequest := func() (hostbuilder.Output, error) {
			return hb.Create("digitalocean", &hostbuilder.Config{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			})
		}

		Context("when hostbuilder creates a host", func() {
			BeforeEach(func() {
				ms := createServer(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/hosts"))
					Expect(r.Method).To(Equal("POST"))

					r.ParseForm()

					provider := r.FormValue("provider")
					m := struct {
						Provider       string `json:"provider"`
						ID             string `json:"id"`
						IPAddress      string `json:"ip_address"`
						FreighterdPort uint16 `json:"freighterd_port"`
					}{
						Provider:       provider,
						ID:             "some_id",
						IPAddress:      "0.0.0.0",
						FreighterdPort: 1234,
					}

					payload, err := json.Marshal(m)
					if err != nil {
						panic(err) // should not fail
					}

					w.WriteHeader(http.StatusCreated)
					w.Header().Set("Content-Type", "application/json")
					w.Write(payload)
				})

				hb = hostbuilder.NewHostbuilder(ms.URL)
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
				ms := mockErrorServer("/hosts", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
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
				ms := mockOKServer("/hosts/delete", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				ms := mockErrorServer("/hosts/delete", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
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
				ms := mockOKServer("/hosts/start", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				ms := mockErrorServer("/hosts/start", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
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
				ms := mockOKServer("/hosts/stop", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				ms := mockErrorServer("/hosts/stop", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
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
				ms := mockOKServer("/hosts/restart", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				ms := mockErrorServer("/hosts/restart", "POST")
				hb = hostbuilder.NewHostbuilder(ms.URL)
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
