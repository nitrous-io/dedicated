package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type Host struct {
	ID                        int64  `json:"id"`
	UserID                    int64  `json:"user_id"`
	RegionID                  int64  `json:"region_id"`
	PlanID                    int64  `json:"plan_id"`
	OrgID                     *int64 `json:"org_id"`
	Slug                      string `json:"slug"`
	Name                      string `json:"name"`
	IPAddress                 string `json:"ip_address"`
	StaticIP                  bool   `json:"static_ip"`
	FreighterdPort            int64  `json:"freighterd_port"`
	FreighterdVersion         string `json:"freighterd_version"`
	DockerVersion             string `json:"docker_version"`
	ProviderIdentifier        string `json:"provider_identifier"`
	ProviderStorageIdentifier string `json:"provider_storage_identifier"`
	Hotswap                   bool   `json:"hotswap"`
	Hostname                  string `json:"hostname"`       // where we can access the host i.e. slug.nitrouspro.com
	ProxyHostname             string `json:"proxy_hostname"` // slug.nitrousproxy.com
	Size                      string `json:"size"`
	State                     string `json:"state"`
	Username                  string `json:"username,omitempty"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`

	LastRunningAt *time.Time `json:"last_running_at,omitempty"`
	LastPingedAt  *time.Time `json:"last_pinged_at,omitempty"`
	AutoShutdown  bool       `json:"auto_shutdown"`

	MaxContainers int64 `json:"max_containers,omitempty"`

	PendingRespec bool `json:"pending_respec"`

	SubscriptionSlug string `json:"subscription_slug,omitempty"`
	Free             bool   `json:"free"`

	Shared bool `json:"shared"`
}

type HostWithUser struct {
	Host *Host `json:"host,omitempty"`

	Username     string `json:"username,omitempty"`
	GravatarHash string `json:"gravatar_hash,omitempty"`
}

func SerializeHost(h *host.Host) *Host {
	if h == nil {
		return nil
	}
	serialized := &Host{
		ID:                        h.ID,
		UserID:                    h.UserID,
		RegionID:                  h.RegionID,
		PlanID:                    h.PlanID,
		Slug:                      h.Slug,
		Name:                      h.Name.String,
		IPAddress:                 h.IPAddress.String,
		StaticIP:                  h.StaticIP,
		FreighterdPort:            h.FreighterdPort.Int64,
		FreighterdVersion:         h.FreighterdVersion,
		DockerVersion:             h.DockerVersion,
		ProviderIdentifier:        h.ProviderIdentifier.String,
		ProviderStorageIdentifier: h.ProviderStorageIdentifier.String,
		Hotswap:                   h.Hotswap,
		Size:                      h.Size.String,
		State:                     h.State,
		Hostname:                  h.Hostname(),
		ProxyHostname:             h.ProxyHostname(),
		CreatedAt:                 h.CreatedAt,
		UpdatedAt:                 h.UpdatedAt,
		TerminatedAt:              h.TerminatedAt,
		AutoShutdown:              h.AutoShutdown,
		MaxContainers:             h.MaxContainers.Int64,
		PendingRespec:             h.PendingRespec,
		SubscriptionSlug:          h.SubscriptionSlug.String,
		Free:                      h.Free,
		Shared:                    h.Shared,
	}
	if h.AssignedAt != nil {
		serialized.CreatedAt = h.AssignedAt
	}
	if h.LastRunningAt.Valid {
		serialized.LastRunningAt = &(h.LastRunningAt.Time)
	}
	if h.Username.Valid {
		serialized.Username = h.Username.String
	}
	if h.LastPingedAt.Valid {
		serialized.LastPingedAt = &(h.LastPingedAt.Time)
	}
	if h.OrgID.Valid {
		serialized.OrgID = &h.OrgID.Int64
	} else {
		serialized.OrgID = nil
	}
	return serialized
}

func SerializeHosts(hosts []*host.Host) []*Host {
	ret := make([]*Host, len(hosts))
	for i, h := range hosts {
		ret[i] = SerializeHost(h)
	}
	return ret
}

func SerializeUserHosts(hosts []*host.HostWithUser) []*HostWithUser {
	ret := make([]*HostWithUser, len(hosts))
	for i, h := range hosts {
		serHost := SerializeHost(h.Host)
		ret[i] = &HostWithUser{
			Host: serHost,
			Username: h.Username,
			GravatarHash: generateGravatarHash(h.UserEmail),
		}
	}
	return ret
}
