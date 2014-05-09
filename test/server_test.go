package freightertest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/server"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", func() {
	var (
		handler  *mux.Router
		res      *httptest.ResponseRecorder
		req      *http.Request
		err      error
		provider *MockProvider
	)

	BeforeEach(func() {
		provider = &MockProvider{}
		handler = server.NewRouter(provider)
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

			var j map[string]interface{}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			Expect(j["status"]).To(Equal("ok"))
		})
	})

	Describe("POST /containers", func() {
		var (
			params     url.Values
			name       string
			publicKey  string
			publicKey2 string
			webHookUrl string
		)

		BeforeEach(func() {
			params = url.Values{}
			name = "fragrant-wind-69"
			publicKey = "ssh-rsa Foo"
			publicKey2 = "ssh-rsa Bar"
			webHookUrl = "http://www.example.com/created"
			params.Set("name", name)
			params.Set("webhook_url", webHookUrl)
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

				var j map[string]interface{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				Expect(j["status"]).To(Equal("accepted"))
			})

			It("calls provider.Create() asynchronously", func() {
				Expect(provider.CreateCalled).To(Equal(false))
				runtime.Gosched()
				Expect(provider.CreateCalled).To(Equal(true))
				Expect(provider.Name).To(Equal(name))
				Expect(provider.PublicKeys).To(HaveLen(2))
				Expect(provider.PublicKeys).To(ContainElement(publicKey))
				Expect(provider.PublicKeys).To(ContainElement(publicKey2))
			})
		})

		for _, param := range []string{"name", "webhook_url", "public_key[]"} {
			Context(fmt.Sprintf("when param '%v' is missing", param), func() {
				BeforeEach(func() {
					params.Del(param)
					doRequest()
				})

				It("returns 422 unprocessable entity", func() {
					Expect(res.Code).To(Equal(422))

					var j map[string]interface{}
					err = json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					Expect(j["error"]).To(Equal("invalid params"))
				})

				It("does not call provider.Create()", func() {
					Expect(provider.CreateCalled).To(Equal(false))
					runtime.Gosched()
					Expect(provider.CreateCalled).To(Equal(false))
				})
			})
		}
	})
})
