package digitalocean_test

import (
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotest"
	. "github.com/nitrous-io/hostbuilder/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDigitalOceanWait(t *testing.T) {
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

	It("waits for the desired state", func() {
		s.AddResponse("droplets/id", dotest.NewResponse(200, NewDropletState))
		s.AddResponse("droplets/id", dotest.NewResponse(200, ActiveDropletState))

		droplet, err := WaitForState("active", 1, c, 1*time.Millisecond, 1*time.Second)
		Expect(err).To(BeNil())
		Expect(droplet.Status).To(Equal("active"))
	})

	It("should be able to timed out", func() {
		s.AddResponse("droplets/id", dotest.NewResponse(200, NewDropletState))
		s.AddResponse("droplets/id", dotest.NewResponse(200, ActiveDropletState))

		droplet, err := WaitForState("active", 1, c, 1*time.Millisecond, 1*time.Millisecond)
		Expect(err).ToNot(BeNil())
		Expect(droplet).To(BeNil())
	})

	It("fails when we cant get the desired state before timing out", func() {
		s.AddResponse("droplets/id", dotest.NewResponse(200, NewDropletState))
		droplet, err := WaitForState("active", 1, c, 1*time.Millisecond, 5*time.Millisecond)
		Expect(err).ToNot(BeNil())
		Expect(droplet).To(BeNil())
	})
})
