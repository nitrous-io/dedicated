package hosts

import (
	"crypto/rsa"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host size can't be blank")
	ErrSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug can't be blank")
	ErrRegionRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "region can't be blank")
	ErrPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	ErrRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	ErrHostNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching host found for the given slug")
	ErrHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not stopped")
	ErrHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	ErrHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")

	hostbuilderURL string
	aesKey         string
)

const HostbuilderURLEnvVar = "HOSTBUILDER_URL"

var HostbuilderClient *hostbuilderclient.Hostbuilder

func init() {
	hostbuilderURL = os.Getenv(HostbuilderURLEnvVar)
	if hostbuilderURL == "" {
		panic(fmt.Sprintf("%v is not set", HostbuilderURLEnvVar))
	}

	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		panic("AES_KEY is not set")
	}
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

	slug := r.FormValue("slug")
	if slug == "" {
		return ErrSlugRequired
	}

	regionName := r.FormValue("region")
	if regionName == "" {
		return ErrRegionRequired
	}

	name := r.FormValue("name")
	if name == "" {
		name = slug
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

	pts, err := providertemplate.FindAllByRegionAndPlan(db, rg.ID, pl.ID)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return ErrProvidertemplateNotFound
	}
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	pt := pts[0]

	priv, pubkey, err := generateKey()
	if err != nil {
		return err
	}

	// generate hostbuilder config
	config := hostbuilderclient.Config{}
	for k, v := range pt.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}
	config["name"] = slug
	config["public_keys"] = []string{string(pubkey)}

	info, err := HostbuilderClient.Create(pt.Provider, &config)
	if err != nil {
		return err
	}

	h := &host.Host{
		UserID:             u.ID,
		RegionID:           rg.ID,
		PlanID:             pl.ID,
		ProviderTemplateID: pt.ID,

		Slug:          slug,
		Name:          sql.NullString{name, true},
		MaxContainers: sql.NullInt64{pl.MaxContainers, true},
		Size:          ndb.NullString{sql.NullString{size, true}},

		IPAddress:          sql.NullString{info.IPAddress, true},
		ProviderIdentifier: sql.NullString{info.ID, true},

		FreighterdPort: sql.NullInt64{int64(info.FreighterdPort), true},
		State:          host.StateRunning,
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	// create the host
	err = h.Insert(db)
	if err != nil {
		tx.Rollback()
		return err
	}

	// create the keypair
	_, err = keypairs.Create(tx, priv, aesKey, h.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
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

	ht, err := getHost(db, env.User, vars["slug"])
	if err != nil {
		return err
	}

	_, err = HostbuilderClient.Delete(ht.Provider, &hostbuilderclient.Config{"id": ht.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	err = ht.Delete(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = container.DeleteAllByHostID(tx, ht.ID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err = keypairs.Delete(tx, ht.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	respondOK(w, logger)
	return nil
}

func Start(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["slug"])
	if err != nil {
		return err
	}

	err = transition(db, ht,
		&stateTransition{host.StateStopped, host.StateStarting, host.StateRunning},
		HostbuilderClient.Start)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostStartInvalidState
		}
		return err
	}

	respondOK(w, logger)
	return nil
}

func Stop(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["slug"])
	if err != nil {
		return err
	}

	err = transition(db, ht,
		&stateTransition{host.StateRunning, host.StateStopping, host.StateStopped},
		HostbuilderClient.Stop)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostStopInvalidState
		}
		return err
	}

	respondOK(w, logger)
	return nil
}

func Restart(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()
	vars := env.Vars

	ht, err := getHost(db, env.User, vars["slug"])
	if err != nil {
		return err
	}

	err = transition(db, ht,
		&stateTransition{host.StateRunning, host.StateRestarting, host.StateRunning},
		HostbuilderClient.Restart)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostRestartInvalidState
		}
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

func getHost(db *ndb.Database, user *user.User, slug string) (*host.Host, error) {
	ht, err := host.FindByUserAndSlug(db, user, slug)
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

type stateTransitionError struct {
	Expected string
	Actual   string
}

func (e *stateTransitionError) Error() string {
	return fmt.Sprintf("Expected initial state to be %s, was %s", e.Expected, e.Actual)
}

func transition(d ndb.QueryReceiver, h *host.Host, st *stateTransition, fn hostbuilderclient.OperationFn) error {
	if h.State != string(st.From) {
		return &stateTransitionError{Expected: string(st.From), Actual: h.State}
	}

	err := h.UpdateState(d, st.Intermediate)
	if err != nil {
		return err
	}

	_, err = fn(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	err = h.UpdateState(d, st.To)
	if err != nil {
		return err
	}

	return nil
}

// generate a private key and return it, together with its public key
func generateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
	// generate a private key for the host
	priv, err = keypairs.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	pub, err = keypairs.PublicKey(&priv.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}
