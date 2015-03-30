package host_test

import (
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

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
		timeout := 500 * time.Millisecond // Give server more than enough time to respond.

		Expect(host.WaitForServer(server.URL(), timeout, attemptTimes)).To(BeTrue())
		Expect(server.ReceivedRequests()).To(HaveLen(1))
	})

	It("should return false if the server times out", func() {
		clientTimeout := 50 * time.Millisecond
		serverTimeout := 200 * time.Millisecond // Set this high enough so that server never responds.

		timeoutHandler := ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(serverTimeout)
			},
		)

		server := ghttp.NewTLSServer()
		defer server.Close()
		server.AppendHandlers(timeoutHandler, timeoutHandler)

		delay1 := 50 * time.Millisecond
		delay2 := 10 * time.Second
		attemptTimes := []time.Duration{delay1, delay2}

		start := time.Now()
		Expect(host.WaitForServer(server.URL(), clientTimeout, attemptTimes)).To(BeFalse())
		since := time.Since(start)
		Expect(since).To(BeNumerically(">=", 2*clientTimeout+delay1))
		Expect(since).To(BeNumerically("<=", 2*clientTimeout+delay1+delay2))
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
		timeout := 500 * time.Millisecond // Give server more than enough time to respond.

		Expect(host.WaitForServer(server.URL(), timeout, attemptTimes)).To(BeFalse())
		Expect(server.ReceivedRequests()).To(HaveLen(2))
	})
})
