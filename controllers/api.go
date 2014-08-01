package controllers

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrHostIPUnknown = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
)

type JSONResponse struct {
	Code int
	Data map[string]interface{}
}

type ContainerAPI struct {
	ID        int64  `json:"id"`
	CID       string `json:"cid"`
	UserID    int64  `json:"user_id"`
	Image     string `json:"image"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	State     string `json:"state"`

	Host HostAPI `json:"host"`
}

type HostAPI struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int64  `json:"freighterd_port"`
	Hostname       string `json:"hostname"` // where we can access the host i.e. slug.nitrouspro.com

	// tls authentication settings for a client
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type HostDetailedAPI struct {
	ID             int64      `json:"id"`
	Slug           string     `json:"slug"`
	IPAddress      string     `json:"ip_address"`
	FreighterdPort int64      `json:"freighterd_port"`
	State          string     `json:"state"`
	Hostname       string     `json:"hostname"`
	Region         *RegionAPI `json:"region"`
	Plan           *PlanAPI   `json:"plan"`
	Stats          *StatsAPI  `json:"stats"`
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

type StatsAPI struct {
	Uptime       uint64    `json:"uptime"`
	MemoryUsage  uint64    `json:"memory_usage"`
	MemoryTotal  uint64    `json:"memory_total"`
	StorageUsage uint64    `json:"storage_usage"`
	StorageTotal uint64    `json:"storage_total"`
	NumCPU       int       `json:"num_cpu"`
	PerCPUUsage  []float64 `json:"per_cpu_usage"`
}

func NewContainerAPI(c *container.Container, h *host.Host, crt *cert.Cert) ContainerAPI {
	return ContainerAPI{
		ID:        c.ID,
		CID:       c.CID,
		UserID:    c.UserID,
		Image:     c.Image,
		Slug:      c.Slug,
		Name:      c.Name.String,
		IPAddress: c.IPAddress.String,
		State:     c.State,
		Host: HostAPI{
			ID:             h.ID,
			Slug:           h.Slug,
			IPAddress:      h.IPAddress.String,
			FreighterdPort: h.FreighterdPort.Int64,
			Hostname:       h.Hostname(),

			CA:   crt.CA,
			Cert: crt.ClientCert,
			Key:  crt.ClientKey,
		},
	}
}

func NewHostDetailedAPI(h *host.Host, r *region.Region, p *plan.Plan, stats *freighterclient.HostInfo) *HostDetailedAPI {
	hDict := &HostDetailedAPI{
		ID:             h.ID,
		Slug:           h.Slug,
		IPAddress:      h.IPAddress.String,
		FreighterdPort: h.FreighterdPort.Int64,
		State:          h.State,
		Hostname:       h.Hostname(),
	}
	if r != nil {
		hDict.Region = &RegionAPI{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
		}
	}
	if p != nil {
		hDict.Plan = &PlanAPI{
			ID:   p.ID,
			Name: p.Name,
		}
	}
	if stats != nil {
		hDict.Stats = &StatsAPI{
			Uptime:       stats.Uptime,
			MemoryUsage:  stats.MemoryUsage,
			MemoryTotal:  stats.MemoryTotal,
			StorageUsage: stats.StorageUsage,
			StorageTotal: stats.StorageTotal,
			NumCPU:       stats.NumCPU,
			PerCPUUsage:  stats.PerCPUUsage,
		}
	}
	return hDict
}
