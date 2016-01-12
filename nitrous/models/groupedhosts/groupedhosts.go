package groupedhosts

import (
	"sort"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
)

const (
	defaultGracePeriod      = 10 * time.Minute
	inconsistentGracePeriod = 5 * time.Minute
	respecGracePeriod       = 15 * time.Minute
)

type HostEntry struct {
	LiveHost *hostbuilder.Response
	DBHost   *host.Host
}

type hostEntriesByID []*HostEntry
type dbHostsByID []*host.Host
type liveHostsByID []*hostbuilder.Response

func (entries hostEntriesByID) Len() int {
	return len(entries)
}

func (entries hostEntriesByID) Less(i, j int) bool {
	entry1, entry2 := entries[i], entries[j]
	if entry1.DBHost != nil && entry2.DBHost != nil {
		// sort db host id in reverse order
		return entry1.DBHost.ID > entry2.DBHost.ID
	} else if entry1.DBHost != nil && entry2.DBHost == nil {
		// prioritize host with db entry
		return true
	} else if entry1.DBHost == nil && entry2.DBHost != nil {
		return false
	} else if entry1.LiveHost != nil && entry2.LiveHost != nil {
		// if both entry.db_host are nil, their live_host
		// should not be nil
		return entry1.LiveHost.ID < entry2.LiveHost.ID
	} else {
		// this should never reach. return dummy value
		return false
	}
}

func (entries hostEntriesByID) Swap(i, j int) {
	entries[i], entries[j] = entries[j], entries[i]
}

func (entries dbHostsByID) Len() int {
	return len(entries)
}

func (entries dbHostsByID) Less(i, j int) bool {
	// sort db host id in reverse order
	return entries[i].ID > entries[j].ID
}

func (entries dbHostsByID) Swap(i, j int) {
	entries[i], entries[j] = entries[j], entries[i]
}

func (entries liveHostsByID) Len() int {
	return len(entries)
}

func (entries liveHostsByID) Less(i, j int) bool {
	return entries[i].ID < entries[j].ID
}

func (entries liveHostsByID) Swap(i, j int) {
	entries[i], entries[j] = entries[j], entries[i]
}

type HostList struct {
	AllHosts          []*HostEntry
	HealthyHosts      []*HostEntry
	DanglingHosts     []*host.Host
	UnaccountedHosts  []*hostbuilder.Response
	InconsistentHosts []*HostEntry
}

func (list *HostList) sort() {
	sort.Sort(hostEntriesByID(list.AllHosts))
	sort.Sort(hostEntriesByID(list.HealthyHosts))
	sort.Sort(dbHostsByID(list.DanglingHosts))
	sort.Sort(liveHostsByID(list.UnaccountedHosts))
	sort.Sort(hostEntriesByID(list.InconsistentHosts))
}

// ProviderIdentifier/ProviderStorageIdentifier -> HostEntry map.
type HostIndex map[string](*HostEntry)

// getEntry returns the HostEntry with the given key. It initializes and returns
// a pointer to a zero-value HostEntry if no entry exists.
func (index HostIndex) getEntry(key string) *HostEntry {
	entry, ok := index[key]
	if !ok {
		entry = &HostEntry{}
		index[key] = entry
	}
	return entry
}

func indexHosts(liveHosts []*hostbuilder.Response, dbHosts []*host.Host) (HostIndex, []*host.Host) {
	index := HostIndex{}
	danglingHosts := []*host.Host{}

	// Add all live host data to a map.
	for _, h := range liveHosts {
		identifier := h.ID
		if identifier != "" {
			entry := index.getEntry(identifier)
			entry.LiveHost = h
		}

		if identifier := h.VolumeID; identifier != "" {
			entry := index.getEntry(identifier)
			entry.LiveHost = h
		}
	}

	// Now, add all db host data to the same map, and also identify "dangling"
	// hosts.
	// A hotswap host is "dangling" when a record exists in db but we don't know
	// what its volume ID is.
	for _, h := range dbHosts {
		var identifiable bool

		if identifier := h.ProviderIdentifier.String; identifier != "" {
			entry := index.getEntry(identifier)
			entry.DBHost = h
			identifiable = true
		}

		if identifier := h.ProviderStorageIdentifier.String; identifier != "" {
			entry := index.getEntry(identifier)
			entry.DBHost = h
			identifiable = true
		}

		if identifiable {
			continue
		}

		if h.State == host.StateBorn && time.Since(*h.CreatedAt) < defaultGracePeriod {
			continue // allow some time for provider to build host
		}

		danglingHosts = append(danglingHosts, h)
	}

	return index, danglingHosts
}

