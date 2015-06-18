package digitalocean

import (
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/digitalocean/godo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() { logrus.SetLevel(logrus.PanicLevel) }

func TestDigitalOceanUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/digitalocean")
}

var _ = Describe("MetricsLabel", func() {
	It("returns metrics label of the host", func() {
		var h *Host
		h = &Host{}
		Expect(h.MetricsLabel("wow")).To(Equal("wow.digitalocean"))

		h = &Host{droplet: &godo.Droplet{}}
		Expect(h.MetricsLabel("wow")).To(Equal("wow.digitalocean"))

		h = &Host{droplet: &godo.Droplet{Region: &godo.Region{}}}
		Expect(h.MetricsLabel("wow")).To(Equal("wow.digitalocean"))

		h = &Host{droplet: &godo.Droplet{Region: &godo.Region{Slug: "sgp1"}}}
		Expect(h.MetricsLabel("wow")).To(Equal("wow.digitalocean.sgp1"))
	})
})
