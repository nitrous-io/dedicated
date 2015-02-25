package regions

import (
	"log"
	"net/http"
	"sync"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	validRegionIdsForProvider map[int64]bool
	mu                        sync.Mutex
)

func initValidRegionIdsForProvider(db db.QueryReceiver) {
	mu.Lock()
	defer mu.Unlock()
	if validRegionIdsForProvider != nil {
		return
	}
	defaultProvider := envvars.DefaultProvider
	validRegionIdsForProvider = make(map[int64]bool)

	templates, err := providertemplate.All(db)
	if err != nil {
		log.Printf("initValidRegionIdsForProvider(): providertemplate.All() failed with '%s'", err)
	}
	for _, t := range templates {
		if t.Provider == defaultProvider {
			validRegionIdsForProvider[t.RegionID] = true
		}
	}
}

func filterByDefaultProvider(rs []*region.Region) []*region.Region {
	var res []*region.Region
	for _, r := range rs {
		if validRegionIdsForProvider[r.ID] {
			res = append(res, r)
		}
	}
	return res
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	rs, err := region.All(env.DB)
	if err != nil {
		return err
	}

	initValidRegionIdsForProvider(env.DB)
	rs = filterByDefaultProvider(rs)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"regions": serializers.SerializeRegions(rs),
	}
	return nil
}
