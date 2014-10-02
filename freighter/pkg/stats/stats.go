package stats

import (
	"syscall"
	"time"

	"github.com/c9s/goprocinfo/linux"
)

type Stats struct {
	// Uptime in seconds.
	Uptime uint64 `json:"uptime"`

	// MemoryUsage in bytes.
	MemoryUsage uint64 `json:"memory_usage"`
	// MemoryTotal in bytes.
	MemoryTotal uint64 `json:"memory_total"`

	// StorageUsage in bytes.
	StorageUsage uint64 `json:"storage_usage"`
	// StorageTotal in bytes.
	StorageTotal uint64 `json:"storage_total"`

	NumCPU int `json:"num_cpu"`
	// Percent usage per CPU.
	PerCPUUsage []float64 `json:"per_cpu_usage"`
}

var Sample = func(period time.Duration) (*Stats, error) {
	stats := &Stats{}

	sys := &syscall.Sysinfo_t{}
	err := syscall.Sysinfo(sys)
	if err != nil {
		return nil, err
	}
	unit := uint64(sys.Unit) // Sysinfo_t.Unit is the memory unit size in bytes.
	stats.Uptime = uint64(sys.Uptime)
	stats.MemoryUsage = (uint64(sys.Totalram) - uint64(sys.Freeram)) / unit
	stats.MemoryTotal = uint64(sys.Totalram) / unit

	statfs := &syscall.Statfs_t{}
	syscall.Statfs("/", statfs) // Assume root volume is the volume made available to users.
	total := statfs.Blocks * uint64(statfs.Bsize)
	used := total - (statfs.Bfree * uint64(statfs.Bsize))
	stats.StorageUsage = used
	stats.StorageTotal = total

	errch := make(chan error)
	usagech := make(chan []float64)
	go func() {
		usage, err := cpuUsage(period)
		if err != nil {
			errch <- err
			return
		}
		usagech <- usage
	}()

	select {
	case usage := <-usagech:
		stats.NumCPU = len(usage)
		stats.PerCPUUsage = usage
	case err := <-errch:
		return nil, err
	}

	return stats, nil
}

type cpuSample struct {
	idle  uint64
	total uint64
}

// sampleCPU gets the idle and total ticks for each CPU.
// This parses the /proc/stat file (documented here: https://www.kernel.org/doc/Documentation/filesystems/proc.txt).
func sampleCPU() ([]cpuSample, error) {
	stat, err := linux.ReadStat("/proc/stat")
	if err != nil {
		return nil, err
	}
	s := make([]cpuSample, len(stat.CPUStats))
	for i, c := range stat.CPUStats {
		s[i].total = c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ +
			c.SoftIRQ + c.Steal + c.Guest + c.GuestNice
		s[i].idle = c.Idle + c.IOWait
	}
	return s, nil
}

// cpuUsage computes per-CPU usage over the given period as percentages.
func cpuUsage(period time.Duration) ([]float64, error) {
	start, err := sampleCPU()
	if err != nil {
		return nil, err
	}
	time.Sleep(period)
	end, err := sampleCPU()
	if err != nil {
		return nil, err
	}
	usage := make([]float64, len(start))
	for i := range start {
		idleTicks := float64(end[i].idle - start[i].idle)
		totalTicks := float64(end[i].total - start[i].total)
		usage[i] = (totalTicks - idleTicks) * 100 / totalTicks
	}
	return usage, nil
}
