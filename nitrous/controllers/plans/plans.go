package plans

import (
	"log"
	"net/http"
	"sync"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	validPlanIdsForDefaultProvider map[int64]bool
	mu                             sync.Mutex
)

func initValidPlansIdsForDefaultProvider(db db.QueryReceiver) {
	mu.Lock()
	defer mu.Unlock()

	if validPlanIdsForDefaultProvider != nil {
		return
	}
	defaultProvider := envvars.DefaultProvider
	validPlanIdsForDefaultProvider = make(map[int64]bool)
	templates, err := providertemplate.All(db)
	if err != nil {
		log.Printf("initValidPlansIdsForDefaultProvider: providertemplate.All() failed with '%s'\n", err)
		return
	}
	for _, t := range templates {
		if t.Provider == defaultProvider {
			validPlanIdsForDefaultProvider[t.PlanID] = true
		}
	}
}

func filterByDefaultProvider(pls []*plan.Plan) []*plan.Plan {
	var res []*plan.Plan
	for _, p := range pls {
		if validPlanIdsForDefaultProvider[p.ID] {
			res = append(res, p)
		}
	}
	return res
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pls, err := plan.Enabled(env.DB)
	if err != nil {
		return err
	}
	initValidPlansIdsForDefaultProvider(env.DB)
	pls = filterByDefaultProvider(pls)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(pls),
	}
	return nil
}
