package serializers

import (
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/host"
)

type Host struct {
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

	Region *Region   `json:"region,omitempty"`
	Plan   *Plan     `json:"plan,omitempty"`
	Stats  *HostInfo `json:"stats,omitempty"`
}

func SerializeHost(h *host.Host, info *freighterclient.HostInfo) *Host {
	if h == nil {
		return nil
	}
	ret := &Host{
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
	ret.Region = SerializeRegion(h.Region)
	ret.Plan = SerializePlan(h.Plan)
	ret.Stats = SerializeHostInfo(info)
	return ret
}
