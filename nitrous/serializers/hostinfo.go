package serializers

import "github.com/nitrous-io/dedicated/freighter/freighterclient"

type HostInfo struct {
	Uptime       uint64    `json:"uptime"`
	MemoryUsage  uint64    `json:"memory_usage"`
	MemoryTotal  uint64    `json:"memory_total"`
	StorageUsage uint64    `json:"storage_usage"`
	StorageTotal uint64    `json:"storage_total"`
	InodeUsage   uint64    `json:"inode_usage"`
	InodeTotal   uint64    `json:"inode_total"`
	NumCPU       int       `json:"num_cpu"`
	PerCPUUsage  []float64 `json:"per_cpu_usage"`
}

func SerializeHostInfo(info *freighterclient.HostInfo) *HostInfo {
	if info == nil {
		return nil
	}
	return &HostInfo{
		Uptime:       info.Uptime,
		MemoryUsage:  info.MemoryUsage,
		MemoryTotal:  info.MemoryTotal,
		StorageUsage: info.StorageUsage,
		StorageTotal: info.StorageTotal,
		InodeUsage:   info.InodeUsage,
		InodeTotal:   info.InodeTotal,
		NumCPU:       info.NumCPU,
		PerCPUUsage:  info.PerCPUUsage,
	}
}
