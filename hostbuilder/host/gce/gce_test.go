package gce

import (
	"io/ioutil"
	"net/http"
	"strings"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ipAddress", func() {
	It("returns the first valid public ip of the instance", func() {
		n := []*compute.NetworkInterface{
			&compute.NetworkInterface{AccessConfigs: nil},
			&compute.NetworkInterface{
				AccessConfigs: []*compute.AccessConfig{
					&compute.AccessConfig{NatIP: "127.0.0.1"},
				},
			},
		}
		h := &Host{instance: &compute.Instance{NetworkInterfaces: n}}

		Expect(h.ipAddress()).To(Equal("127.0.0.1"), "invalid ip address")
	})

	It("returns nothing when instance doesnt have public ip", func() {
		n := []*compute.NetworkInterface{
			&compute.NetworkInterface{AccessConfigs: nil},
		}
		h := &Host{instance: &compute.Instance{NetworkInterfaces: n}}

		Expect(h.ipAddress()).To(BeEmpty(), "unexpected ip address")
	})
})

var _ = Describe("parseHostConfig", func() {
	It("sets DiskSize of the HostConfig correctly", func() {
		c, err := parseHostConfig(`{ "disk_size": "20" }`)
		Expect(err).To(BeNil(), "unexpected error")
		Expect(c.DiskSizeRaw).To(Equal("20"))
		Expect(c.DiskSize).To(Equal(int64(20)))

		_, err = parseHostConfig(`{ "disk_size": "what" }`)
		Expect(err).ToNot(BeNil(), "expect error for invalid disk size")
	})
})

var _ = Describe("metadata", func() {
	It("sets SSH keys in the metadata if provided", func() {
		var h *Host
		var m []*compute.MetadataItems

		// metadata will be empty by default
		h = &Host{Config: &HostConfig{}}
		m = h.metadata([]string{})
		Expect(m).To(BeEmpty(), "unexpected metadata")

		// add HostConfig.Metadata
		h = &Host{Config: &HostConfig{
			Metadata: map[string]string{"hello": "world"},
		}}
		m = h.metadata([]string{})
		Expect(m).To(ConsistOf(
			&compute.MetadataItems{Key: "hello", Value: "world"},
		), "unexpected metadata")

		// add ssh keys as additional metadata
		h = &Host{Config: &HostConfig{
			Metadata: map[string]string{"hello": "world"},
		}}
		m = h.metadata([]string{"ssh-key"})
		Expect(m).To(ConsistOf(
			&compute.MetadataItems{Key: "hello", Value: "world"},
			&compute.MetadataItems{Key: "sshKeys", Value: "root:ssh-key\n"},
		), "unexpected metadata")
	})
})

var _ = Describe("Info", func() {
	It("returns info of an instance", func() {
		service, _ := compute.New(&http.Client{
			Transport: &FakeRoundTripper{
				message: getInstanceResponse,
				status:  http.StatusOK,
			},
		})
		h := &Host{
			service: service,
			Config: &HostConfig{
				RegionID: "asia-east1-c",
				ID:       "id-should-be-from-response",
			},
		}

		i, err := h.Info()
		Expect(err).To(BeNil(), "unexpected error")
		Expect(i).To(Equal(&host.Info{
			Provider:       "gce",
			ID:             "supersonic-harrenhal-host-55",
			IPAddress:      "130.211.240.56",
			FreighterdPort: uint16(65333),
			RegionID:       "asia-east1-c",
		}), "unexpected instance info")
	})
})

// FakeRoundTripper implements a fake Transport layer that records the executed
// requests and provide a fake response
//
// This is more lightweight than implementing a full HTTP server and should be
// approriate for some tests
type FakeRoundTripper struct {
	message  string
	status   int
	header   map[string]string
	requests []*http.Request
}

func (rt *FakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	body := strings.NewReader(rt.message)
	rt.requests = append(rt.requests, r)
	res := &http.Response{
		StatusCode: rt.status,
		Body:       ioutil.NopCloser(body),
		Header:     make(http.Header),
	}
	for k, v := range rt.header {
		res.Header.Set(k, v)
	}
	return res, nil
}

func (rt *FakeRoundTripper) Reset() {
	rt.requests = nil
}
