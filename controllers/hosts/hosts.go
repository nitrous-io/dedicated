package hosts

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/hostbuilder"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host size not provided")
	ErrPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	ErrRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	ErrHostNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching host found for the given name")

	hostbuilderURL string
)

const HostbuilderURLEnvVar = "HOSTBUILDER_URL"

var HostbuilderClient *hostbuilder.Hostbuilder

func init() {
	hostbuilderURL = os.Getenv(HostbuilderURLEnvVar)
	if hostbuilderURL == "" {
		panic(fmt.Sprintf("%v is not set", HostbuilderURLEnvVar))
	}

	HostbuilderClient = hostbuilder.NewHostbuilder(hostbuilderURL)
}

func Index(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	hosts, err := host.FindAllByUser(db, u)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hosts,
	}
	return nil
}

func Create(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	size := r.FormValue("size")
	if size == "" {
		return ErrSizeRequired
	}

	name := r.FormValue("name")
	if name == "" {
		name = "double-rainbow" //FIXME: implement a random name generator
	}

	regionName := r.FormValue("region")
	if regionName == "" {
		regionName = "apse" //FIXME: user model should specify a default region?
	}

	longname := r.FormValue("longname")
	if longname == "" {
		longname = name
	}

	// TODO: check if user account is eligible to create hosts (has a credit card on file)

	pl, err := plan.FindByName(db, size)
	if err != nil {
		return err
	}
	if pl == nil {
		return ErrPlanNotFound
	}

	rg, err := region.FindByName(db, regionName)
	if err != nil {
		return err
	}
	if rg == nil {
		return ErrRegionNotFound
	}

	pts, err := providertemplate.FindAllByRegionAndPlan(db, pl.ID, rg.ID)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return ErrProvidertemplateNotFound
	}
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	pt := pts[0]

	config := hostbuilder.Config{}
	for k, v := range pt.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}
	config["name"] = name
	config["public_keys"] = []string{} //FIXME: pass actual public keys

	info, err := HostbuilderClient.Create(pt.Provider, &config)
	if err != nil {
		return err
	}

	h := &host.Host{
		UserID:             u.ID,
		RegionID:           rg.ID,
		PlanID:             pl.ID,
		ProviderTemplateID: pt.ID,

		Name:          name,
		LongName:      sql.NullString{longname, true},
		MaxContainers: sql.NullInt64{pl.MaxContainers, true},

		IPAddress:          sql.NullString{info.IPAddress, true},
		ProviderIdentifier: sql.NullString{info.ID, true},

		FreighterdPort: sql.NullInt64{int64(info.FreighterdPort), true},
		State:          host.StateCreated,
	}
	err = h.Insert(db)
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
		"host":   h,
	}
	return nil
}

func Destroy(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["name"])
	if err != nil {
		return err
	}

	_, err = HostbuilderClient.Delete(ht.Provider, &hostbuilder.Config{"id": ht.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	err = ht.Delete(db)
	if err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func Start(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["name"])
	if err != nil {
		return err
	}

	err = transition(db, ht,
		&stateTransition{host.StateStopped, host.StateStarting, host.StateRunning},
		HostbuilderClient.Start)
	if err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func Stop(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["name"])
	if err != nil {
		return err
	}

	err = transition(db, ht,
		&stateTransition{host.StateRunning, host.StateStopping, host.StateStopped},
		HostbuilderClient.Stop)
	if err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func Restart(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["name"])
	if err != nil {
		return err
	}

	err = transition(db, ht,
		&stateTransition{host.StateRunning, host.StateRestarting, host.StateRunning},
		HostbuilderClient.Restart)
	if err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func respondOK(w http.ResponseWriter, logger *logger.Logger) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")
}

func getHost(db *db.Database, user *user.User, name string) (*host.Host, error) {
	ht, err := host.FindByUserAndName(db, user, name)
	if err != nil {
		return nil, err
	}
	if ht == nil {
		return nil, ErrHostNotFound
	}
	if ht.ProviderIdentifier.Valid != true {
		return nil, errors.New("Provider identifier is nil")
	}

	return ht, nil
}

// a stateTransition reflects the transition of host states during a hostbuilder operation
type stateTransition struct {
	From         string
	Intermediate string
	To           string
}

func transition(d db.QueryReceiver, h *host.Host, st *stateTransition, fn hostbuilder.OperationFn) error {
	if h.State != st.From {
		return fmt.Errorf("Expected initial state to be %s, was %s", st.From, h.State)
	}

	err := h.UpdateState(d, st.Intermediate)
	if err != nil {
		return err
	}

	_, err = fn(h.Provider, &hostbuilder.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	err = h.UpdateState(d, st.To)
	if err != nil {
		return err
	}

	return nil
}
