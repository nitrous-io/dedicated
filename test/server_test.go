package freightertest

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/server"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", func() {
	var (
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		err     error
		prov    *MockProvider
		cb      *MockCallback
	)

	BeforeEach(func() {
		prov = &MockProvider{}
		cb = &MockCallback{}
		handler = server.NewRouter(prov, cb)
		res = httptest.NewRecorder()
	})

	Describe("GET /", func() {
		BeforeEach(func() {
			req, err = NewRequest("GET", "/", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		})

		It("returns 200 ok", func() {
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(res.Body.String()).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("GET /containers", func() {
		doRequest := func() {
			req, err = NewRequest("GET", "/containers", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when there isn't any container", func() {
			BeforeEach(func() {
				doRequest()
			})

			It("returns 200 ok with empty container info array", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Body.String()).To(MatchJSON(`{"containers":[]}`))
			})
		})

		Context("when there exists containers", func() {
			BeforeEach(func() {
				prov.Containers = []provider.ContainerInfo{
					provider.ContainerInfo{Name: "foo", Host: "1.2.3.4", Port: 10000},
					provider.ContainerInfo{Name: "bar", Host: "1.2.3.5", Port: 10001},
				}
				doRequest()
			})

			It("returns 200 ok with container info array", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Body.String()).To(MatchJSON(`{"containers":[{"name":"foo","host":"1.2.3.4","port":10000}, {"name":"bar","host":"1.2.3.5","port":10001}]}`))
			})
		})
	})

	Describe("POST /containers", func() {
		var (
			params      url.Values
			name        string
			publicKey   string
			publicKey2  string
			callbackUrl string
		)

		BeforeEach(func() {
			params = url.Values{}
			name = "fragrant-wind-69"
			publicKey = "ssh-rsa Foo"
			publicKey2 = "ssh-rsa Bar"
			callbackUrl = "http://www.example.com/created"
			params.Set("name", name)
			params.Set("callback_url", callbackUrl)
			params.Set("public_key[]", publicKey)
			params.Add("public_key[]", publicKey2)
		})

		doRequest := func() {
			req, err = NewFormRequest("POST", "/containers", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when params are valid", func() {
			BeforeEach(func() {
				doRequest()
			})

			It("returns 202 accepted immediately", func() {
				Expect(res.Code).To(Equal(http.StatusAccepted))
				Expect(res.Body.String()).To(MatchJSON(`{"status":"accepted"}`))
			})

			It("calls provider.Create() asynchronously", func() {
				Expect(prov.CreateCalled).To(BeFalse())
				Wait()
				Expect(prov.CreateCalled).To(BeTrue())
				Expect(prov.Name).To(Equal(name))
				Expect(prov.PublicKeys).To(HaveLen(2))
				Expect(prov.PublicKeys).To(ContainElement(publicKey))
				Expect(prov.PublicKeys).To(ContainElement(publicKey2))
			})

			Context("when provider.Create()", func() {
				Context("returns an error", func() {
					BeforeEach(func() {
						prov.CreateError = errors.New("pc load letter")
						doRequest()
					})

					It("calls callback with error", func() {
						Expect(cb.InvokeCalled).To(BeFalse())
						Wait()
						Expect(cb.InvokeCalled).To(BeTrue())
						Expect(cb.To).To(Equal(callbackUrl))
						Expect(cb.Payload).To(MatchJSON(`{"error":"pc load letter"}`))
					})
				})

				Context("returns a container info with no error", func() {
					BeforeEach(func() {
						prov.CreateContainerInfo = provider.ContainerInfo{Name: name, Host: "1.2.3.4", Port: 10000}
						doRequest()
					})

					It("calls callback with container info", func() {
						Expect(cb.InvokeCalled).To(BeFalse())
						Wait()
						Expect(cb.InvokeCalled).To(BeTrue())
						Expect(cb.To).To(Equal(callbackUrl))
						Expect(cb.Payload).To(MatchJSON(`{"status":"created", "container":{"name":"fragrant-wind-69", "host":"1.2.3.4", "port":10000}}`))
					})
				})
			})
		})

		for _, param := range []string{"name", "callback_url", "public_key[]"} {
			Context(fmt.Sprintf("when param '%v' is missing", param), func() {
				BeforeEach(func() {
					params.Del(param)
					doRequest()
				})

				It("returns 422 unprocessable entity", func() {
					Expect(res.Code).To(Equal(422))
					Expect(res.Body.String()).To(MatchJSON(`{"error":"invalid params"}`))
				})

				It("does not call provider.Create()", func() {
					Expect(prov.CreateCalled).To(BeFalse())
					Wait()
					Expect(prov.CreateCalled).To(BeFalse())
				})
			})
		}
	})

	Describe("GET /containers/:container_name", func() {
		BeforeEach(func() {
			prov.Containers = []provider.ContainerInfo{
				provider.ContainerInfo{Name: "foo", Host: "1.2.3.4", Port: 10000},
			}
		})

		doRequest := func(name string) {
			req, err = NewRequest("GET", fmt.Sprintf("/containers/%v", name), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when container by the given name exists", func() {
			BeforeEach(func() {
				doRequest("foo")
			})

			It("returns 200 ok with container information", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Body.String()).To(MatchJSON(`{"container":{"name":"foo","host":"1.2.3.4","port":10000}}`))
			})
		})

		Context("when container by the given name does not exist", func() {
			BeforeEach(func() {
				doRequest("bar")
			})

			It("returns 404 not found", func() {
				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(res.Body.String()).To(MatchJSON(`{"error":"not found"}`))
			})
		})
	})
})
