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
	"github.com/nitrous-io/nitrous/pkg/hostbuilder"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrUserNil                  = errors.New("user is nil")
	ErrSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host size not provided")
	ErrPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	ErrRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	ErrHostNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching host found for the given name")

	hostbuilderURL string
)

const HostbuilderURLEnvVar = "HOSTBUILDER_URL"

func init() {
	hostbuilderURL = os.Getenv(HostbuilderURLEnvVar)
	if hostbuilderURL == "" {
		panic(fmt.Sprintf("%v is not set", HostbuilderURLEnvVar))
	}
}

func Index(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	u := env.User
	if u == nil {
		return ErrUserNil
	}

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

func Create(hb *hostbuilder.Hostbuilder, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	u := env.User
	if u == nil {
		return ErrUserNil
	}

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
		regionName = "apse1" //FIXME: user model should specify a default region?
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

	config := map[string]string{}
	for k, v := range pt.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}
	config["name"] = name
	config["public_keys"] = "" //FIXME: pass actual public keys

	info, err := hb.Create(pt.Provider, config)
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

		IPAddress:          sql.NullString{info["ip_address"], true},
		ProviderIdentifier: sql.NullString{info["id"], true},
		State:              "created",
	}
	err = h.Insert(db)
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"host": h,
	}
	return nil
}

// Provider name should be stored in the host?
type hostWithProvider struct {
	host     *host.Host
	provider string
}

func getHostWithProvider(db *db.Database, name string) (*hostWithProvider, error) {
	htp := &hostWithProvider{}
	ht, err := host.FindByName(db, name)
	if err != nil {
		return htp, err
	}
	if ht == nil {
		return nil, ErrHostNotFound
	}

	htp.host = ht

	// Consider storing provider name in host?
	pt, err := providertemplate.FindByID(db, ht.ProviderTemplateID)
	if err != nil {
		return nil, err
	}
	if pt == nil {
		return nil, ErrProvidertemplateNotFound
	}

	htp.provider = pt.Provider

	return htp, nil
}

func Destroy(hb *hostbuilder.Hostbuilder, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	htp, err := getHostWithProvider(db, vars["name"])
	if err != nil {
		return err
	}

	ht := htp.host
	config := map[string]string{}
	if ht.ProviderIdentifier.Valid {
		config["id"] = ht.ProviderIdentifier.String
	} else {
		return errors.New("Provider identifier is nil")
	}

	_, err = hb.Delete(htp.provider, config)
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

func Start(hb *hostbuilder.Hostbuilder, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	htp, err := getHostWithProvider(db, vars["name"])
	if err != nil {
		return err
	}

	ht := htp.host

	config := map[string]string{}
	if ht.ProviderIdentifier.Valid {
		config["id"] = ht.ProviderIdentifier.String
	} else {
		return errors.New("Provider identifier is nil")
	}

	_, err = hb.Start(htp.provider, config)
	if err != nil {
		return err
	}

	ht.State = "running"
	err = ht.UpdateColumn(db, "state")
	if err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func Stop(hb *hostbuilder.Hostbuilder, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	htp, err := getHostWithProvider(db, vars["name"])
	if err != nil {
		return err
	}

	ht := htp.host

	config := map[string]string{}
	if ht.ProviderIdentifier.Valid {
		config["id"] = ht.ProviderIdentifier.String
	} else {
		return errors.New("Provider identifier is nil")
	}

	_, err = hb.Stop(htp.provider, config)
	if err != nil {
		return err
	}

	ht.State = "stopped"
	err = ht.UpdateColumn(db, "state")
	if err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func Restart(hb *hostbuilder.Hostbuilder, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	htp, err := getHostWithProvider(db, vars["name"])
	if err != nil {
		return err
	}

	ht := htp.host

	config := map[string]string{}
	if ht.ProviderIdentifier.Valid {
		config["id"] = ht.ProviderIdentifier.String
	} else {
		return errors.New("Provider identifier is nil")
	}

	_, err = hb.Restart(htp.provider, config)
	if err != nil {
		return err
	}

	// not entirely sure about the purpose of state,
	// but we don't check the state before running the command,
	// so just setting it to "running" if the action was successful.
	ht.State = "running"
	err = ht.UpdateColumn(db, "state")
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

func UseHostbuilder(fn func(hb *hostbuilder.Hostbuilder, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		hb := hostbuilder.NewHostbuilder(hostbuilderURL)
		return fn(hb, env, c, w, r)
	}
}
