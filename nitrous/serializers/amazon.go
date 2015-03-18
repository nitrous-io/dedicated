package serializers

import (
	"github.com/nitrous-io/dedicated/hostbuilder"
	"github.com/nitrous-io/dedicated/nitrous/models/amazon"
)

type LiveHost struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort uint16 `json:"freighterd_port"`
	State          string `json:"state"`
}

type HostEntry struct {
	DBHost   *AdminHost `json:"db_host,omitempty"`
	LiveHost *LiveHost  `json:"live_host,omitempty"`
}

type HostList struct {
	AllHosts          []*HostEntry `json:"all_hosts"`
	HealthyHosts      []*HostEntry `json:"healthy_hosts"`
	InconsistentHosts []*HostEntry `json:"inconsistent_hosts"`
	DanglingHosts     []*AdminHost `json:"dangling_hosts"`
	UnaccountedHosts  []*LiveHost  `json:"unaccounted_hosts"`
}

func SerializeLiveHost(h *hostbuilder.Response) *LiveHost {
	if h == nil {
		return nil
	}

	return &LiveHost{
		Provider:       h.Provider,
		ID:             h.ID,
		Name:           h.Name,
		IPAddress:      h.IPAddress,
		FreighterdPort: h.FreighterdPort,
		State:          h.State,
	}
}

func SerializeLiveHosts(liveHosts []*hostbuilder.Response) []*LiveHost {
	res := make([]*LiveHost, len(liveHosts))
	for i, liveHost := range liveHosts {
		res[i] = SerializeLiveHost(liveHost)
	}
	return res
}

func SerializeHostEntry(entry *groupedhosts.HostEntry) *HostEntry {
	return &HostEntry{
		LiveHost: SerializeLiveHost(entry.LiveHost),
		DBHost:   SerializeAdminHost(entry.DBHost),
	}
}

func SerializeHostEntries(entries []*groupedhosts.HostEntry) []*HostEntry {
	res := make([]*HostEntry, len(entries))
	for i, entry := range entries {
		res[i] = SerializeHostEntry(entry)
	}
	return res
}

func SerializeHostList(list *groupedhosts.HostList) *HostList {
	return &HostList{
		AllHosts:          SerializeHostEntries(list.AllHosts),
		HealthyHosts:      SerializeHostEntries(list.HealthyHosts),
		InconsistentHosts: SerializeHostEntries(list.InconsistentHosts),
		DanglingHosts:     SerializeAdminHosts(list.DanglingHosts),
		UnaccountedHosts:  SerializeLiveHosts(list.UnaccountedHosts),
	}
}
