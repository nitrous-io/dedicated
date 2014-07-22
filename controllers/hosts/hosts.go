package hosts

import (
	"crypto/rsa"
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/sslider/sslgen"
)

var (
	ErrSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "size is required")
	ErrSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	ErrRegionRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "region is required")
	ErrPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	ErrRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	ErrHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not stopped")
	ErrHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	ErrHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")

	hostbuilderURL string
	aesKey         string
)

var (
	validations = map[string]error{
		"index_hosts_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

const HostbuilderURLEnvVar = "HOSTBUILDER_URL"

var HostbuilderClient *hostbuilderclient.Hostbuilder
var FreighterClient freighterclient.Client

func init() {
	hostbuilderURL = os.Getenv(HostbuilderURLEnvVar)
	if hostbuilderURL == "" {
		panic(fmt.Sprintf("%v is not set", HostbuilderURLEnvVar))
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	FreighterClient = freighterclient.NewClient(nil)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		panic("AES_KEY is not set")
	}
}

func Index(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(c.DB(), env.User)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hosts,
	}
	return nil
}

// TODO:
// check if we need to save the slug first to secure the uniqueness of domain,
// before we generate the cert.
func Create(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

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

	h := &host.Host{
		UserID:             env.User.ID,
		RegionID:           rg.ID,
		PlanID:             pl.ID,
		ProviderTemplateID: pt.ID,
		Slug:               slug,
		Name:               sql.NullString{name, true},
		MaxContainers:      sql.NullInt64{pl.MaxContainers, true},
		Size:               ndb.NullString{sql.NullString{size, true}},
		State:              host.StateBorn,
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	err = h.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	crt, err := generateCerts(fmt.Sprintf("%s.nitrouspro.com", slug))
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
	config["tls"] = map[string]string{
		"ca":   crt.CA,
		"cert": crt.ServerCert,
		"key":  crt.ServerKey,
	}

	info, err := HostbuilderClient.Create(pt.Provider, &config)
	if err != nil {
		updErr := h.UpdateState(db, host.StateOrphan)
		if updErr != nil {
			c.Logger().Errorf("Unable to update host %d to orphan state that failed on hostbuilder: %v", h.ID, updErr)
		}
		return err
	}

	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.ProviderIdentifier = sql.NullString{info.ID, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
	h.State = host.StateRunning

	tx, err = db.Beginx()
	if err != nil {
		return err
	}
	err = h.Update(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	if _, err = keypairs.Create(tx, priv, aesKey, h.ID); err != nil {
		tx.Rollback()
		return err
	}

	// create the cert
	if err := crt.Insert(tx, h.ID); err != nil {
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

func Show(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	var info *freighterclient.HostInfo
	var err error

	// Request info from Freighter if host is running with an IP address.
	if h.IPAddress.String != "" && h.State == host.StateRunning {
		fHost := freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false)
		info, err = FreighterClient.HostInfo(fHost)
		if err != nil {
			return err
		}
	}

	rg, err := region.FindByID(c.DB(), h.RegionID)
	if err != nil {
		return err
	}

	pl, err := plan.FindByID(c.DB(), h.PlanID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": controllers.NewHostDetailedAPI(h, rg, pl, info),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	_, err := HostbuilderClient.Delete(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	tx, err := c.DB().Beginx()
	if err != nil {
		return err
	}
	err = h.Delete(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = container.DeleteAllByHostID(tx, h.ID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err = keypairs.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := cert.Delete(tx, ht.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func Start(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	err := transition(c.DB(), h,
		&stateTransition{host.StateStopped, host.StateStarting, host.StateRunning},
		HostbuilderClient.Start)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostStartInvalidState
		}
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func Stop(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	err := transition(c.DB(), h,
		&stateTransition{host.StateRunning, host.StateStopping, host.StateStopped},
		HostbuilderClient.Stop)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostStopInvalidState
		}
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func Restart(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	err := transition(c.DB(), h,
		&stateTransition{host.StateRunning, host.StateRestarting, host.StateRunning},
		HostbuilderClient.Restart)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostRestartInvalidState
		}
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func respondOK(w http.ResponseWriter, logger *logger.Logger) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")
}

// stateTransition reflects the transition of host states during a hostbuilder operation.
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

// generateKey generates and returns a private key together with its public key.
func generateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
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

// generate a certificate that include the certificate authority, server
// certificate and key, client certificate and key. The data will be suitable
// for TLS authentication
func generateCerts(hostname string) (*cert.Cert, error) {
	// generate certificate authority
	ca, err := sslgen.CreateCA(hostname)
	if err != nil {
		return nil, err
	}
	_, caCrt, err := ca.Generate()
	if err != nil {
		return nil, err
	}

	// generate server certificates
	srv, err := sslgen.CreateServerCert(hostname, ca.PrivateKey)
	if err != nil {
		return nil, err
	}
	srvKey, srvCrt, err := srv.Generate()
	if err != nil {
		return nil, err
	}

	// generate client certificates
	cl, err := sslgen.CreateClientCert(hostname, ca.PrivateKey)
	if err != nil {
		return nil, err
	}
	clKey, clCrt, err := cl.Generate()
	if err != nil {
		return nil, err
	}

	return &cert.Cert{
		CA:         string(caCrt),
		ServerCert: string(srvCrt),
		ServerKey:  string(srvKey),
		ClientCert: string(clCrt),
		ClientKey:  string(clKey),
	}, nil
}
