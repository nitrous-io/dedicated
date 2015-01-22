package server_test

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "middleware")
}

var _ = Describe("server", func() {
	var r *http.Request
	var err error

	BeforeEach(func() {
		r, err = NewRequest("GET", "/test", nil)
		Expect(err).To(BeNil())
	})

	Describe("Start", func() {
		It("starts a HTTP server listening on the given host and port", func() {
			db, err := TestDatabase("nitrous_test")
			Expect(err).To(BeNil())
			logger := &logger.Logger{Level: logger.LogLevelQuiet}
			svr := server.NewServer("", db, logger, nil)
			defer svr.Stop()

			go svr.Start("127.0.0.1", uint16(0)) // port=0 assigns random port.
			time.Sleep(10 * time.Millisecond)    // Sadly, we need to sleep here for server to start.

			listenAddr := svr.Listener().Addr().String()
			Expect(listenAddr).To(MatchRegexp(`127.0.0.1:\d+`))
			conn, err := net.Dial("tcp", listenAddr)
			Expect(err).To(BeNil())
			conn.Close()
		})
	})

	Describe("Stop", func() {
		It("stops the HTTP server and host provisioner", func() {
			db, err := TestDatabase("nitrous_test")
			Expect(err).To(BeNil())
			logger := &logger.Logger{Level: logger.LogLevelQuiet}
			svr := server.NewServer("", db, logger, nil)

			errCh := make(chan error)
			go func() {
				errCh <- svr.Start("127.0.0.1", uint16(0)) // port=0 assigns random port.
			}()
			time.Sleep(10 * time.Millisecond)

			// Check that we can connect to the server and host provisioner is
			// running.
			listenAddr := svr.Listener().Addr().String()
			conn, err := net.Dial("tcp", listenAddr)
			Expect(err).To(BeNil())
			conn.Close()

			svr.Stop()

			err = <-errCh
			Expect(err).To(HaveOccurred())
			conn, err = net.Dial("tcp", listenAddr)
			Expect(err).To(HaveOccurred())
		})
	})
})
