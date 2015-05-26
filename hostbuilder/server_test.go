package hostbuilder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/nitrous-io/dedicated/hostbuilder/host/dummy"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostbuilder")
}

var _ = Describe("ProgressWriter", func() {
	It("writes the stream data as newline-delimited json stream", func() {
		buf := bytes.NewBuffer(nil)
		w := NewProgressWriter(buf)
		w.Write([]byte("started: creating instance"))
		w.Write([]byte("failed: creating instance"))

		lines := strings.Split(buf.String(), "\n")
		Expect(lines[0]).To(Equal(`{"stream":"started: creating instance"}`))
		Expect(lines[1]).To(Equal(`{"stream":"failed: creating instance"}`))
	})
})

var _ = Describe("hostbuilder", func() {
	var ts *httptest.Server
	var s *Server
	var err error

	BeforeEach(func() {
		config := fmt.Sprintf(`
		{"dummy":{},
		 "allow_partial_list": true,
		 "redis":{"host":"%s","port":"%s","database":%d}
		}`,
			test.RedisHost, test.RedisPort, test.RedisTestDb)
		s, err = New([]byte(config))
		Expect(err).To(BeNil())
		ts = httptest.NewServer(s.Router)

		dummy.ReceivedNewHostDefaultConfig = nil
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Provider config", func() {
		It("Should make copy of provider config instead of returning shared copy", func() {
			provider := "dummy"

			s.Config.Providers = map[string]providerConfig{
				"dummy": providerConfig{
					"foo": "bar",
				},
			}

			originalConfig := s.Config.Providers[provider]

			gotConfig := s.providerConfig(provider)
			Expect(gotConfig["foo"]).To(Equal("bar"))

			gotConfig["foo"] = "baz"
			Expect(originalConfig["foo"]).To(Equal("bar"))
		})
	})

	Describe("Creating a host", func() {
		It("should successfully create a host and respond with host's info", func() {
			h := &dummy.Host{
				ID:            "123",
				Name:          "Created",
				CreateSuccess: true,
			}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("uid", "123")
			form.Set("id", "456")
			form.Set("slug", "dummy-456")
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			decoder := json.NewDecoder(resp.Body)
			var info Response
			for {
				err := decoder.Decode(&info)
				if err != nil {
					break
				}
			}
			// get the final result
			Expect(info.ID).To(Equal("123"))
			Expect(info.IPAddress).To(Equal(h.IPAddress))
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
			Expect(info.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})

		It("should send messages to queue when queue is true", func() {
			h := &dummy.Host{
				ID:            "123",
				Name:          "Created",
				CreateSuccess: true,
			}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("uid", "123")
			form.Set("id", "456")
			form.Set("slug", "dummy-456")
			form.Set("queue", "true")
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			decoder := json.NewDecoder(resp.Body)
			var info Response
			for {
				err := decoder.Decode(&info)
				if err != nil {
					break
				}
			}

			// get the final result
			Expect(info.ID).To(Equal("123"))
			Expect(info.IPAddress).To(Equal(h.IPAddress))
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
			Expect(info.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())

			qp := queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "v1.host")
			Expect(err).To(BeNil())
			Expect(qp.GetInputLength()).To(BeNumerically(">", 0))
			qp.Delete()
		})

		It("should return error when fails to create a host when queue is true", func() {
			h := &dummy.Host{
				ID:            "123",
				Name:          "Created",
				CreateSuccess: false,
			}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("uid", "123")
			form.Set("id", "456")
			form.Set("slug", "dummy-456")
			form.Set("queue", "true")
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			decoder := json.NewDecoder(resp.Body)
			var info Response
			for {
				err := decoder.Decode(&info)
				if err != nil {
					break
				}
			}

			Expect(info.Error).NotTo(BeZero())
			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())

			qp := queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "v1.host")
			Expect(err).To(BeNil())
			Expect(qp.GetInputLength()).To(BeZero())
			qp.Delete()
		})
	})

	Describe("Updating a host", func() {
		It("should successfully update the host", func() {
			h := &dummy.Host{UpdateSuccess: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, err := json.Marshal(h)
			Expect(err).To(BeNil())
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/update"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			decoder := json.NewDecoder(resp.Body)
			var info Response
			for {
				err := decoder.Decode(&info)
				if err != nil {
					break
				}
			}
			Expect(info.IPAddress).To(Equal(h.IPAddress))
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
			Expect(info.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})
	})

	Describe("Respeccing a host", func() {
		It("should successfully respec the host", func() {
			h := &dummy.Host{RespecSuccess: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil())
			defer srv.Close()

			config, err := json.Marshal(h)
			Expect(err).To(BeNil())
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/respec"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			decoder := json.NewDecoder(resp.Body)
			var info Response
			for {
				err := decoder.Decode(&info)
				if err != nil {
					break
				}
			}
			Expect(info.IPAddress).To(Equal(h.IPAddress))
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
			Expect(info.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})
	})

	Describe("Destroying a host", func() {
		It("should successfully destroy the host", func() {
			config, _ := json.Marshal(dummy.Host{DestroySuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/delete"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})
	})

	Describe("Stopping a host", func() {
		It("should successfully stop the host", func() {
			h := &dummy.Host{StopSuccess: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/stop"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			var info Response
			err = json.Unmarshal(body, &info)
			Expect(err).To(BeNil())
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
		})
	})

	Describe("Starting host", func() {
		It("should successfully start the host", func() {
			h := &dummy.Host{StartSuccess: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/start"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			var info Response
			err = json.Unmarshal(body, &info)
			Expect(err).To(BeNil())
			Expect(info.IPAddress).To(Equal(h.IPAddress))
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
			Expect(info.State).To(Equal(state.Running.String()))
		})
	})

	Describe("Restarting host", func() {
		It("should successfully start the host", func() {
			h := &dummy.Host{RestartSuccess: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/restart"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			var info Response
			err = json.Unmarshal(body, &info)
			Expect(err).To(BeNil())
			Expect(info.IPAddress).To(Equal(h.IPAddress))
			Expect(info.FreighterdPort).To(Equal(uint16(h.FreighterdPort)))
			Expect(info.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})
	})

	Describe("Listing hosts", func() {
		It("should successfully return all hosts", func() {
			h := &dummy.Host{ListSuccess: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.Get(fmt.Sprintf("%s/%s?%s", ts.URL, "hosts", form.Encode()))
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			var j struct {
				Hosts []Response `json:"hosts"`
			}
			json.Unmarshal(body, &j)
			Expect(j.Hosts).To(HaveLen(2)) // See hostbuilder's dummy host.

			h1 := j.Hosts[0]
			Expect(h1.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})

		It("should successfully return partial hosts with AllowPartialList enabled", func() {
			Expect(s.Config.AllowPartialList).To(BeTrue())
			h := &dummy.Host{ListSuccess: true, ListPartialError: true}
			srv, err := setupDummyHost(h)
			Expect(err).To(BeNil(), "failed to setup a dummy host")
			defer srv.Close()

			config, _ := json.Marshal(h)
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.Get(fmt.Sprintf("%s/%s?%s", ts.URL, "hosts", form.Encode()))
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			fmt.Println(string(body))
			Expect(err).To(BeNil())
			var j struct {
				Hosts        []Response `json:"hosts"`
				PartialError string     `json:"partial_error"`
			}
			json.Unmarshal(body, &j)
			Expect(j.PartialError).To(Equal("not enough dummies"))
			Expect(j.Hosts).To(HaveLen(2)) // See hostbuilder's dummy host.

			h1 := j.Hosts[0]
			Expect(h1.State).To(Equal(state.Running.String()))

			Expect(dummy.ReceivedNewHostDefaultConfig["uuid"]).ToNot(BeEmpty())
		})
	})
})

type closer interface {
	Close()
}

func setupDummyHost(h *dummy.Host) (closer, error) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello")
	}))
	addr := ts.Listener.Addr().String()
	fhost, port, err := net.SplitHostPort(addr)
	if err != nil {
		ts.Close()
		return nil, err
	}
	fport, err := strconv.ParseInt(port, 10, 64)
	if err != nil {
		ts.Close()
		return nil, err
	}

	h.IPAddress = fhost
	h.FreighterdPort = fport
	return ts, nil

}
