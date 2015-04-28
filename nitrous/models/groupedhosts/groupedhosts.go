package groupedhosts

import (
	"regexp"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder"
	dbLib "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
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

type HostIndex map[string](*HostEntry)

type HostList struct {
	AllHosts          []*HostEntry
	HealthyHosts      []*HostEntry
	DanglingHosts     []*host.Host
	UnaccountedHosts  []*hostbuilder.Response
	InconsistentHosts []*HostEntry
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

func mapHost(list *HostList, entry *HostEntry) {
	liveHost := entry.LiveHost
	dbHost := entry.DBHost

	// Skip non-Nitrous hosts
	if liveHost != nil && nonNitrousHosts.MatchString(liveHost.Name) {
		return
	}

	list.AllHosts = append(list.AllHosts, entry)

	switch {
	case liveHost != nil && dbHost != nil:
		if strings.ToLower(liveHost.State) == strings.ToLower(dbHost.State) {
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

func mapHosts(liveHosts []*hostbuilder.Response, dbHosts []*host.Host) *HostList {
	list := &HostList{}

	index, danglingHosts := indexHosts(liveHosts, dbHosts)

	for _, danglingHost := range danglingHosts {
		list.AllHosts = append(list.AllHosts, &HostEntry{
			DBHost: danglingHost,
		})
	}
	list.DanglingHosts = danglingHosts

	for _, entry := range index {
		mapHost(list, entry)
	}

	return list
}

func ListHosts(db *dbLib.Database, client *hostbuilderclient.Hostbuilder, provider string) (*HostList, error) {
	dbHosts, err := host.FindAllByProvider(db, provider)
	if err != nil {
		return nil, err
	}

	liveHosts, err := client.List(provider)
	if err != nil {
		return nil, err
	}

	res := mapHosts(liveHosts, dbHosts)

	return res, nil
}
