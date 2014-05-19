package servertest

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/nitrous-io/freighter/server"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTPCallback", func() {
	var (
		cb *server.HTTPCallback
	)

	BeforeEach(func() {
		cb = &server.HTTPCallback{}
	})

	Describe("Invoke()", func() {
		var (
			serv    *httptest.Server
			req     *http.Request
			reqbody string
			payload string
		)

		BeforeEach(func() {
			payload = `{"foo": "bar"}`
			serv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := ioutil.ReadAll(r.Body)
				reqbody = string(b)
				req = r
			}))
		})

		AfterEach(func() {
			serv.Close()
		})

		doAction := func() {
			cb.Invoke(serv.URL, payload)
		}

		It("makes an http request", func() {
			doAction()
			Expect(req).NotTo(BeNil())
			Expect(req.Method).To(Equal("POST"))
			Expect(reqbody).To(Equal(payload))
		})
	})
})
