package freighterclienttools_test

import (
	"database/sql"
	"testing"

	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/freighterclienttools"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "freighterclienttools")
}

var _ = Describe("freighterclienttools", func() {
	Describe("MakeFreighterHost()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			h = &host.Host{
				UserID:         2,
				Slug:           "my-host-1",
				FreighterdPort: sql.NullInt64{int64(1907), true},
			}
		})

		Context("when a host lacks an IPAddress", func() {
			BeforeEach(func() {
				h.IPAddress = sql.NullString{"", true}
			})

			It("should return a ErrHostIpUnknown error", func() {
				frClientHost, err := freighterclienttools.MakeFreighterHost(h, false)
				Expect(frClientHost).To(BeNil())
				Expect(err).To(Equal(freighterclienttools.ErrHostIPUnknown))
			})
		})

		Context("when a host has an IPAddress", func() {
			BeforeEach(func() {
				h.IPAddress = sql.NullString{"192.168.1.254", true}
			})

			Context("with secure passed as false", func() {
				It("should return a freighterClient.Host with secure set to false", func() {
					frClientHost, err := freighterclienttools.MakeFreighterHost(h, false)
					Expect(err).To(BeNil())
					Expect(frClientHost.Host).To(Equal("192.168.1.254"))
					Expect(frClientHost.Port).To(Equal(1907))
					Expect(frClientHost.Secure).To(BeFalse())
				})
			})

			Context("with secure passed as true", func() {
				It("should return a freighterClient.Host with secure set to true", func() {
					frClientHost, err := freighterclienttools.MakeFreighterHost(h, true)
					Expect(err).To(BeNil())
					Expect(frClientHost.Host).To(Equal("192.168.1.254"))
					Expect(frClientHost.Port).To(Equal(1907))
					Expect(frClientHost.Secure).To(BeTrue())
				})
			})
		})
	})
})
