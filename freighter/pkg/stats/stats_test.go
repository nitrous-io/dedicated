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
			Expect(stats.InodeUsage).NotTo(BeZero())
			Expect(stats.InodeTotal).NotTo(BeZero())
			Expect(stats.NumCPU).To(Equal(runtime.NumCPU()))
			Expect(stats.PerCPUUsage).To(HaveLen(runtime.NumCPU()))
		})
	})

	Describe("Utilization", func() {
		var (
			util *stats.Utilization
		)

		BeforeEach(func() {
			util = stats.NewUtilization(5)
		})

		It("should increment the index when adding an item", func() {
			Expect(util.Index).To(Equal(0))
			util.RecordSample(time.Now(), &stats.Stats{})
			Expect(util.Index).To(Equal(1))
		})

		It("should roll over the index when reaching the end", func() {
			Expect(util.Index).To(Equal(0))
			util.RecordSample(time.Now(), &stats.Stats{})
			util.RecordSample(time.Now(), &stats.Stats{})
			util.RecordSample(time.Now(), &stats.Stats{})
			util.RecordSample(time.Now(), &stats.Stats{})
			util.RecordSample(time.Now(), &stats.Stats{})
			Expect(util.Index).To(Equal(0))
		})

		It("should fetch the added items", func() {
			time1, _ := time.Parse("2006-01-02", "2014-12-30")
			time2, _ := time.Parse("2006-01-02", "2004-12-31")
			stats1 := &stats.Stats{Uptime: 5}
			stats2 := &stats.Stats{Uptime: 10}
			util.RecordSample(time1, stats1)
			util.RecordSample(time2, stats2)
			fetch := util.Fetch()
			Expect(fetch).To(HaveLen(2))
			Expect(fetch[0].Time).To(Equal(time1))
			Expect(fetch[0].Stats).To(Equal(stats1))
			Expect(fetch[1].Time).To(Equal(time2))
			Expect(fetch[1].Stats).To(Equal(stats2))
		})
	})
})
