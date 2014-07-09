package httpclient_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
		c      *httpclient.Client
		ms     *MockServer
		check  func()
		token  = "correct_token"
		params = &url.Values{
			"containers": {"arch", "ubuntu"},
			"say":        {"hello world!"},
		}
	)

	BeforeEach(func() {
		ms = nil
		check = nil
		c = httpclient.New(token)
	})

	ItHasAuthHeader := func() {
		It("has auth header", func() {
			ms = AuthMockServer(token)
			check()
		})
	}

	ItHasNoAuthHeader := func() {
		It("has no auth header", func() {
			ms = AuthMockServer("")
			check()
		})
	}

	ItUsesHTTPVerb := func(verb string) {
		It("is a "+verb+" request", func() {
			ms = MethodMockServer(verb)
			check()
		})
	}

	ItIncludesQueryString := func() {
		It("includes query string", func() {
			ms = &MockServer{}
			ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
				Expect(strings.HasSuffix(r.URL.String(), "?containers=arch&containers=ubuntu&say=hello+world%21")).To(BeTrue())
				ms.Called = true
			}
			check()
		})
	}

	ItIncludesFormData := func() {
		It("includes form data", func() {
			ms = &MockServer{}
			ms.HandlerFn = func(w http.ResponseWriter, r *http.Request) {
				body, err := ioutil.ReadAll(r.Body)
				Expect(err).To(BeNil())
				Expect(body).To(Equal([]byte("containers=arch&containers=ubuntu&say=hello+world%21")))
				ms.Called = true
			}
			check()
		})
	}

	Describe("Get()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.Get(ms.URL, params)
				})
			}
		})

		ItHasNoAuthHeader()
		ItUsesHTTPVerb("GET")
		ItIncludesQueryString()
	})

	Describe("Delete()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.Delete(ms.URL, params)
				})
			}
		})

		ItHasNoAuthHeader()
		ItUsesHTTPVerb("DELETE")
		ItIncludesQueryString()
	})

	Describe("PostForm()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.PostForm(ms.URL, params)
				})
			}
		})

		ItHasNoAuthHeader()
		ItUsesHTTPVerb("POST")
		ItIncludesFormData()
	})

	Describe("PutForm()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.PutForm(ms.URL, params)
				})
			}
		})

		ItHasNoAuthHeader()
		ItUsesHTTPVerb("PUT")
		ItIncludesFormData()
	})

	Describe("GetAuthed()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.GetAuthed(ms.URL, params)
				})
			}
		})

		ItHasAuthHeader()
		ItUsesHTTPVerb("GET")
		ItIncludesQueryString()
	})

	Describe("DeleteAuthed()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.DeleteAuthed(ms.URL, params)
				})
			}
		})

		ItHasAuthHeader()
		ItUsesHTTPVerb("DELETE")
		ItIncludesQueryString()
	})

	Describe("PostFormAuthed()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.PostFormAuthed(ms.URL, params)
				})
			}
		})

		ItHasAuthHeader()
		ItUsesHTTPVerb("POST")
		ItIncludesFormData()
	})

	Describe("PutFormAuthed()", func() {
		BeforeEach(func() {
			check = func() {
				ms.Check(func() (*http.Response, error) {
					return c.PutFormAuthed(ms.URL, params)
				})
			}
		})

		ItHasAuthHeader()
		ItUsesHTTPVerb("PUT")
		ItIncludesFormData()
	})
})
