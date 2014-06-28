package servertest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	. "github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/host/dummy"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDigitalOceanAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostbuilder")
}

type webhookTest func(data WebhookData)

func webhookTestServer(wg *sync.WaitGroup, f webhookTest) *httptest.Server {
	wg.Add(1)
	wh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer wg.Done()
		r.ParseForm()

		payload := r.Form.Get("payload")
		var data WebhookData
		json.Unmarshal([]byte(payload), &data)
		f(data)
	}))
	return wh
}

var _ = Describe("hostbuilder", func() {
	var ts *httptest.Server
	var s *Server
	var err error

	BeforeEach(func() {
		s, err = New([]byte(`{"dummy":{}}`))
		Expect(err).To(BeNil())
		ts = httptest.NewServer(s.Router)
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Creating a host", func() {
		It("should successfully create a host and call a webhook", func() {
			var wg sync.WaitGroup
			wh := webhookTestServer(&wg, func(data WebhookData) {
				Expect(data.Success).To(Equal(true))
				var info host.Info
				json.Unmarshal([]byte(data.Message), &info)
				Expect(info.Id).To(Equal("123"))
				Expect(info.IpAddress).To(Equal("10.0.0.1"))
			})
			defer wh.Close()

			config, _ := json.Marshal(dummy.Host{Id: "123", IpAddress: "10.0.0.1", CreateSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			form.Set("webhook", string(wh.URL))
			http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host"), form)
			wg.Wait()
		})
	})

	Describe("Destroying a host", func() {
		It("should successfully destroy a host and call a webhook", func() {
			var wg sync.WaitGroup
			wh := webhookTestServer(&wg, func(data WebhookData) {
				Expect(data.Success).To(Equal(true))
			})
			defer wh.Close()

			config, _ := json.Marshal(dummy.Host{DestroySuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("webhook", string(wh.URL))
			form.Set("config", string(config))
			http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/delete"), form)

			wg.Wait()
		})
	})

	Describe("Stopping a host", func() {
		It("should successfully stop host and call webhook", func() {
			var wg sync.WaitGroup
			wh := webhookTestServer(&wg, func(data WebhookData) {
				Expect(data.Success).To(Equal(true))
			})
			defer wh.Close()

			config, _ := json.Marshal(dummy.Host{StopSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("webhook", string(wh.URL))
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/stop"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			wg.Wait()
		})
	})

	Describe("Starting host", func() {
		It("should successfully start host and call webhook", func() {
			var wg sync.WaitGroup
			wh := webhookTestServer(&wg, func(data WebhookData) {
				Expect(data.Success).To(Equal(true))
			})
			defer wh.Close()

			config, _ := json.Marshal(dummy.Host{StartSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("webhook", string(wh.URL))
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/start"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			wg.Wait()
		})
	})

	Describe("Restarting host", func() {
		It("should successfully start host and call webhook", func() {
			var wg sync.WaitGroup
			wh := webhookTestServer(&wg, func(data WebhookData) {
				Expect(data.Success).To(Equal(true))
			})
			defer wh.Close()

			config, _ := json.Marshal(dummy.Host{RestartSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("webhook", string(wh.URL))
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/restart"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			wg.Wait()
		})
	})
})
