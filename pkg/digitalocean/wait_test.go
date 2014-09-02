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

	Describe("WaitForState", func() {
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

	Describe("WaitForEvent", func() {
		newEventState := `{
			"status": "OK",
			"event": {
				"id": 7499,
				"action_status": "in-progress",
				"droplet_id": 100824,
				"event_type_id": 1,
				"percentage": "80"
			}
		}`

		completedEventState := `{
			"status": "OK",
			"event": {
				"id": 7499,
				"action_status": "done",
				"droplet_id": 100824,
				"event_type_id": 1,
				"percentage": "100"
			}
		}`

		It("waits for the event to be completed", func() {
			s.AddResponse("events/id", dotest.NewResponse(200, newEventState))
			s.AddResponse("events/id", dotest.NewResponse(200, completedEventState))

			err := WaitForEvent(1, c, 1*time.Millisecond, 1*time.Second)
			Expect(err).To(BeNil(), "failed to wait until event is completed")
		})

		It("should be able to timed out", func() {
			s.AddResponse("events/id", dotest.NewResponse(200, newEventState))
			s.AddResponse("events/id", dotest.NewResponse(200, newEventState))
			s.AddResponse("events/id", dotest.NewResponse(200, completedEventState))

			err := WaitForEvent(1, c, 1*time.Millisecond, 1*time.Millisecond)
			Expect(err).ToNot(BeNil(), "failed to timeout the wait loop for event")
		})

		It("fails when the event isnt completed before timing out", func() {
			s.AddResponse("events/id", dotest.NewResponse(200, newEventState))
			err := WaitForEvent(1, c, 1*time.Millisecond, 1*time.Second)
			Expect(err).ToNot(BeNil(), "failed to exit the wait loop when event failed")
		})
	})
})
