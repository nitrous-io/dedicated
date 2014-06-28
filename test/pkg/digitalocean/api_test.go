package digitalocean

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	. "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotest"
	. "github.com/nitrous-io/hostbuilder/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDigitalOceanAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/digitalocean")
}

var _ = Describe("pkg/digitalocean", func() {
	var s *dotest.Server
	var ts *httptest.Server
	var c *Client

	BeforeEach(func() {
		s = dotest.NewServer()
		ts = httptest.NewServer(s.Router)
		c = TestClient(ts.URL)
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Create Droplet", func() {
		Context("Succesful Creation", func() {
			It("should successfully create a droplet", func() {
				s.AddResponse("droplets", dotest.NewResponse(200, AllDroplets))
				s.AddResponse("sizes", dotest.NewResponse(200, AllSizes))
				s.AddResponse("images/id", dotest.NewResponse(200, OneImage))
				s.AddResponse("regions", dotest.NewResponse(200, AllRegions))
				s.AddResponse("droplets/new", dotest.NewResponse(200, NewDroplet))

				droplet, err := c.CreateDroplet("test222", "33", "2", "3", 1, false)
				Expect(err).To(BeNil())

				expectedDroplet := &DropletResp{}
				json.Unmarshal([]byte(NewDroplet), expectedDroplet)
				Expect(reflect.DeepEqual(droplet, expectedDroplet.Droplet)).To(Equal(true))
			})
		})
	})

	Describe("Destroy Droplet", func() {
		Context("Successful Destruction", func() {
			It("should successfully destroy a droplet", func() {
				s.AddResponse("droplets/id/destroy", dotest.OkResponse)
				err := c.DestroyDroplet(123)
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/destroy"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/destroy"))
			})
		})
	})

	Describe("Power Off Droplet", func() {
		Context("Powering Off Droplet", func() {
			It("should successfully power off the droplet", func() {
				s.AddResponse("droplets/id/power_off", dotest.OkResponse)
				err := c.PowerOffDroplet(123)
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/power_off"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/power_off"))
			})
		})
	})

	Describe("Power On Droplet", func() {
		Context("Powering on Droplet", func() {
			It("should successfully power on the droplet", func() {
				s.AddResponse("droplets/id/power_on", dotest.OkResponse)
				err := c.PowerOnDroplet(123)
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/power_on"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/power_on"))
			})
		})
	})

	Describe("Power Cycle Droplet", func() {
		Context("Power cycling a Droplet", func() {
			It("should successfully power cycle the droplet", func() {
				s.AddResponse("droplets/id/power_cycle", dotest.OkResponse)
				err := c.PowerCycleDroplet(123)
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/power_cycle"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/power_cycle"))
			})
		})
	})

	Describe("Rebooting Droplet", func() {
		Context("Rebooting the Droplet", func() {
			It("should successfully reboot the droplet", func() {
				s.AddResponse("droplets/id/reboot", dotest.OkResponse)
				err := c.RebootDroplet(123)
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/reboot"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/reboot"))
			})
		})
	})

	Describe("Snapshotting Droplet", func() {
		Context("Succesfully Snapshotting a Droplet", func() {
			It("should successfully snapshot the droplet", func() {
				s.AddResponse("droplets/id/snapshot", dotest.OkResponse)
				err := c.CreateSnapshot(123, "winterfell-rails-123")
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/snapshot"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/snapshot"))
				Expect(request.URL.Query().Get("name")).To(Equal("winterfell-rails-123"))
			})
		})
	})

	Describe("Restoring a Droplet Snapshot", func() {
		Context("Successfully Restoring a Droplet Snapshot", func() {
			It("should successfully snapshot the droplet", func() {
				s.AddResponse("droplets/id/restore", dotest.OkResponse)
				err := c.RestoreSnapshot(123, 234)
				Expect(err).To(BeNil())
				request := s.Requests["droplets/id/restore"][0]
				Expect(request.URL.Path).To(Equal("/droplets/123/restore"))
				Expect(request.URL.Query().Get("image_id")).To(Equal("234"))
			})
		})
	})

	Describe("Creating a Key", func() {
		Context("Successfully Creating a key", func() {
			It("should create a key", func() {
				s.AddResponse("ssh_keys/new", dotest.NewResponse(200, NewSSH))
				key, err := c.CreateKey("my_key", "ssh-rsa key user@host")
				Expect(err).To(BeNil())
				Expect(key.Id).To(Equal(47))
				Expect(key.Name).To(Equal("my_key"))
				Expect(key.PubKey).To(Equal("ssh-rsa key user@host"))

				request := s.Requests["ssh_keys/new"][0]
				Expect(request.URL.Query().Get("name")).To(Equal("my_key"))
				Expect(request.URL.Query().Get("ssh_pub_key")).To(Equal("ssh-rsa key user@host"))
			})
		})
	})

	Describe("Destroying a key", func() {
		Context("Successfully Destroying a key", func() {
			It("should destroy a key", func() {
				s.AddResponse("ssh_keys/id/destroy", dotest.OkResponse)
				err := c.DestroyKey(123)
				Expect(err).To(BeNil())

				request := s.Requests["ssh_keys/id/destroy"][0]
				Expect(request.URL.Path).To(Equal("/ssh_keys/123/destroy"))
			})
		})
	})

})
