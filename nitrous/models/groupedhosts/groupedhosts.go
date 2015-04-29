package groupedhosts

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
)

const danglingGracePeriod = 10 * time.Minute

var (
	// matches the agreed format for testing images `create-test-#{region}-#{image_id}-#{unix_epoch}`
	nonNitrousHosts = regexp.MustCompile(`(?i)^create-test-(\w+)-(\d+)-(\d{10,})$`)
)

type HostEntry struct {
	LiveHost *hostbuilder.Response
	DBHost   *host.Host
}

type hostEntriesByID []*HostEntry
type dbHostsByID []*host.Host
type liveHostsByID []*hostbuilder.Response

type HostIndex map[string](*HostEntry)

type HostList struct {
	AllHosts          []*HostEntry
	HealthyHosts      []*HostEntry
	DanglingHosts     []*host.Host
	UnaccountedHosts  []*hostbuilder.Response
	InconsistentHosts []*HostEntry
}

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

func (list *HostList) sort() {
	sort.Sort(hostEntriesByID(list.AllHosts))
	sort.Sort(hostEntriesByID(list.HealthyHosts))
	sort.Sort(dbHostsByID(list.DanglingHosts))
	sort.Sort(liveHostsByID(list.UnaccountedHosts))
	sort.Sort(hostEntriesByID(list.InconsistentHosts))
}

func (index HostIndex) getEntry(id string) *HostEntry {
	entry, ok := index[id]
	if !ok {
		entry = &HostEntry{}
		index[id] = entry
	}
	return entry
}

func indexHosts(liveHosts []*hostbuilder.Response, dbHosts []*host.Host) (HostIndex, []*host.Host) {
	index := HostIndex{}
	danglingHosts := []*host.Host{}

	for _, liveHost := range liveHosts {
		id := liveHost.ID

		entry := index.getEntry(id)
		entry.LiveHost = liveHost
	}

	for _, dbHost := range dbHosts {
		pid := dbHost.ProviderIdentifier
		if pid.Valid {
			entry := index.getEntry(pid.String)
			entry.DBHost = dbHost
			continue
		}
		if dbHost.State == host.StateBorn && time.Since(*dbHost.CreatedAt) < danglingGracePeriod {
			continue // allow some time for provider to build host
		}

		danglingHosts = append(danglingHosts, dbHost)
	}

	return index, danglingHosts
}

func isDbAndLiveConsistent(db *ndb.Database, liveHost *hostbuilder.Response, dbHost *host.Host) bool {
	// if live and db state is the same, we're consistent
	liveState := strings.ToLower(liveHost.State)
	if liveState == strings.ToLower(dbHost.State) {
		return true
	}
	// Note: if doing lookup for each host turns out to be too slow,
	// we'll have to rework it to grab more transitions in one go
	transition, err := hosttransition.FindMostRecentForHost(db, dbHost.ID)
	if err != nil {
		return false
	}

	// we're consistent if live state is the same as the state we're
	// transitionining from and the transition isn't taking more than 5 mins
	if liveState != strings.ToLower(transition.PreviousState.String) {
		return false
	}
	// paranoia - CreatedAt shouldn't be nil but be safe if it is
	if transition.CreatedAt == nil {
		return false
	}
	// db is correctly marked as transitioning to live state but transition
	// is taking too long
	return time.Since(*transition.CreatedAt) < time.Minute*5
}

func mapHost(db *ndb.Database, list *HostList, entry *HostEntry) {
	liveHost := entry.LiveHost
	dbHost := entry.DBHost

	// Skip non-Nitrous hosts
	if liveHost != nil && nonNitrousHosts.MatchString(liveHost.Name) {
		return
	}

	list.AllHosts = append(list.AllHosts, entry)

	switch {
	case liveHost != nil && dbHost != nil:
		if isDbAndLiveConsistent(db, liveHost, dbHost) {
			list.HealthyHosts = append(list.HealthyHosts, entry)
		} else {
			list.InconsistentHosts = append(list.InconsistentHosts, entry)
		}

	case liveHost != nil && dbHost == nil:
		if liveHost.State != "Terminated" {
			list.UnaccountedHosts = append(list.UnaccountedHosts, liveHost)
		}

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
