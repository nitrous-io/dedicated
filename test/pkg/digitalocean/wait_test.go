package digitalocean

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

	Describe("State Success", func() {
		It("should wait for success", func() {
			s.AddResponse("droplets/id", dotest.NewResponse(200, NewDropletState))
			s.AddResponse("droplets/id", dotest.NewResponse(200, ActiveDropletState))

			droplet, err := WaitForState("active", 1, c, 1*time.Millisecond, 1*time.Second)
			Expect(err).To(BeNil())
			Expect(droplet.Status).To(Equal("active"))
		})
	})

	Describe("State Timeout", func() {
		It("should timeout", func() {
			s.AddResponse("droplets/id", dotest.NewResponse(200, NewDropletState))
			s.AddResponse("droplets/id", dotest.NewResponse(200, ActiveDropletState))

			droplet, err := WaitForState("active", 1, c, 1*time.Millisecond, 1*time.Millisecond)
			Expect(err).ToNot(BeNil())
			Expect(droplet).To(BeNil())
		})
	})

	Describe("State Failed", func() {
		It("should fail", func() {
			s.AddResponse("droplets/id", dotest.NewResponse(200, NewDropletState))
			droplet, err := WaitForState("active", 1, c, 1*time.Millisecond, 5*time.Millisecond)
			Expect(err).ToNot(BeNil())
			Expect(droplet).To(BeNil())
		})
	})
})
