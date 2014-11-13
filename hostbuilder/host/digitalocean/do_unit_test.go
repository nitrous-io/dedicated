package digitalocean

import (
	"testing"

	"github.com/digitalocean/godo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDigitalOceanUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/digitalocean")
}

var _ = Describe("metricsLabel", func() {
	It("returns metrics label of the host", func() {
		var h *Host
		h = &Host{}
		Expect(h.metricsLabel("wow")).To(Equal("wow.digitalocean"))

		h = &Host{droplet: &godo.Droplet{}}
		Expect(h.metricsLabel("wow")).To(Equal("wow.digitalocean"))

		h = &Host{droplet: &godo.Droplet{Region: &godo.Region{}}}
		Expect(h.metricsLabel("wow")).To(Equal("wow.digitalocean"))

		h = &Host{droplet: &godo.Droplet{Region: &godo.Region{Slug: "sgp1"}}}
		Expect(h.metricsLabel("wow")).To(Equal("wow.digitalocean.sgp1"))
	})
})
