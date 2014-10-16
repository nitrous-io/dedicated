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

		Expect(host.WaitForServer(server.URL(), 100*time.Millisecond, attemptTimes)).To(BeTrue())
		Expect(server.ReceivedRequests()).To(HaveLen(1))
	})

	It("should return false if the server times out", func(done Done) {
		clientTimeout := 100

		timeoutHandler := ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(time.Duration(clientTimeout+1) * time.Millisecond)
			},
		)

		server := ghttp.NewTLSServer()
		defer server.Close()
		server.AppendHandlers(timeoutHandler, timeoutHandler)

		attemptTimes := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}

		start := time.Now()
		Expect(host.WaitForServer(server.URL(), time.Duration(clientTimeout)*time.Millisecond, attemptTimes)).To(BeFalse())
		finished := time.Now()
		Expect(finished).To(BeTemporally(">=", start.Add(time.Duration(2*clientTimeout+1*50)*time.Millisecond)))
		Expect(finished).To(BeTemporally("<=", start.Add(time.Duration(2*clientTimeout+2*50)*time.Millisecond)))
		Expect(server.ReceivedRequests()).To(HaveLen(2))
		close(done)
	})

	It("should return false if the server is up but does not return 200", func() {
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