func isConsistent(db *ndb.Database, liveHost *hostbuilder.Response, dbHost *host.Host) bool {
	switch dbHost.State {
	case host.StateRunning:
		// A running host should be actually running.
		if liveHost.State != "Running" {
			return false
		}

		// Host have an underlying instance.
		if dbHost.ProviderIdentifier.String == "" {
			return false
		}

		return true

	case host.StateStopped:
		// A stopped host should not have a provider identifier (since volume is not
		// attached to any instance).
		if dbHost.ProviderIdentifier.String != "" {
			return false
		}

		if liveHost.State == "Unknown" || liveHost.State == "" {
			return true
		}
	}

	return isTransitioning(db, dbHost)
}

func isTransitioning(db *ndb.Database, h *host.Host) bool {
	// Note: if doing lookup for each host turns out to be too slow,
	// we'll have to rework it to grab more transitions in one go
	transition, err := hosttransition.FindMostRecentForHost(db, h.ID)
	if err != nil {
		return false
	}

	// paranoia - CreatedAt shouldn't be nil but be safe if it is
	if transition.CreatedAt == nil {
		return false
	}

	// allow more time for respeccing
	if h.State == host.StateRespeccing {
		return time.Since(*transition.CreatedAt) < respecGracePeriod
	}

	// db is correctly marked as transitioning to live state but transition
	// is taking too long
	return time.Since(*transition.CreatedAt) < inconsistentGracePeriod
}

func mapHost(db *ndb.Database, list *HostList, entry *HostEntry) {
	liveHost := entry.LiveHost
	dbHost := entry.DBHost

	list.AllHosts = append(list.AllHosts, entry)

	switch {
	case liveHost != nil && dbHost != nil:
		if isConsistent(db, liveHost, dbHost) {
			list.HealthyHosts = append(list.HealthyHosts, entry)
		} else {
			list.InconsistentHosts = append(list.InconsistentHosts, entry)
		}
	case liveHost != nil && dbHost == nil:
		if liveHost.State == "Terminated" {
			break
		}
		if liveHost.LaunchTime != nil && time.Since(*liveHost.LaunchTime) <= defaultGracePeriod {
			break
		}
		list.UnaccountedHosts = append(list.UnaccountedHosts, liveHost)

	case liveHost == nil && dbHost != nil:
		list.DanglingHosts = append(list.DanglingHosts, dbHost)
	}
}

func mapHosts(db *ndb.Database, liveHosts []*hostbuilder.Response, dbHosts []*host.Host) *HostList {
	list := &HostList{}

	index, danglingHosts := indexHosts(liveHosts, dbHosts)

	for _, danglingHost := range danglingHosts {
		list.AllHosts = append(list.AllHosts, &HostEntry{
			DBHost: danglingHost,
		})
	}
	list.DanglingHosts = danglingHosts

	for _, entry := range index {
		mapHost(db, list, entry)
	}

	list.sort()
	return list
}

func ListHosts(db *ndb.Database, client *hostbuilderclient.Hostbuilder, provider string) (*HostList, error) {
	dbHosts, err := host.FindAllByProvider(db, provider)
	if err != nil {
		return nil, err
	}

	liveHosts, err := client.List(provider)
	if err != nil {
		return nil, err
	}

	res := mapHosts(db, liveHosts, dbHosts)

	return res, nil
}
