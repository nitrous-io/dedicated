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

func mockOKServer(path string) *httptest.Server {
	return createServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal(path))
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
}

func mockErrorServer(path string) *httptest.Server {
	return createServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal(path))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("some_error"))
	})
}

var _ = Describe("hostbuilder", func() {
	Describe("Create()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.OutputType, error) {
			return hb.Create("digitalocean", &hostbuilder.ConfigType{
				"name":        "doublerainbow",
				"public_keys": []string{"value1", "value2"},
			})
		}

		Context("when hostbuilder creates a host", func() {
			BeforeEach(func() {
				mockHostbuilder := createServer(func(w http.ResponseWriter, r *http.Request) {
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

				hb = hostbuilder.NewHostbuilder(mockHostbuilder.URL)
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
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Delete()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.OutputType, error) {
			return hb.Delete("digitalocean", &hostbuilder.ConfigType{
				"id": "hostid",
			})
		}

		Context("when hostbuilder deletes the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/delete").URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/delete").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Start()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.OutputType, error) {
			return hb.Start("digitalocean", &hostbuilder.ConfigType{
				"id": "hostid",
			})
		}

		Context("when hostbuilder starts the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/start").URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/start").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Stop()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.OutputType, error) {
			return hb.Stop("digitalocean", &hostbuilder.ConfigType{
				"id": "hostid",
			})
		}

		Context("when hostbuilder stops the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/stop").URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/stop").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Restart()", func() {
		var hb *hostbuilder.Hostbuilder

		doRequest := func() (hostbuilder.OutputType, error) {
			return hb.Restart("digitalocean", &hostbuilder.ConfigType{
				"id": "hostid",
			})
		}

		Context("when hostbuilder restarts the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/restart").URL)
			})

			It("returns no errors", func() {
				_, err := doRequest()
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/restart").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := doRequest()
				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("OverrideRequestHandler()", func() {
		It("overrides the default request handler", func() {
			hb := hostbuilder.NewHostbuilder("www.exmple.com")

			ch := func(path string, provider string, config *hostbuilder.ConfigType) (hostbuilder.OutputType, error) {
				return nil, errors.New("this is a custom handler")
			}
			hb.OverrideRequestHandler(ch)

			o, err := hb.Create("provider", &hostbuilder.ConfigType{})
			Expect(o).To(BeNil())
			Expect(err).To(MatchError("this is a custom handler"))
		})
	})
})
