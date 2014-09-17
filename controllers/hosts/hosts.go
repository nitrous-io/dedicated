package hosts

import (
	"crypto/rsa"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/hostbuilder"

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
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	errSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "size is required")
	errSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errRegionRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "region is required")
	errPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	errRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	errProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	errHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not stopped")
	errHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errHostNotRunning           = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errCertNotFound             = httperror.New(http.StatusNotFound, "not_found", "host certificate does not exist")
)

var (
	validations = map[string]error{
		"index_hosts_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var HostbuilderClient *hostbuilderclient.Hostbuilder
var defaultProvider string
var aesKey string
var defaultAPTLocation string
var defaultAPTDist string

func init() {
	hostbuilderURL := os.Getenv("HOSTBUILDER_URL")
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		panic("AES_KEY is not set")
	}

	defaultProvider = os.Getenv("DEFAULT_PROVIDER")
	if defaultProvider == "" {
		panic("DEFAULT_PROVIDER is not set")
	}

	defaultAPTLocation = os.Getenv("DEFAULT_APT_LOCATION")
	if defaultAPTLocation == "" {
		panic("DEFAULT_APT_LOCATION is not set")
	}

	defaultAPTDist = os.Getenv("DEFAULT_APT_DIST")
	if defaultAPTDist == "" {
		panic("DEFAULT_APT_DIST is not set")
	}
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(env.DB, env.User)
	if err != nil {
		return err
	}

	hostsResp := make([]*serializers.Host, len(hosts))
	for i, h := range hosts {
		hostsResp[i] = serializers.SerializeHost(&h)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hostsResp,
	}
	return nil
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	size := r.FormValue("size")
	if size == "" {
		return errSizeRequired
	}
	slug := strings.ToLower(r.FormValue("slug"))
	if slug == "" {
		return errSlugRequired
	}
	regionSlug := r.FormValue("region")
	if regionSlug == "" {
		return errRegionRequired
	}
	name := r.FormValue("name")
	if name == "" {
		name = slug
	}

	// TODO: check if user account is eligible to create hosts (has a credit card on file)
	pl, err := plan.FindBySlug(db, size)
	if err != nil {
		return err
	}
	if pl == nil {
		return errPlanNotFound
	}

	rg, err := region.FindBySlug(db, regionSlug)
	if err != nil {
		return err
	}
	if rg == nil {
		return errRegionNotFound
	}

	pt, err := providertemplate.FindByRegionAndPlanAndProvider(db, rg.ID, pl.ID, defaultProvider)
	if err != nil {
		return err
	}
	if pt == nil {
		return errProvidertemplateNotFound
	}

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
		Size:               sql.NullString{size, true},
		State:              host.StateBorn,
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	if err := h.Insert(tx); err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}
	// create the cert
	crt, err := generateCerts(h.Hostname())
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := crt.Insert(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
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

	// Assuming that t is not nil cos if user gets here,
	// that means she has a token, but return err if there's
	// some other unexpected error
	t, err := token.FindByUser(db, env.User.ID)
	if err != nil {
		return err
	}

	config["name"] = slug
	config["public_keys"] = []string{string(pubkey)}
	config["secret"] = h.Secret
	config["apt_username"] = env.User.Username
	config["apt_password"] = t.Token
	config["apt_location"] = defaultAPTLocation
	config["apt_dist"] = defaultAPTDist

	caInfo, err := controllers.GetCAInfo(false)
	if err != nil {
		return err
	}
	config["tls"] = map[string]string{
		"ca":   string(caInfo.CertPEM),
		"cert": crt.ServerCert,
		"key":  crt.ServerKey,
	}

	createHost := func() error {
		var err error
		defer func() {
			if err != nil {
				h.State = host.StateOrphan
				_ = h.Update(db) // Ignore error here.
				publishHostEvent(env, controllers.EventHostCreateFailed, h)
			}
		}()

		var info *hostbuilder.Response
		info, err = HostbuilderClient.Create(pt.Provider, &config)
		if err != nil {
			return err
		}

		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{info.ID, true}
		h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
		h.State = host.StateRunning

		var tx *sqlx.Tx
		tx, err = db.Beginx()
		if err != nil {
			return err
		}
		if err = h.Update(tx); err != nil {
			tx.Rollback()
			return err
		}
		if _, err = keypairs.Create(tx, priv, aesKey, h.ID); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}

		publishHostEvent(env, controllers.EventHostCreated, h)
		return nil
	}

	async, _ := strconv.ParseBool(r.FormValue("async"))
	switch async {
	case true:
		go createHost()
		env.Status = http.StatusAccepted
	default:
		if err := createHost(); err != nil {
			return err
		}
		env.Status = http.StatusCreated
	}

	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
	return nil
}

