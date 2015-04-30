package digitalocean_test

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/dedicated/hostbuilder/host/digitalocean"
	dotest "github.com/nitrous-io/dedicated/hostbuilder/pkg/dotestv2"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type newlineWriter struct {
	w io.Writer
}

func (w *newlineWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		return n, err
	}
	w.w.Write([]byte("\n"))
	return n, nil
}

func init() {
	digitalocean.StateWaitTime = 10 * time.Millisecond
}

var _ = Describe("host/digitalocean", func() {
	var s *dotest.Server
	var ts *httptest.Server
	var c *godo.Client

	BeforeEach(func() {
		s = dotest.NewServer()
		ts = httptest.NewServer(s.Router)
		c = godo.NewClient(nil)
		c.BaseURL, _ = url.Parse(ts.URL)
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Create", func() {
		BeforeEach(func() {
			s.AddResponse("POST", "keys/create", dotest.NewResponse(200, newSSH))
			s.AddResponse("GET", "sizes", dotest.NewResponse(200, allSizes))
			s.AddResponse("GET", "images/id", dotest.NewResponse(200, oneImage))
			s.AddResponse("GET", "regions", dotest.NewResponse(200, allRegions))
			s.AddResponse("POST", "droplets/create", dotest.NewResponse(200, newDroplet))
		})

		Context("Successful creation", func() {
			// FIXME: this test was missing and I started to write it
			// but found out that it actually tries to ssh into the server to do stuff
			// so not sure how to do it - arun
		})

		Context("when there is a timeout creating the host", func() {
			BeforeEach(func() {
				s.AddResponse("GET", "actions/id", dotest.NewResponse(200, completedAction))
				s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
			})

			It("should return an ErrCreateTimeout", func() {
				providerConfig := map[string]string{
					"create_timeout": "1ms", // Use a short timeout.
					"token":          "dummy-token",
				}
				h, err := digitalocean.NewHost(providerConfig, `{
					"id": "1",
					"image_id": "1",
					"region_id": "nyc",
					"size_id": "512mb"
				}`)
				Expect(err).To(BeNil())
				doHost := h.(*digitalocean.Host)
				doHost.Client = c

				_, err = h.Create(&newlineWriter{os.Stdout}, nil)
				Expect(err).To(Equal(digitalocean.ErrCreateTimeout))
			})
		})

		Context("when DigitalOcean fails to create the host", func() {
			BeforeEach(func() {
				s.AddResponse("GET", "actions/id", dotest.NewResponse(200, erroredAction))
				s.AddResponse("DELETE", "droplets/id/destroy", dotest.NoResponse)
				s.AddResponse("DELETE", "ssh_keys/id/destroy", dotest.NoResponse)
			})

			It("should clean up the droplet", func() {
				h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
					"id": "1",
					"image_id": "1",
					"region_id": "nyc",
					"size_id": "512mb"
				}`)
				Expect(err).To(BeNil())
				doHost := h.(*digitalocean.Host)
				doHost.Client = c

				_, err = h.Create(&newlineWriter{os.Stdout}, nil)
				Expect(err).NotTo(BeNil(), "expect create to fail")
			})
		})
	})

	Describe("Stop", func() {
		BeforeEach(func() {
			s.AddResponse("POST", "droplets/id/actions", dotest.NewResponse(200, inprogressAction))
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
		})

		It("stops the droplet and waits until the action is completed", func() {
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, completedAction))
			h := digitalocean.Host{Config: &digitalocean.HostConfig{ID: "1"}, Client: c}

			info, err := h.Stop()
			Expect(info).ToNot(BeNil())
			Expect(err).To(BeNil())

			request := s.Requests["POST"]["droplets/id/actions"][0]
			var m map[string]string
			err = json.NewDecoder(request.Body).Decode(&m)
			Expect(m["type"]).To(Equal("power_off"), "unexpected action type for stop")

			request = s.Requests["GET"]["droplets/id"][0]
			Expect(request.URL.Path).To(Equal("/v2/droplets/1"))

			Expect(len(s.Requests["GET"]["actions/id"])).To(Equal(1), "expect stop to waits for action to be completed")
		})

		It("returns error when the action failed to complete", func() {
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, erroredAction))
			h := digitalocean.Host{Config: &digitalocean.HostConfig{ID: "1"}, Client: c}
			_, err := h.Stop()
			Expect(err).ToNot(BeNil(), "failed to return error when stop failed")
		})
	})

	Describe("Start", func() {
		BeforeEach(func() {
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
			s.AddResponse("POST", "droplets/id/actions", dotest.NewResponse(200, inprogressAction))
		})

		It("starts the droplet and waits until the action is completed", func() {
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, completedAction))
			h := digitalocean.Host{Config: &digitalocean.HostConfig{ID: "1"}, Client: c}

			info, err := h.Start()
			Expect(info).ToNot(BeNil())
			Expect(err).To(BeNil())

			request := s.Requests["POST"]["droplets/id/actions"][0]
			var m map[string]string
			err = json.NewDecoder(request.Body).Decode(&m)
			Expect(m["type"]).To(Equal("power_on"), "unexpected action type for start")

			request = s.Requests["GET"]["droplets/id"][0]
			Expect(request.URL.Path).To(Equal("/v2/droplets/1"))

			Expect(len(s.Requests["GET"]["actions/id"])).To(Equal(1), "expect start to waits for action to be completed")
		})

		It("returns error when the action failed to complete", func() {
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, erroredAction))
			h := digitalocean.Host{Config: &digitalocean.HostConfig{ID: "1"}, Client: c}
			_, err := h.Start()
			Expect(err).ToNot(BeNil(), "failed to return error when start failed")
		})
	})

	Describe("Restart", func() {
		BeforeEach(func() {
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
			s.AddResponse("POST", "droplets/id/actions", dotest.NewResponse(200, inprogressAction))
		})

		It("restarts the droplet and waits until the action is completed", func() {
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, completedAction))
			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			info, err := h.Restart()
			Expect(info).ToNot(BeNil())
			Expect(err).To(BeNil())

			request := s.Requests["POST"]["droplets/id/actions"][0]
			var m map[string]string
			err = json.NewDecoder(request.Body).Decode(&m)
			Expect(m["type"]).To(Equal("reboot"), "unexpected action type for restart")

			request = s.Requests["GET"]["droplets/id"][0]
			Expect(request.URL.Path).To(Equal("/v2/droplets/1"))

			Expect(len(s.Requests["GET"]["actions/id"])).To(Equal(1),
				"expected restart to waits for action to be completed")
		})

		It("returns error when the action failed to complete", func() {
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, erroredAction))
			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			_, err = h.Restart()
			Expect(err).ToNot(BeNil(), "failed to return error when restart failed")
		})
	})

	Describe("Info", func() {
		BeforeEach(func() {
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
		})

		It("return info about the droplet", func() {
			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			info, err := h.Info()
			Expect(err).To(BeNil())
			Expect(info).ToNot(BeNil())

			request := s.Requests["GET"]["droplets/id"][0]
			Expect(request.URL.Path).To(Equal("/v2/droplets/1"))
		})
	})

	Describe("Update", func() {
		It("renames the droplet and waits until the action is completed", func() {
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
			s.AddResponse("POST", "droplets/id/actions", dotest.NewResponse(200, inprogressAction))
			s.AddResponse("GET", "actions/id", dotest.NewResponse(200, completedAction))

			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1",
				"name": "new-name"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			info, err := h.Update(ioutil.Discard)
			Expect(err).ToNot(HaveOccurred())
			Expect(info).ToNot(BeNil())

			req := s.Requests["POST"]["droplets/id/actions"][0]
			var m map[string]string
			err = json.NewDecoder(req.Body).Decode(&m)
			Expect(m["type"]).To(Equal("rename"))
			Expect(m["name"]).To(Equal("new-name"))

			req = s.Requests["GET"]["droplets/id"][0]
			Expect(req.URL.Path).To(Equal("/v2/droplets/1"))

			Expect(len(s.Requests["GET"]["actions/id"])).To(Equal(1),
				"expected update to wait for rename action to be completed")
		})

		It("fails when droplet is not found", func() {
			s.AddResponse("GET", "droplets/id", dotest.NotfoundResponse)
			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			_, err = h.Update(ioutil.Discard)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Destroy", func() {
		It("succeeds when the droplet is not found", func() {
			s.AddResponse("GET", "droplets/id", dotest.NotfoundResponse)
			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			err = h.Destroy()
			Expect(err).To(BeNil(), "failed to destroy not-found droplet")
		})

		It("succeeds when destroy droplet returns 404", func() {
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(200, droplet))
			s.AddResponse("POST", "droplets/id/actions", dotest.NotfoundResponse)

			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			err = h.Destroy()
			Expect(err).To(BeNil(), "failed to destroy not-found droplet")
		})

		It("fails when the API errors out", func() {
			s.AddResponse("GET", "droplets/id", dotest.NewResponse(500, ""))
			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{
				"id": "1"
			}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			err = h.Destroy()
			Expect(err).ToNot(BeNil(), "expect error when DO failed to process request")
		})
	})

	Describe("List", func() {
		It("returns all user created droplets (except tender hosts)", func() {
			s.AddResponse("GET", "droplets", dotest.NewResponse(200, allDroplets))

			h, err := digitalocean.NewHost(map[string]string{"token": "dummy-token"}, `{}`)
			Expect(err).To(BeNil())
			doHost := h.(*digitalocean.Host)
			doHost.Client = c

			hosts, err := h.List()
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(2))

			h1, h2 := hosts[0], hosts[1]
			Expect(h1.ID).To(Equal("20"))
			Expect(h1.Name).To(Equal("test.example.com"))
			Expect(h1.IPAddress).To(Equal("127.0.0.20"))
			Expect(h1.State).To(Equal(state.Running))
			Expect(h2.ID).To(Equal("21"))
			Expect(h2.Name).To(Equal("test2.example.com"))
			Expect(h2.IPAddress).To(Equal("127.0.0.22"))
			Expect(h2.State).To(Equal(state.Running))

			Expect(len(s.Requests["GET"]["droplets"])).To(Equal(1))
		})
	})
})

// fixtures
var (
	inprogressAction = `
	{
		"action": {
			"id": 20,
			"status": "in-progress",
			"type": "create",
			"started_at": "2014-06-10T22:25:14Z",
			"completed_at": "2014-06-10T23:25:14Z",
			"resource_id": 123,
			"resource_type": "droplet",
			"region": "nyc1"
		}
	}
	`
	completedAction = `
	{
		"action": {
			"id": 20,
			"status": "completed",
			"type": "create",
			"started_at": "2014-06-10T22:25:14Z",
			"completed_at": "2014-06-10T23:25:14Z",
			"resource_id": 123,
			"resource_type": "droplet",
			"region": "nyc1"
		}
	}
	`

	erroredAction = `
	{
		"action": {
			"id": 20,
			"status": "errored",
			"type": "create",
			"started_at": "2014-06-10T22:25:14Z",
			"completed_at": "2014-06-10T23:25:14Z",
			"resource_id": 123,
			"resource_type": "droplet",
			"region": "nyc1"
		}
	}
	`

	// /droplets/new
	newDroplet = `
	{
		"droplet": {
			"id": 25,
			"name": "My-Droplet",
			"memory": 512,
			"vcpus": 1,
			"disk": 20,
			"region": {
				"slug": "nyc1",
				"name": "New York",
				"sizes": [ "1gb", "512mb" ],
				"available": true,
				"features": [ "virtio", "private_networking", "backups", "ipv6" ]
			},
			"image": {
				"id": 449676389,
				"name": "Ubuntu 13.04",
				"distribution": "ubuntu",
				"slug": null,
				"public": true,
				"regions": [ "nyc1" ],
				"created_at": "2014-09-05T02:02:07Z"
			},
			"size": {
				"slug": "512mb",
				"transfer": 1,
				"price_monthly": 5.0,
				"price_hourly": 0.00744
			},
			"locked": false,
			"status": "new",
			"networks": {
			},
			"kernel": {
				"id": 485432972,
				"name": "Ubuntu 14.04 x64 vmlinuz-3.13.0-24-generic (1221)",
				"version": "3.13.0-24-generic"
			},
			"created_at": "2014-09-05T02:02:07Z",
			"features": [ "virtio" ],
			"backup_ids": [ ],
			"snapshot_ids": [ ]
		},
		"links": {
			"actions": [ { "id": 20, "rel": "create", "href": "http://example.org/v2/actions/20" } ]
		}
	}
	`

	// droplet show
	droplet = `
	{
		"droplet": {
			"id": 20,
			"name": "test.example.com",
			"memory": 512,
			"vcpus": 1,
			"disk": 20,
			"region": {
				"slug": "nyc1",
				"name": "New York",
				"sizes": [ "1gb", "512mb" ],
				"available": true,
				"features": [ "virtio", "private_networking", "backups", "ipv6" ]
			},
			"image": {
				"id": 119192817,
				"name": "Ubuntu 13.04",
				"distribution": "ubuntu",
				"slug": "ubuntu1304",
				"public": true,
				"regions": [ "nyc1" ],
				"created_at": "2014-09-05T02:02:05Z"
			},
			"size": {
				"slug": "512mb",
				"transfer": 1,
				"price_monthly": 5.0,
				"price_hourly": 0.00744
			},
			"locked": false,
			"status": "active",
			"networks": {
				"v4": [ { "ip_address": "127.0.0.20", "netmask": "255.255.255.0", "gateway": "127.0.0.21", "type": "public" } ],
				"v6": [ { "ip_address": "2001::14", "cidr": 124, "gateway": "2400:6180:0000:00D0:0000:0000:0009:7000", "type": "public" } ]
			},
			"kernel": {
				"id": 485432986,
				"name": "DO-recovery-static-fsck",
				"version": "3.8.0-25-generic"
			},
			"created_at": "2014-09-05T02:02:05Z",
			"features": [ "ipv6" ],
			"backup_ids": [ ],
			"snapshot_ids": [ ]
		}
	}
	`

	allDroplets = `
	{
		"droplets": [
			{
				"id": 20,
				"name": "test.example.com",
				"memory": 512,
				"vcpus": 1,
				"disk": 20,
				"region": {
					"slug": "nyc1",
					"name": "New York",
					"sizes": [ "1gb", "512mb" ],
					"available": true,
					"features": [ "virtio", "private_networking", "backups", "ipv6" ]
				},
				"image": {
					"id": 119192817,
					"name": "Ubuntu 13.04",
					"distribution": "ubuntu",
					"slug": "ubuntu1304",
					"public": true,
					"regions": [ "nyc1" ],
					"created_at": "2014-09-05T02:02:05Z"
				},
				"size": {
					"slug": "512mb",
					"transfer": 1,
					"price_monthly": 5.0,
					"price_hourly": 0.00744
				},
				"locked": false,
				"status": "active",
				"networks": {
					"v4": [ { "ip_address": "127.0.0.20", "netmask": "255.255.255.0", "gateway": "127.0.0.21", "type": "public" } ],
					"v6": [ { "ip_address": "2001::14", "cidr": 124, "gateway": "2400:6180:0000:00D0:0000:0000:0009:7000", "type": "public" } ]
				},
				"kernel": {
					"id": 485432986,
					"name": "DO-recovery-static-fsck",
					"version": "3.8.0-25-generic"
				},
				"created_at": "2014-09-05T02:02:05Z",
				"features": [ "ipv6" ],
				"backup_ids": [ ],
				"snapshot_ids": [ ]
			},
			{
				"id": 21,
				"name": "test2.example.com",
				"memory": 512,
				"vcpus": 1,
				"disk": 20,
				"region": {
					"slug": "nyc1",
					"name": "New York",
					"sizes": [ "1gb", "512mb" ],
					"available": true,
					"features": [ "virtio", "private_networking", "backups", "ipv6" ]
				},
				"image": {
					"id": 119192818,
					"name": "Ubuntu 13.04",
					"distribution": "ubuntu",
					"slug": "ubuntu1304",
					"public": true,
					"regions": [ "nyc1" ],
					"created_at": "2014-09-05T02:04:05Z"
				},
				"size": {
					"slug": "512mb",
					"transfer": 1,
					"price_monthly": 5.0,
					"price_hourly": 0.00744
				},
				"locked": false,
				"status": "active",
				"networks": {
					"v4": [ { "ip_address": "127.0.0.22", "netmask": "255.255.255.0", "gateway": "127.0.0.21", "type": "public" } ],
					"v6": [ { "ip_address": "2001::15", "netmask": 64, "cidr": 124, "gateway": "2400:6180:0000:00D0:0000:0000:0009:7000", "type": "public" } ]
				},
				"kernel": {
					"id": 485432986,
					"name": "DO-recovery-static-fsck",
					"version": "3.8.0-25-generic"
				},
				"created_at": "2014-09-05T02:04:05Z",
				"features": [ "ipv6" ],
				"backup_ids": [ ],
				"snapshot_ids": [ ]
			},
			{
				"id": 22,
				"name": "tender.nitrousproxy-nyc3.com",
				"memory": 512,
				"vcpus": 1,
				"disk": 20,
				"region": {
					"slug": "nyc1",
					"name": "New York",
					"sizes": [ "1gb", "512mb" ],
					"available": true,
					"features": [ "virtio", "private_networking", "backups", "ipv6" ]
				},
				"image": {
					"id": 119192818,
					"name": "Ubuntu 13.04",
					"distribution": "ubuntu",
					"slug": "ubuntu1304",
					"public": true,
					"regions": [ "nyc1" ],
					"created_at": "2014-09-05T02:04:05Z"
				},
				"size": {
					"slug": "512mb",
					"transfer": 1,
					"price_monthly": 5.0,
					"price_hourly": 0.00744
				},
				"locked": false,
				"status": "active",
				"networks": {
					"v4": [ { "ip_address": "127.0.0.22", "netmask": "255.255.255.0", "gateway": "127.0.0.21", "type": "public" } ],
					"v6": [ { "ip_address": "2001::15", "netmask": 64, "cidr": 124, "gateway": "2400:6180:0000:00D0:0000:0000:0009:7000", "type": "public" } ]
				},
				"kernel": {
					"id": 485432986,
					"name": "DO-recovery-static-fsck",
					"version": "3.8.0-25-generic"
				},
				"created_at": "2014-09-05T02:04:05Z",
				"features": [ "ipv6" ],
				"backup_ids": [ ],
				"snapshot_ids": [ ]
			}
		],
		"links": {},
		"meta": {
    		"total": 3
  		}
	}
	`

	// keys/create
	newSSH = `
	{
		"ssh_key": {
			"id": 2,
			"fingerprint": "3b:16:bf:e4:8b:00:8b:b8:59:8c:a9:d3:f0:19:45:fa",
			"public_key": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAQQDDHr/jh2Jy4yALcK4JyWbVkPRaWmhck3IgCoeOO3z1e2dBowLh64QAM+Qb72pxekALga2oi4GvT+TlWNhzPH4V example",
			"name": "Example Key"
		}
	}
	`

	// /regions
	allRegions = `
	{
		"regions": [
		{
			"slug": "nyc1",
			"name": "New York",
			"sizes": [ ],
			"available": false,
			"features": [ "virtio", "private_networking", "backups", "ipv6" ]
		},
		{
			"slug": "sfo1",
			"name": "San Francisco",
			"sizes": [ "1gb", "512mb" ],
			"available": true,
			"features": [ "virtio", "backups" ]
		},
		{
			"slug": "ams1",
			"name": "Amsterdam",
			"sizes": [ "1gb", "512mb" ],
			"available": true,
			"features": [ "virtio", "backups" ]
		}
		],
		"meta": {
			"total": 3
		}
	}
	`

	// /sizes
	allSizes = `
	{
		"sizes": [
		{
			"slug": "512mb",
			"memory": 512,
			"vcpus": 1,
			"disk": 20,
			"transfer": 1,
			"price_monthly": 5.0,
			"price_hourly": 0.00744,
			"regions": [ "nyc1", "sfo1", "ams1" ]
		},
		{
			"slug": "1gb",
			"memory": 1024,
			"vcpus": 2,
			"disk": 30,
			"transfer": 2,
			"price_monthly": 10.0,
			"price_hourly": 0.01488,
			"regions": [ "nyc1", "sfo1", "ams1" ]
		}
		],
		"meta": {
			"total": 2
		}
	}
	`

	// /image/id
	oneImage = `
	{
		"image": {
			"id": 1,
			"name": "Ubuntu 13.04",
			"distribution": null,
			"slug": null,
			"public": false,
			"regions": [ "sgp1" ],
			"created_at": "2014-09-05T02:02:08Z"
		}
	}
	`
)
