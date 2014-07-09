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

var _ = Describe("hostbuilder", func() {
	Describe("Create()", func() {
		var mockHostbuilder *httptest.Server
		var hb *hostbuilder.Hostbuilder

		Context("when hostbuilder creates a host", func() {
			BeforeEach(func() {
				mockHostbuilder = createServer(func(w http.ResponseWriter, r *http.Request) {
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
				mockHostbuilder = createServer(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("some_error"))
				})

				hb = hostbuilder.NewHostbuilder(mockHostbuilder.URL)
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