func Show(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	serializeHost(env, h)
	return nil
}

func Update(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// since name can be null, we interpret absence of name field to mean no
	// change, and presence of name field with empty string to mean set it to
	// null
	r.ParseForm()
	if _, ok := r.PostForm["name"]; !ok {
		serializeHost(env, h)
		return nil
	}

	name := r.PostFormValue("name")
	h.Name.String, h.Name.Valid = name, name != ""
	if err := h.Update(env.DB); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostUpdated, h)
	serializeHost(env, h)
	return nil
}

var defaultPeriod = 100 * time.Millisecond

func Stats(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Host should be running and have an IP address.
	if h.State != host.StateRunning || h.IPAddress.String == "" {
		return errHostNotRunning
	}

	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	period, err := time.ParseDuration(r.FormValue("period"))
	if err != nil || period == 0 {
		period = defaultPeriod
	}

	info, err := cl.HostInfo(period)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"stats": serializers.SerializeHostInfo(info),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// If provider identifier is empty, don't attempt to delete the host in hostbuilder
	// Provider identifier can be empty for hosts that are dangling in born or orphan states.
	if h.ProviderIdentifier.String != "" {
		_, err := HostbuilderClient.Delete(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
		if err != nil {
			return err
		}
	}

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Delete(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := container.DeleteAllByHostID(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err = keypairs.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := cert.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostDestroyed, h)
	serializeHost(env, h)
	return nil
}

func Start(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateStopped {
		return errHostStartInvalidState
	}
	if err := transition(env.DB, h,
		host.StateStarting, host.StateRunning,
		HostbuilderClient.Start); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostStarted, h)
	serializeHost(env, h)
	return nil
}

func Stop(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateRunning {
		return errHostStopInvalidState
	}
	if err := stopContainers(env, h); err != nil {
		return err
	}
	if err := transition(env.DB, h,
		host.StateStopping, host.StateStopped,
		HostbuilderClient.Stop); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostStopped, h)
	serializeHost(env, h)
	return nil
}

func Restart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateRunning {
		return errHostRestartInvalidState
	}
	if err := stopContainers(env, h); err != nil {
		return err
	}
	err := transition(env.DB, h,
		host.StateRestarting, host.StateRunning,
		HostbuilderClient.Restart)
	if err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostRestarted, h)
	serializeHost(env, h)
	return nil
}

func transition(d ndb.QueryReceiver, h *host.Host, midState, endState string, fn hostbuilderclient.OperationFn) error {
	h.State = midState
	if err := h.Update(d); err != nil {
		return err
	}

	info, err := fn(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	h.State = endState
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	if err := h.Update(d); err != nil {
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

func serializeHost(env *middleware.Env, h *host.Host) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
}

func publishHostEvent(env *middleware.Env, name controllers.EventName, h *host.Host) {
	env.Events.Publish(
		controllers.NewEvent(name, serializers.SerializeHost(h)),
		env.User.Topic(),
	)
}

// generate cert with server certificate and key. The cert will be used for
// TLS encryption
func generateCerts(hostname string) (*cert.Cert, error) {
	caInfo, err := controllers.GetCAInfo(true)
	if err != nil {
		return nil, err
	}
	return cert.Generate(hostname, caInfo.X509Cert, caInfo.Key)
}

func stopContainers(env *middleware.Env, h *host.Host) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		return err
	}
	unstopped := []string{}
	for _, cn := range cns {
		if err := stopContainer(env, cl, cn); err != nil {
			env.Logger.Errorf("hosts.Stop: Unable to stop container %s", cn.Slug)
			unstopped = append(unstopped, cn.Slug)
		}
	}
	if len(unstopped) == 0 {
		return nil
	}
	return httperror.New(http.StatusBadRequest, "invalid_request", fmt.Sprintf("unable to stop containers %s", strings.Join(unstopped, ", ")))
}

func stopContainer(env *middleware.Env, cl freighterclient.Client, cn *container.Container) error {
	db := env.DB
	err := cn.Lock(1*time.Minute, db)
	if err != nil {
		return err
	}
	defer cn.Unlock(db)

	// it is ok to call stop on stopped container, so we don't need an extra
	// call to freighter to check state
	if _, err := cl.Stop(cn.CID); err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventContainerStopped, serializers.SerializeContainer(cn)),
		env.User.Topic(),
	)
	return nil
}
