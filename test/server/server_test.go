package servertest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/server"

	. "github.com/nitrous-io/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", func() {
	var (
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		err     error
		svr     *server.Server
		prov    *MockProvider
		cb      *MockCallback
	)

	BeforeEach(func() {
		prov = &MockProvider{}
		cb = &MockCallback{}
		svr = server.NewServer(nil, prov, cb)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	Describe("GET /", func() {
		BeforeEach(func() {
			req, err = NewRequest("GET", "/", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		})

		It("responds with 200 ok", func() {
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

			It("responds with 200 ok with empty container info array", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Body.String()).To(MatchJSON(`{"containers":[]}`))
			})
		})

		Context("when there exists containers", func() {
			BeforeEach(func() {
				prov.Containers = []*provider.ContainerInfo{
					{Id: "aaa", Name: "foo", Image: "ubuntu", Host: "1.2.3.4", Port: 10000, State: provider.ContainerStateRunning},
					{Id: "bbb", Name: "bar", Image: "centos", Host: "1.2.3.5", Port: 10001, State: provider.ContainerStateStopped},
				}
				doRequest()
			})

			It("responds with 200 ok with container info array", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Body.String()).To(MatchJSON(`{"containers":[
				  {"id":"aaa","name":"foo","image":"ubuntu","host":"1.2.3.4","port":10000,"state":"running"},
					{"id":"bbb","name":"bar","image":"centos","host":"1.2.3.5","port":10001,"state":"stopped"}
				]}`))
			})
		})
	})

	Describe("POST /containers", func() {
		var (
			params      *url.Values
			name        string
			image       string
			publicKey   string
			publicKey2  string
			callbackUrl string
			state       string
		)

		BeforeEach(func() {
			name = "fragrant-wind-69"
			image = "ubuntu"
			publicKey = "ssh-rsa Foo"
			publicKey2 = "ssh-rsa Bar"
			callbackUrl = "http://www.example.com/cb"
			params = &url.Values{}
			params.Set("name", name)
			params.Set("image", image)
			params.Set("callback_url", callbackUrl)
			params.Set("public_key[]", publicKey)
			params.Add("public_key[]", publicKey2)
			state = RandomName("state")
			params.Set("state", state)
		})

		doRequest := func() {
			req, err = NewFormRequest("POST", "/containers", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when params are valid", func() {
			It("responds with 202 accepted immediately", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusAccepted))
				Expect(res.Body.String()).To(MatchJSON(`{"status":"accepted"}`))
			})

			It("calls provider.Create() asynchronously", func() {
				doRequest()
				Expect(prov.CreateCalled).To(BeFalse())
				Wait()
				Expect(prov.CreateCalled).To(BeTrue())
				Expect(prov.ReceivedName).To(Equal(name))
				Expect(prov.ReceivedImage).To(Equal(image))
				Expect(prov.ReceivedPublicKeys).To(HaveLen(2))
				Expect(prov.ReceivedPublicKeys).To(ContainElement(publicKey))
				Expect(prov.ReceivedPublicKeys).To(ContainElement(publicKey2))
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
						Expect(cb.Payload).To(MatchJSON(`{"error":"pc load letter", "state": "` + state + `"}`))
					})
				})

				Context("returns a container info with no error", func() {
					BeforeEach(func() {
						prov.CreateContainerInfo = &provider.ContainerInfo{Id: "aaa", Name: name, Image: "ubuntu", Host: "1.2.3.4", Port: 10000, State: provider.ContainerStateRunning}
						doRequest()
					})

					It("calls callback with container info", func() {
						Expect(cb.InvokeCalled).To(BeFalse())
						Wait()
						Expect(cb.InvokeCalled).To(BeTrue())
						Expect(cb.To).To(Equal(callbackUrl))
						Expect(cb.Payload).To(MatchJSON(`{"result":"created", "container":{
							"id":"aaa","name":"fragrant-wind-69","image":"ubuntu","host":"1.2.3.4","port":10000,"state":"running"
						}, "state": "` + state + `"}`))
					})
				})
			})
		})

		for _, param := range []string{"name", "image"} {
			Context(fmt.Sprintf("when param '%v' is missing", param), func() {
				BeforeEach(func() {
					params.Del(param)
					doRequest()
				})

				It("responds with 422 unprocessable entity", func() {
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

		Context("when an invalid name is given", func() {
			BeforeEach(func() {
				params.Set("name", "-foobar")
				doRequest()
			})

			It("responds with 422 unprocessable entity", func() {
				Expect(res.Code).To(Equal(422))
				Expect(res.Body.String()).To(MatchJSON(`{"error":"invalid params"}`))
			})

			It("does not call provider.Create()", func() {
				Expect(prov.CreateCalled).To(BeFalse())
				Wait()
				Expect(prov.CreateCalled).To(BeFalse())
			})
		})
	})

	Describe("GET /containers/:container_id", func() {
		BeforeEach(func() {
			prov.Containers = []*provider.ContainerInfo{
				{Id: "aaa", Name: "foo", Image: "ubuntu", Host: "1.2.3.4", Port: 10000, State: provider.ContainerStateRunning},
			}
		})

		doRequest := func(id string) {
			req, err = NewRequest("GET", fmt.Sprintf("/containers/%v", id), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when container by the given id exists", func() {
			BeforeEach(func() {
				doRequest("aaa")
			})

			It("responds with 200 ok with container information", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(res.Body.String()).To(MatchJSON(`{"container":{
					"id":"aaa","name":"foo","image":"ubuntu","host":"1.2.3.4","port":10000,"state":"running"
				}}`))
			})
		})

		Context("when container by the given id does not exist", func() {
			BeforeEach(func() {
				doRequest("bbb")
			})

			It("responds with 404 not found", func() {
				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
			})
		})
	})

	Describe("Container Control", func() {
		var (
			provFuncCalled *bool
			provFuncError  *error
			successStatus  string
			doRequest      func(string)
			params         *url.Values
		)

		itWorks := func() {
			var (
				id          string
				callbackUrl string
				state       string
			)

			BeforeEach(func() {
				id = "aaa"
				callbackUrl = "http://www.example.com/cb"
				params = &url.Values{}
				params.Set("callback_url", callbackUrl)
				state = RandomName("state")
				params.Set("state", state)

				prov.Containers = []*provider.ContainerInfo{
					{Id: id, Name: "foo", Image: "ubuntu", Host: "1.2.3.4", Port: 10000},
				}
			})

			Context("when container by the given id exists", func() {
				It("responds with 202 accepted immediately", func() {
					doRequest(id)
					Expect(res.Code).To(Equal(http.StatusAccepted))
					Expect(res.Body.String()).To(MatchJSON(`{"status":"accepted"}`))
				})

				It("calls provider func asynchronously", func() {
					doRequest(id)
					Expect(*provFuncCalled).To(BeFalse())
					Wait()
					Expect(*provFuncCalled).To(BeTrue())
					Expect(prov.ReceivedId).To(Equal(id))
				})

				Context("when provider func", func() {
					Context("returns an error", func() {
						BeforeEach(func() {
							*provFuncError = errors.New("pc load letter")
							doRequest(id)
						})

						It("calls callback with error", func() {
							Expect(cb.InvokeCalled).To(BeFalse())
							Wait()
							Expect(cb.InvokeCalled).To(BeTrue())
							Expect(cb.To).To(Equal(callbackUrl))
							Expect(cb.Payload).To(MatchJSON(`{"error":"pc load letter", "state":"` + state + `"}`))
						})
					})

					Context("returns a container info with no error", func() {
						BeforeEach(func() {
							doRequest(id)
						})

						It("calls callback with no error", func() {
							Expect(cb.InvokeCalled).To(BeFalse())
							Wait()
							Expect(cb.InvokeCalled).To(BeTrue())
							Expect(cb.To).To(Equal(callbackUrl))
							Expect(cb.Payload).To(MatchJSON(fmt.Sprintf(`{"result":"%v", "container_id":"aaa", "state":"`+state+`"}`, successStatus)))
						})
					})
				})
			})

			Context("when container by the given id does not exist", func() {
				BeforeEach(func() {
					doRequest("bbb")
				})

				It("responds with 404 not found", func() {
					Expect(res.Code).To(Equal(http.StatusNotFound))
					Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
				})
			})
		}

		Describe("DELETE /containers/:container_id", func() {
			BeforeEach(func() {
				provFuncCalled = &prov.TerminateCalled
				provFuncError = &prov.TerminateError
				successStatus = "terminated"

				doRequest = func(id string) {
					req, err = NewRequest("DELETE", fmt.Sprintf("/containers/%v", id), params)
					Expect(err).To(BeNil())
					handler.ServeHTTP(res, req)
				}
			})

			itWorks()
		})

		Describe("POST /containers/:container_id/start", func() {
			BeforeEach(func() {
				provFuncCalled = &prov.StartCalled
				provFuncError = &prov.StartError
				successStatus = "started"

				doRequest = func(id string) {
					req, err = NewFormRequest("POST", fmt.Sprintf("/containers/%v/start", id), params)
					Expect(err).To(BeNil())
					handler.ServeHTTP(res, req)
				}
			})

			itWorks()
		})

		Describe("POST /containers/:container_id/stop", func() {
			BeforeEach(func() {
				provFuncCalled = &prov.StopCalled
				provFuncError = &prov.StopError
				successStatus = "stopped"

				doRequest = func(id string) {
					req, err = NewFormRequest("POST", fmt.Sprintf("/containers/%v/stop", id), params)
					Expect(err).To(BeNil())
					handler.ServeHTTP(res, req)
				}
			})

			itWorks()
		})

		Describe("POST /containers/:container_id/restart", func() {
			BeforeEach(func() {
				provFuncCalled = &prov.RestartCalled
				provFuncError = &prov.RestartError
				successStatus = "restarted"

				doRequest = func(id string) {
					req, err = NewFormRequest("POST", fmt.Sprintf("/containers/%v/restart", id), params)
					Expect(err).To(BeNil())
					handler.ServeHTTP(res, req)
				}
			})

			itWorks()
		})
	})

	Describe("GET /containers/:container_id/public_keys", func() {
		var (
			id           = "cafebabe"
			publicKey1   = "ssh-rsa Foo"
			publicKey2   = "ssh-rsa Bar"
			publicKey3   = "ssh-dsa Baz"
			fingerprint1 = "11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11"
			fingerprint2 = "22:22:22:22:22:22:22:22:22:22:22:22:22:22:22:22"
			fingerprint3 = "33:33:33:33:33:33:33:33:33:33:33:33:33:33:33:33"
		)

		doRequest := func() {
			req, err = NewRequest("GET", fmt.Sprintf("/containers/%v/public_keys", id), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("calls provider.ListPublicKeys()", func() {
			Expect(prov.ListPublicKeysCalled).To(BeFalse())
			doRequest()
			Expect(prov.ListPublicKeysCalled).To(BeTrue())
			Expect(prov.ReceivedId).To(Equal(id))
		})

		Context("when provider.ListPublicKeys() fails with error", func() {
			Context("because no such container exists", func() {
				BeforeEach(func() {
					prov.ListPublicKeysError = ErrMockProviderContainerNotFound
					doRequest()
				})

				It("responds with 404 not found", func() {
					Expect(res.Code).To(Equal(http.StatusNotFound))
					Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
				})
			})

			Context("because of some unknown error", func() {
				BeforeEach(func() {
					prov.ListPublicKeysError = errors.New("unknown error")
					doRequest()
				})

				It("responds with 500 internal server error with error json", func() {
					Expect(res.Code).To(Equal(http.StatusInternalServerError))
					Expect(res.Body.String()).To(MatchJSON(`{"error":"unknown error"}`))
				})
			})
		})

		Context("when provider.ListPublicKeys() succeeds", func() {
			BeforeEach(func() {
				prov.PublicKeys = map[string]string{fingerprint1: publicKey1, fingerprint2: publicKey2, fingerprint3: publicKey3}
				doRequest()
			})

			It("responds with 200 ok with public keys json", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				j, _ := json.Marshal(map[string]interface{}{
					"public_keys": prov.PublicKeys,
				})

				Expect(res.Body.String()).To(MatchJSON(string(j)))
			})
		})
	})

	Describe("POST /containers/:container_id/public_keys", func() {
		var (
			params     *url.Values
			id         = "cafebabe"
			publicKey  = "ssh-rsa Foo"
			publicKey2 = "ssh-rsa Bar"
		)

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("public_key[]", publicKey)
			params.Add("public_key[]", publicKey2)
		})

		doRequest := func() {
			req, err = NewFormRequest("POST", fmt.Sprintf("/containers/%v/public_keys", id), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when params are valid", func() {
			It("calls provider.AddPublicKeys()", func() {
				Expect(prov.AddPublicKeysCalled).To(BeFalse())
				doRequest()
				Expect(prov.AddPublicKeysCalled).To(BeTrue())
				Expect(prov.ReceivedId).To(Equal(id))
				Expect(prov.ReceivedPublicKeys).To(HaveLen(2))
				Expect(prov.ReceivedPublicKeys).To(ContainElement(publicKey))
				Expect(prov.ReceivedPublicKeys).To(ContainElement(publicKey2))
			})

			Context("when provider.AddPublicKeys() fails with error", func() {
				Context("because no such container exists", func() {
					BeforeEach(func() {
						prov.AddPublicKeysError = ErrMockProviderContainerNotFound
						doRequest()
					})

					It("responds with 404 not found", func() {
						Expect(res.Code).To(Equal(http.StatusNotFound))
						Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
					})
				})

				Context("because of some unknown error", func() {
					BeforeEach(func() {
						prov.AddPublicKeysError = errors.New("unknown error")
						doRequest()
					})

					It("responds with 500 internal server error with error json", func() {
						Expect(res.Code).To(Equal(http.StatusInternalServerError))
						Expect(res.Body.String()).To(MatchJSON(`{"error":"unknown error"}`))
					})
				})
			})

			Context("when provider.AddPublicKeys() succeeds", func() {
				Context("when provider.ListPublicKeys() fails with error", func() {
					Context("because no such container exists", func() {
						BeforeEach(func() {
							prov.ListPublicKeysError = ErrMockProviderContainerNotFound
							doRequest()
						})

						It("responds with 404 not found", func() {
							Expect(res.Code).To(Equal(http.StatusNotFound))
							Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
						})
					})

					Context("because of some unknown error", func() {
						BeforeEach(func() {
							prov.ListPublicKeysError = errors.New("unknown error")
							doRequest()
						})

						It("responds with 500 internal server error with error json", func() {
							Expect(res.Code).To(Equal(http.StatusInternalServerError))
							Expect(res.Body.String()).To(MatchJSON(`{"error":"unknown error"}`))
						})
					})
				})

				Context("when provider.ListPublicKeys() succeeds", func() {
					fingerprint1 := "11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11"
					fingerprint2 := "22:22:22:22:22:22:22:22:22:22:22:22:22:22:22:22"
					fingerprint3 := "33:33:33:33:33:33:33:33:33:33:33:33:33:33:33:33"
					publicKey3 := "ssh-dsa Baz"

					BeforeEach(func() {
						prov.PublicKeys = map[string]string{fingerprint1: publicKey, fingerprint2: publicKey2, fingerprint3: publicKey3}
						doRequest()
					})

					It("responds with 200 ok with public keys json", func() {
						Expect(prov.ListPublicKeysCalled).To(BeTrue())
						Expect(prov.ReceivedId).To(Equal(id))
						Expect(res.Code).To(Equal(http.StatusOK))
						j, _ := json.Marshal(map[string]interface{}{
							"public_keys": prov.PublicKeys,
						})

						Expect(res.Body.String()).To(MatchJSON(string(j)))
					})
				})
			})
		})

		Context("when public_key[] param is missing", func() {
			BeforeEach(func() {
				params.Del("public_key[]")
				doRequest()
			})

			It("responds with 422 unprocessable entity", func() {
				Expect(res.Code).To(Equal(422))
				Expect(res.Body.String()).To(MatchJSON(`{"error":"invalid params"}`))
			})

			It("does not call provider.AddPublicKeys()", func() {
				Expect(prov.AddPublicKeysCalled).To(BeFalse())
			})
		})
	})

	Describe("DELETE /containers/:container_id/public_keys/:key_fingerprint", func() {
		var (
			params      *url.Values
			id          = "cafebabe"
			fingerprint = "11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11"
		)

		doRequest := func() {
			req, err = NewRequest("DELETE", fmt.Sprintf("/containers/%v/public_keys/%v", id, fingerprint), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("calls provider.RemovePublicKey()", func() {
			Expect(prov.RemovePublicKeyCalled).To(BeFalse())
			doRequest()
			Expect(prov.RemovePublicKeyCalled).To(BeTrue())
			Expect(prov.ReceivedId).To(Equal(id))
			Expect(prov.ReceivedKeyFingerprint).To(Equal(fingerprint))
		})

		Context("when provider.RemovePublicKey() fails with error", func() {
			Context("because no such container exists", func() {
				BeforeEach(func() {
					prov.RemovePublicKeyError = ErrMockProviderContainerNotFound
					doRequest()
				})

				It("responds with 404 not found", func() {
					Expect(res.Code).To(Equal(http.StatusNotFound))
					Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
				})
			})

			Context("because of some unknown error", func() {
				BeforeEach(func() {
					prov.RemovePublicKeyError = errors.New("unknown error")
					doRequest()
				})

				It("responds with 500 internal server error with error json", func() {
					Expect(res.Code).To(Equal(http.StatusInternalServerError))
					Expect(res.Body.String()).To(MatchJSON(`{"error":"unknown error"}`))
				})
			})
		})

		Context("when provider.RemovePublicKey() succeeds", func() {
			Context("when provider.ListPublicKeys() fails with error", func() {
				Context("because no such container exists", func() {
					BeforeEach(func() {
						prov.ListPublicKeysError = ErrMockProviderContainerNotFound
						doRequest()
					})

					It("responds with 404 not found", func() {
						Expect(res.Code).To(Equal(http.StatusNotFound))
						Expect(res.Body.String()).To(MatchJSON(`{"error":"no such container"}`))
					})
				})

				Context("because of some unknown error", func() {
					BeforeEach(func() {
						prov.ListPublicKeysError = errors.New("unknown error")
						doRequest()
					})

					It("responds with 500 internal server error with error json", func() {
						Expect(res.Code).To(Equal(http.StatusInternalServerError))
						Expect(res.Body.String()).To(MatchJSON(`{"error":"unknown error"}`))
					})
				})
			})

			Context("when provider.ListPublicKeys() succeeds", func() {
				publicKey := "ssh-dsa Baz"

				BeforeEach(func() {
					prov.PublicKeys = map[string]string{fingerprint: publicKey}
					doRequest()
				})

				It("responds with 200 ok with public keys json", func() {
					Expect(prov.ListPublicKeysCalled).To(BeTrue())
					Expect(prov.ReceivedId).To(Equal(id))
					Expect(res.Code).To(Equal(http.StatusOK))
					j, _ := json.Marshal(map[string]interface{}{
						"public_keys": prov.PublicKeys,
					})

					Expect(res.Body.String()).To(MatchJSON(string(j)))
				})
			})
		})
	})
})
