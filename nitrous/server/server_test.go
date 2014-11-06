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
			svr := server.NewServer("", db, logger)
			defer svr.Stop()

			go svr.Start("127.0.0.1", uint16(0)) // port=0 assigns random port.
			time.Sleep(10 * time.Millisecond)    // Sadly, we need to sleep here for server to start.

			listenAddr := svr.Listener().Addr().String()
			Expect(listenAddr).To(MatchRegexp(`127.0.0.1:\d+`))
			conn, err := net.Dial("tcp", listenAddr)
			Expect(err).To(BeNil())
			conn.Close()
		})

		It("starts a host provisioner", func() {
			db, err := TestDatabase("nitrous_test")
			Expect(err).To(BeNil())
			logger := &logger.Logger{Level: logger.LogLevelQuiet}
			svr := server.NewServer("", db, logger)
			defer svr.Stop()

			go svr.Start("127.0.0.1", uint16(0))
			time.Sleep(10 * time.Millisecond)

			Expect(svr.Provisioner.IsRunning()).To(BeTrue())
		})
	})

	Describe("Stop", func() {
		It("stops the HTTP server and host provisioner", func() {
			db, err := TestDatabase("nitrous_test")
			Expect(err).To(BeNil())
			logger := &logger.Logger{Level: logger.LogLevelQuiet}
			svr := server.NewServer("", db, logger)

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
			Expect(svr.Provisioner.IsRunning()).To(BeTrue())

			svr.Stop()

			err = <-errCh
			Expect(err).To(HaveOccurred())
			conn, err = net.Dial("tcp", listenAddr)
			Expect(err).To(HaveOccurred())

			Expect(svr.Provisioner.IsRunning()).To(BeFalse())
		})
	})

	Describe("getRemoteAddr", func() {
		Context("with RemoteAddr set to IP:PORT", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10:63333"
			})

			It("should return the IP", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr set to multiple IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10:63333 202.157.130.9 202.157.130.202:60000"
			})

			It("should return the first IP address", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr set to multiple IP addresses some of which are 'proxy' IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000"
			})

			It("should return the IP address which is not a proxy", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.202"))
			})
		})

		Context("with RemoteAddr is not set, but X-Forwarded-For is set", func() {
			BeforeEach(func() {
				r.RemoteAddr = ""
				r.Header.Set("X-Forwarded-For", "202.157.130.10")
			})

			It("should return the IP address in the X-Forwarded-For header", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr is not set, but X-Forwarded-For is set to multiple IP addresses, some of which are 'trusted' IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = ""
				r.Header.Set("X-Forwarded-For", "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000")
			})

			It("should return the IP address in the X-Forwarded-For header", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.202"))
			})
		})

		Context("with both RemoteAddr and X-Forwarded-For are set", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10"
				r.Header.Set("X-Forwarded-For", "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000")
			})

			It("should return the IP address stored in RemoteAddr", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})
	})
})
