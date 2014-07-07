package httpclient_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/nitro/pkg/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "httpclient")
}

type MockServer struct {
	*httptest.Server

	HandlerFn http.HandlerFunc
	Called    bool
}

// a RequestFn makes a request to the MockServer
type RequestFn func() (*http.Response, error)

func (ms *MockServer) Check(fn RequestFn) {
	ms.Server = httptest.NewServer(ms.HandlerFn)
	defer ms.Server.Close()
	resp, err := fn()
	Expect(err).To(BeNil())
	defer resp.Body.Close()
	Expect(ms.Called).To(BeTrue())
	return
}

// an AuthMockServer expects an Authorization header with the given token if token is non-empty.  If token is empty, the Authorization header is expected to be absent
func AuthMockServer(token string) *MockServer {
	ms := &MockServer{}
	ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			Expect(r.Header.Get("Authorization")).To(BeEmpty())
		} else {
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + token))
		}
		ms.Called = true
	}
	return ms
}

// a MethodMockServer expects the request method to equal the method argument
func MethodMockServer(method string) *MockServer {
	ms := &MockServer{}
	ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(method))
		ms.Called = true
	}
	return ms
}

var _ = Describe("httpclient", func() {
	var (
		c     *httpclient.Client
		token = "correct_token"
	)

	BeforeEach(func() {
		c = httpclient.New(token)
	})

	Describe("AuthedGet()", func() {
		It("has auth header", func() {
			ms := AuthMockServer(token)
			ms.Check(func() (*http.Response, error) { return c.AuthedGet(ms.URL, nil) })
		})

		It("is a GET request", func() {
			ms := MethodMockServer("GET")
			ms.Check(func() (*http.Response, error) { return c.AuthedGet(ms.URL, nil) })
		})

		It("includes params in querystring", func() {
			ms := &MockServer{}
			ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.String()).To(Equal("/?host=starport-123"))
				ms.Called = true
			}
			ms.Check(func() (*http.Response, error) {
				return c.AuthedGet(ms.URL, &url.Values{
					"host": {"starport-123"},
				})
			})
		})
	})

	Describe("AuthedDelete()", func() {
		It("has auth header", func() {
			ms := AuthMockServer(token)
			ms.Check(func() (*http.Response, error) { return c.AuthedDelete(ms.URL, nil) })
		})

		It("is a DELETE request", func() {
			ms := MethodMockServer("DELETE")
			ms.Check(func() (*http.Response, error) { return c.AuthedDelete(ms.URL, nil) })
		})

		It("includes params in querystring", func() {
			ms := &MockServer{}
			ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.String()).To(Equal("/?container=kings-landing&host=starport%3A123"))
				ms.Called = true
			}
			ms.Check(func() (*http.Response, error) {
				return c.AuthedDelete(ms.URL, &url.Values{
					"container": {"kings-landing"},
					"host":      {"starport:123"},
				})
			})
		})
	})

	Describe("AuthedPostForm()", func() {
		It("has auth header", func() {
			ms := AuthMockServer(token)
			ms.Check(func() (*http.Response, error) { return c.AuthedPostForm(ms.URL, nil) })
		})

		It("is a POST request", func() {
			ms := MethodMockServer("POST")
			ms.Check(func() (*http.Response, error) { return c.AuthedPostForm(ms.URL, nil) })
		})
	})

	Describe("AuthedPutForm()", func() {
		It("has auth header", func() {
			ms := AuthMockServer(token)
			ms.Check(func() (*http.Response, error) { return c.AuthedPostForm(ms.URL, nil) })
		})

		It("is a PUT request", func() {
			ms := MethodMockServer("POST")
			ms.Check(func() (*http.Response, error) { return c.AuthedPostForm(ms.URL, nil) })
		})
	})

	Describe("Delete()", func() {
		It("has no auth header", func() {
			ms := AuthMockServer("")
			ms.Check(func() (*http.Response, error) { return c.Delete(ms.URL, nil) })
		})

		It("is a GET request", func() {
			ms := MethodMockServer("DELETE")
			ms.Check(func() (*http.Response, error) { return c.Delete(ms.URL, nil) })
		})
	})

	Describe("Get()", func() {
		It("has no auth header", func() {
			ms := AuthMockServer("")
			ms.Check(func() (*http.Response, error) { return c.Get(ms.URL, nil) })
		})

		It("is a GET request", func() {
			ms := MethodMockServer("GET")
			ms.Check(func() (*http.Response, error) { return c.Get(ms.URL, nil) })
		})

		It("includes params in querystring", func() {
			ms := &MockServer{}
			ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.String()).To(Equal("/?host=starport%22123%22"))
				ms.Called = true
			}
			ms.Check(func() (*http.Response, error) {
				return c.Get(ms.URL, &url.Values{"host": {"starport\"123\""}})
			})
		})
	})

	Describe("PutForm()", func() {
		It("has no auth header", func() {
			ms := AuthMockServer("")
			ms.Check(func() (*http.Response, error) { return c.PutForm(ms.URL, nil) })
		})

		It("is a PUT request", func() {
			ms := MethodMockServer("PUT")
			ms.Check(func() (*http.Response, error) { return c.PutForm(ms.URL, nil) })
		})

		It("has encoded data", func() {
			ms := &MockServer{}
			ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
				body, err := ioutil.ReadAll(r.Body)
				Expect(err).To(BeNil())
				Expect(body).To(Equal([]byte("containers=arch&containers=ubuntu")))
				ms.Called = true
			}
			ms.Check(func() (*http.Response, error) {
				return c.PutForm(ms.URL, &url.Values{
					"containers": {"arch", "ubuntu"},
				})
			})
		})
	})
})
