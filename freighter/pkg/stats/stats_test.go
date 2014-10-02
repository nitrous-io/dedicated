package stats_test

import (
	"runtime"

	"github.com/nitrous-io/dedicated/freighter/pkg/stats"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
	"time"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "stats")
}

var _ = Describe("stats", func() {
	Describe(".Sample()", func() {
		It("returns info and stats about the host", func() {
			stats, err := stats.Sample(50 * time.Millisecond)
			Expect(err).To(BeNil())
			Expect(stats.Uptime).NotTo(BeZero())
			Expect(stats.MemoryUsage).NotTo(BeZero())
			Expect(stats.MemoryTotal).NotTo(BeZero())
			Expect(stats.StorageUsage).NotTo(BeZero())
			Expect(stats.StorageTotal).NotTo(BeZero())
			Expect(stats.NumCPU).To(Equal(runtime.NumCPU()))
			Expect(stats.PerCPUUsage).To(HaveLen(runtime.NumCPU()))
		})
	})
})
