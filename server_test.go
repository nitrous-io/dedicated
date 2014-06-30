package hostbuilder_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		It("should successfully create a host and respond with host's info", func() {
			config, _ := json.Marshal(dummy.Host{Id: "123", IpAddress: "10.0.0.1", CreateSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(201))

			var info host.Info
			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())

			json.Unmarshal(body, &info)
			Expect(info.Id).To(Equal("123"))
			Expect(info.IpAddress).To(Equal("10.0.0.1"))
		})
	})

	Describe("Destroying a host", func() {
		It("should successfully destroy a host and call a webhook", func() {
			config, _ := json.Marshal(dummy.Host{DestroySuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/delete"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("Stopping a host", func() {
		It("should successfully stop host and call webhook", func() {
			config, _ := json.Marshal(dummy.Host{StopSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/stop"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("Starting host", func() {
		It("should successfully start host and call webhook", func() {
			config, _ := json.Marshal(dummy.Host{StartSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/start"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("Restarting host", func() {
		It("should successfully start host and call webhook", func() {
			config, _ := json.Marshal(dummy.Host{RestartSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/restart"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})
})
