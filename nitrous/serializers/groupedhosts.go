package serializers

import (
	"fmt"

	"github.com/nitrous-io/dedicated/hostbuilder"
	"github.com/nitrous-io/dedicated/nitrous/models/groupedhosts"
)

type LiveHost struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	VolumeID       string `json:"volume_id"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort uint16 `json:"freighterd_port"`
	Region         string `json:"region"`
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

type Healthcheck struct {
	Passed            bool     `json:"passed"`
	HealthyCount      int      `json:"healthy_count"`
	Inconsistent      []string `json:"inconsistent"`
	Dangling          []string `json:"dangling"`
	Unaccounted       []string `json:"unaccounted"`
	InconsistentCount int      `json:"inconsistent_count"`
	DanglingCount     int      `json:"dangling_count"`
	UnaccountedCount  int      `json:"unaccounted_count"`
}

func SerializeLiveHost(h *hostbuilder.Response) *LiveHost {
	if h == nil {
		return nil
	}

	return &LiveHost{
		Provider:       h.Provider,
		ID:             h.ID,
		VolumeID:       h.VolumeID,
		Name:           h.Name,
		IPAddress:      h.IPAddress,
		FreighterdPort: h.FreighterdPort,
		Region:         h.Region,
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

func SerializeHealthcheck(list *groupedhosts.HostList) *Healthcheck {
	healthyCount := len(list.HealthyHosts)
	inconsistentCount := len(list.InconsistentHosts)
	danglingCount := len(list.DanglingHosts)
	unaccountedCount := len(list.UnaccountedHosts)
	passed := inconsistentCount == 0 && danglingCount == 0 && unaccountedCount == 0

	inconsistent := []string{}
	for _, h := range list.InconsistentHosts {
		info := fmt.Sprintf("%s (db:%s,live:%s)", h.DBHost.Slug, h.DBHost.State, h.LiveHost.State)
		inconsistent = append(inconsistent, info)
	}
	dangling := []string{}
	for _, h := range list.DanglingHosts {
		dangling = append(dangling, h.Slug)
	}
	unaccounted := []string{}
	for _, h := range list.UnaccountedHosts {
		unaccounted = append(unaccounted, h.ID)
	}

	return &Healthcheck{
		Passed:            passed,
		Inconsistent:      inconsistent,
		Dangling:          dangling,
		Unaccounted:       unaccounted,
		HealthyCount:      healthyCount,
		InconsistentCount: inconsistentCount,
		DanglingCount:     danglingCount,
		UnaccountedCount:  unaccountedCount,
	}
}
