package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("host info serializer", func() {
	Describe("SerializeHostInfo", func() {
		var model = &freighterclient.HostInfo{
			Uptime:       42,
			MemoryUsage:  512000,
			MemoryTotal:  1024000,
			StorageUsage: 1000000,
			StorageTotal: 10000000,
			InodeUsage:   1000000,
			InodeTotal:   10000000,
			NumCPU:       2,
			PerCPUUsage:  []float64{1.5, 80.0},
		}

		It("serializes to JSON", func() {
			info := serializers.SerializeHostInfo(model)
			b, err := json.Marshal(info)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"uptime": 42,
				"memory_usage": 512000,
				"memory_total": 1024000,
				"storage_usage": 1000000,
				"storage_total": 10000000,
				"inode_usage": 1000000,
				"inode_total": 10000000,
				"num_cpu": 2,
				"per_cpu_usage": [1.5, 80.0]
			}`))
		})
	})
})
