package controllers

import (
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
)

type ContainerAPI struct {
	ID        int64  `json:"id"`
	CID       string `json:"cid"`
	UserID    int64  `json:"user_id"`
	Image     string `json:"image"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	State     string `json:"state"`

	Host *HostAPI `json:"host"`
}

type HostAPI struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int    `json:"freighterd_port"`
	Hostname       string `json:"hostname"` // where we can access the host i.e. slug.nitrouspro.com
	Size           string `json:"size"`
	State          string `json:"state"`

	// tls authentication settings for a client
	CA   string `json:"ca,omitempty"`
	Cert string `json:"cert,omitempty"`
	Key  string `json:"key,omitempty"`

	Region *RegionAPI   `json:"region,omitempty"`
	Plan   *PlanAPI     `json:"plan,omitempty"`
	Stats  *HostInfoAPI `json:"stats,omitempty"`
}

type RegionAPI struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PlanAPI struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type HostInfoAPI struct {
	Uptime       uint64    `json:"uptime"`
	MemoryUsage  uint64    `json:"memory_usage"`
	MemoryTotal  uint64    `json:"memory_total"`
	StorageUsage uint64    `json:"storage_usage"`
	StorageTotal uint64    `json:"storage_total"`
	NumCPU       int       `json:"num_cpu"`
	PerCPUUsage  []float64 `json:"per_cpu_usage"`
}

func NewSerializableContainer(c *container.Container) *ContainerAPI {
	return &ContainerAPI{
		ID:        c.ID,
		CID:       c.CID,
		UserID:    c.UserID,
		Image:     c.Image,
		Slug:      c.Slug,
		Name:      c.Name.String,
		IPAddress: c.IPAddress.String,
		State:     c.State,
		Host:      NewSerializableHost(c.Host, nil),
	}
}

func NewSerializableHost(h *host.Host, info *freighterclient.HostInfo) *HostAPI {
	if h == nil {
		return nil
	}
	ret := &HostAPI{
		ID:             h.ID,
		Slug:           h.Slug,
		IPAddress:      h.IPAddress.String,
		FreighterdPort: int(h.FreighterdPort.Int64),
		Size:           h.Size.String,
		State:          h.State,
		Hostname:       h.Hostname(),
	}
	if h.Cert != nil {
		ret.CA = h.Cert.CA
		ret.Cert = h.Cert.ClientCert
		ret.Key = h.Cert.ClientKey
	}
	ret.Region = NewSerializableRegion(h.Region)
	ret.Plan = NewSerializablePlan(h.Plan)
	ret.Stats = NewSerializableHostInfo(info)
	return ret
}

func NewSerializablePlan(p *plan.Plan) *PlanAPI {
	if p == nil {
		return nil
	}
	return &PlanAPI{
		ID:   p.ID,
		Name: p.Name,
	}
}

func NewSerializableRegion(r *region.Region) *RegionAPI {
	if r == nil {
		return nil
	}
	return &RegionAPI{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
	}
}

func NewSerializableHostInfo(info *freighterclient.HostInfo) *HostInfoAPI {
	if info == nil {
		return nil
	}
	return &HostInfoAPI{
		Uptime:       info.Uptime,
		MemoryUsage:  info.MemoryUsage,
		MemoryTotal:  info.MemoryTotal,
		StorageUsage: info.StorageUsage,
		StorageTotal: info.StorageTotal,
		NumCPU:       info.NumCPU,
		PerCPUUsage:  info.PerCPUUsage,
	}
}
