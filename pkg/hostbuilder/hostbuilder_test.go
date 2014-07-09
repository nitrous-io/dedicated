package hostbuilder_test

import (
	"database/sql"
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

		Context("when hostbuilder creates a host", func() {
			BeforeEach(func() {
				mockHostbuilder := createServer(func(w http.ResponseWriter, r *http.Request) {
					r.ParseForm()

					provider := r.FormValue("provider")
					m := struct {
						Provider  string `json:"provider"`
						ID        string `json:"id"`
						IPAddress string `json:"ip_address"`
					}{
						Provider:  provider,
						ID:        "some_id",
						IPAddress: "0.0.0.0",
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
				info, err := hb.Create("digitalocean", map[string]sql.NullString{
					"name":        sql.NullString{"doublerainbow", true},
					"public_keys": sql.NullString{"value1,value2", true},
				})

				Expect(err).To(BeNil())
				Expect(info["provider"]).To(Equal("digitalocean"))
				Expect(info["id"]).To(Equal("some_id"))
				Expect(info["ip_address"]).To(Equal("0.0.0.0"))
			})
		})

		Context("when hostbuilder fails to create a host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := hb.Create("digitalocean", map[string]sql.NullString{
					"name":        sql.NullString{"doublerainbow", true},
					"public_keys": sql.NullString{"value1,value2", true},
				})

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Delete()", func() {
		var hb *hostbuilder.Hostbuilder

		Context("when hostbuilder deletes the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/delete").URL)
			})

			It("returns no errors", func() {
				_, err := hb.Delete("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hostid", true},
				})
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/delete").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := hb.Delete("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hosid", true},
				})

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Start()", func() {
		var hb *hostbuilder.Hostbuilder

		Context("when hostbuilder starts the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/start").URL)
			})

			It("returns no errors", func() {
				_, err := hb.Start("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hostid", true},
				})
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/start").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := hb.Start("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hosid", true},
				})

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Stop()", func() {
		var hb *hostbuilder.Hostbuilder

		Context("when hostbuilder stops the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/stop").URL)
			})

			It("returns no errors", func() {
				_, err := hb.Stop("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hostid", true},
				})
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/stop").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := hb.Stop("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hosid", true},
				})

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("Restart()", func() {
		var hb *hostbuilder.Hostbuilder

		Context("when hostbuilder restarts the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockOKServer("/hosts/restart").URL)
			})

			It("returns no errors", func() {
				_, err := hb.Restart("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hostid", true},
				})
				Expect(err).To(BeNil())
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				hb = hostbuilder.NewHostbuilder(mockErrorServer("/hosts/restart").URL)
			})

			It("returns an error returned by hostbuilder", func() {
				_, err := hb.Restart("digitalocean", map[string]sql.NullString{
					"id": sql.NullString{"hosid", true},
				})

				Expect(err).To(MatchError("some_error"))
			})
		})
	})

	Describe("OverrideRequestHandler()", func() {
		It("overrides the default request handler", func() {
			hb := hostbuilder.NewHostbuilder("www.exmple.com")

			ch := func(path string, provider string, config *map[string]sql.NullString) (map[string]string, error) {
				return nil, errors.New("this is a custom handler")
			}
			hb.OverrideRequestHandler(ch)

			o, err := hb.Create("provider", map[string]sql.NullString{})
			Expect(o).To(BeNil())
			Expect(err).To(MatchError("this is a custom handler"))
		})
	})
})
