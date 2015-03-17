package host_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestDigitalOcean(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host")
}

var _ = Describe("WaitForServer", func() {
	It("should immediately return true if no attempts are required", func(done Done) {
		Expect(host.WaitForServer("", 100*time.Millisecond, []time.Duration{})).To(BeTrue())
		close(done)
	}, 0.2)

	It("should return true if the server is up and returns 200", func() {
		server := ghttp.NewTLSServer()
		defer server.Close()
		server.AppendHandlers(
			ghttp.VerifyRequest("GET", "/"),
		)

		attemptTimes := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}

		Expect(host.WaitForServer(server.URL(), 200*time.Millisecond, attemptTimes)).To(BeTrue())
		Expect(server.ReceivedRequests()).To(HaveLen(1))
	})

	// The following tests are marked as Pending to avoid the tests failing during high CI load
	// due to resource contention. When modifying `WaitForServer` remove the Pending state and
	// test yourself.

	PIt("should return false if the server times out", func() {
		clientTimeout := 50 * time.Millisecond
		serverTimeout := 100 * time.Millisecond

		timeoutHandler := ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(serverTimeout)
			},
		)

		server := ghttp.NewTLSServer()
		defer server.Close()
		server.AppendHandlers(timeoutHandler, timeoutHandler)

		attemptTimes := []time.Duration{10 * time.Millisecond, 10 * time.Second}

		start := time.Now()
		Expect(host.WaitForServer(server.URL(), clientTimeout, attemptTimes)).To(BeFalse())
		since := time.Since(start)
		Expect(since).To(BeNumerically(">=", 2*clientTimeout+10*time.Millisecond))
		Expect(since).To(BeNumerically("<=", 2*clientTimeout+10*time.Millisecond+10*time.Second))
		Expect(server.ReceivedRequests()).To(HaveLen(2))
	})

	PIt("should return false if the server is up but does not return 200", func() {
		unavailableHandler := ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			ghttp.RespondWith(http.StatusServiceUnavailable, ""),
		)

		server := ghttp.NewTLSServer()
		defer server.Close()
		server.AppendHandlers(unavailableHandler, unavailableHandler)

		attemptTimes := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}

		Expect(host.WaitForServer(server.URL(), 100*time.Millisecond, attemptTimes)).To(BeFalse())
		Expect(server.ReceivedRequests()).To(HaveLen(2))
	})
})
