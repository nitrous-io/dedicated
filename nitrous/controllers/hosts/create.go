package hosts

import (
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous-common/flusher"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/hostbuilder"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/images"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errContainerCreationFailed = httperror.New(http.StatusInternalServerError, "internal_server_error", "one of the containers failed during creation")
	errVolumeCreationFailed    = httperror.New(http.StatusInternalServerError, "internal_server_error", "one of the volumes failed during creation")
)

var (
	CreateContainerFn = containers.CreateContainer
	CreateVolumeFn    = volumes.CreateVolume
	PullImageFn       = images.PullImage
)

type volumeCreate struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type containerCreate struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	Env        []string `json:"env"`
	Volumes    []string `json:"volumes"`
	Ports      []string `json:"ports"`
	Cmd        string   `json:"cmd"`
	PublishAll bool     `json:"publish_all"`
}

type hostCreate struct {
	Name       string            `json:"name"`
	Size       string            `json:"size"`
	Region     string            `json:"region"`
	Slug       string            `json:"slug"`
	Async      bool              `json:"async"`
	Containers []containerCreate `json:"containers"`
	Volumes    []volumeCreate    `json:"volumes"`
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	options, err := buildCreateOptions(r)
	if err != nil {
		return err
	}

	if options.Size == "" {
		return errSizeRequired
	}
	if options.Region == "" {
		return errRegionRequired
	}
	if options.Slug == "" {
		return errSlugRequired
	}
	if options.Name == "" {
		options.Name = options.Slug
	}

	ctx, err := newCreateContext(db, env.User, options.Size, options.Region, options.Slug, options.Name)
	if err != nil {
		return err
	}

	async := options.Async
	// if async, we will just discard the progress data from hostbuilder. It will
	// only be outputed into the events stream. The only output we want to
	// include in async is the host data
	var writer io.Writer
	pp := &controllers.ProgressPublisher{
		EventName: controllers.EventHostCreating,
		Topic:     env.User.Topic(),
		Events:    env.Events,
		ID:        ctx.Host.ID,
		Slug:      ctx.Host.Slug,
	}
	if async {
		writer = pp
	} else {
		// turn off nginx buffer caching so that we can flush output to the
		// clients immediately
		w.Header().Set("X-Accel-Buffering", "no")
		writer = io.MultiWriter(pp, flusher.NewWriteFlusher(w))
	}
	h, err := ctx.run(env, writer, async, options.Containers, options.Volumes)
	if err != nil {
		return err
	}
	if async {
		env.Status = http.StatusAccepted
	} else {
		env.Status = http.StatusCreated
	}
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}

	env.AddEventData(map[string]interface{}{
		"slug":    h.Slug,
		"plan":    ctx.Plan.Name,
		"region":  ctx.Region.Name,
		"host_id": h.ID,
	})

	return nil
}

func buildCreateOptions(r *http.Request) (hostCreate, error) {
	var nested struct {
		Host hostCreate `json:"host"`
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&nested)
		if err == nil {
			nested.Host.Slug = strings.ToLower(nested.Host.Slug)
		}
		return nested.Host, err
	} else {
		nested.Host.Size = r.FormValue("size")
		nested.Host.Region = r.FormValue("region")
		nested.Host.Slug = strings.ToLower(r.FormValue("slug"))
		nested.Host.Name = r.FormValue("name")
		async, _ := strconv.ParseBool(r.FormValue("async"))
		nested.Host.Async = async
		return nested.Host, nil
	}
}

// createContext handles the creation of host record and its dependencies in
// database and propagate the changes to host builder.
type createContext struct {
	User     *user.User
	Plan     *plan.Plan
	Region   *region.Region
	Template *providertemplate.ProviderTemplate
	Keypair  *keypairs.Keypair
	Cert     *cert.Cert
	Host     *host.Host
	Token    *token.Token
	Pubkey   []byte

	db *ndb.Database
}

// run propagates the changes to hostbuilder. If we want the creation to
// happens asynchronously, the current host infomation will be returned
// immediately while we build the host. Otherwise, the default is building the
// host asynchronously
func (ctx *createContext) run(env *middleware.Env, writer io.Writer, async bool, containers []containerCreate, volumes []volumeCreate) (*host.Host, error) {
	h, db := ctx.Host, ctx.db

	// get ca to save to the host
	caInfo, err := controllers.GetCAInfo(false)
	if err != nil {
		return nil, err
	}

	// generate hostbuilder config
	config := hostbuilderclient.Config{
		"name":         h.Slug,
		"secret":       h.Secret,
		"public_keys":  []string{string(ctx.Pubkey)},
		"apt_username": ctx.User.Username,
		"apt_password": ctx.Token.Token,
		"apt_location": defaultAPTLocation,
		"apt_dist":     defaultAPTDist,
		"tls": map[string]string{
			"ca":   string(caInfo.CertPEM),
			"cert": ctx.Cert.ServerCert,
			"key":  ctx.Cert.ServerKey,
		},
	}
	for k, v := range ctx.Template.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}

	createHost := func() error {
		var err error
		defer func() {
			if err != nil {
				h.State = host.StateOrphan
				_ = h.Update(db) // Ignore error here.
				env.Events.Publish(
					controllers.NewEvent(controllers.EventHostCreateFailed, map[string]interface{}{
						"host":  serializers.SerializeHost(h),
						"error": err.Error(),
					}),
					env.User.Topic(),
				)
			}
		}()

		var info *hostbuilder.Response
		info, err = HostbuilderClient.Create(ctx.Template.Provider, &config, writer)
		if err != nil {
			return err
		}

		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{info.ID, true}
		h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
		h.RegionSlug = sql.NullString{info.RegionID, true}
		h.State = host.StateRunning

		if err = h.Update(db); err != nil {
			return err
		}

		if err = createSharedVolume(env, db, h); err != nil {
			return err
		}

		if err = createVolumes(volumes, env, h, writer); err != nil {
			return err
		}

		err = createContainers(containers, env, h, writer)

		publishHostEvent(env, controllers.EventHostCreated, h)
		return err
	}

	if async {
		go createHost()
		return h, nil
	}
	err = createHost()
	return h, err
}

// create a new createContext with user-supplied parameters. This will tries to
// create a host with proper plan, region, template, etc..., associate them
// with the current context and save them to database
//
// TODO: check if user account is eligible to create hosts (has a credit card on file)
func newCreateContext(db *ndb.Database, user *user.User, size, regionSlug, slug, name string) (*createContext, error) {
	var err error

	ctx := &createContext{
		User: user,
		db:   db,
	}

	if ctx.Plan, err = findPlan(db, size); err != nil {
		return nil, err
	}
	if ctx.Region, err = findRegion(db, regionSlug); err != nil {
		return nil, err
	}
	if ctx.Template, err = findTemplate(db, ctx.Plan, ctx.Region, defaultProvider); err != nil {
		return nil, err
	}

	// Assuming that t is not nil cos if user gets here,
	// that means she has a token, but return err if there's
	// some other unexpected error
	if ctx.Token, err = token.FindByUser(db, user.ID); err != nil {
		return nil, err
	}

	// generate the keypair to communicate with the host
	priv, pubkey, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypairs for '%s', err %s", slug, err)
	}

	ctx.Pubkey = pubkey
	ctx.Host = &host.Host{
		UserID:             ctx.User.ID,
		RegionID:           ctx.Region.ID,
		PlanID:             ctx.Plan.ID,
		ProviderTemplateID: ctx.Template.ID,
		Slug:               slug,
		Name:               sql.NullString{name, true},
		MaxContainers:      sql.NullInt64{ctx.Plan.MaxContainers, true},
		Size:               sql.NullString{ctx.Plan.Slug, true},
		State:              host.StateBorn,
	}

	if err = ctx.Host.Validate(); err != nil {
		return nil, httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("failed to open a transaction for host %s, err: %s", slug, err)
	}

	// don't allow hosts to be created concurrently
	locked, err := ndb.TransactionLock(tx, ndb.HOST_CREATE_LOCK, int(user.ID))
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if !locked {
		tx.Rollback()
		return nil, errMultipleRequests
	}

	canCreateHosts, err := user.CanCreateHosts(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if !canCreateHosts {
		tx.Rollback()
		return nil, errHostCreationNotAllowed
	}

	if err := ctx.Host.Insert(tx); err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return nil, validationErr
			}
		}
		return nil, fmt.Errorf("failed to insert new host %s, err: %s", slug, err)
	}

	// create the cert
	if ctx.Cert, err = generateCerts(ctx.Host.Hostname()); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to generate certificates for '%s', err %s", slug, err)
	}
	if err := ctx.Cert.Insert(tx, ctx.Host.ID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to save certificates for '%s', err %s", slug, err)
	}

	// create the keypair
	ctx.Keypair, err = keypairs.Create(tx, priv, aesKey, ctx.Host.ID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to save keypairs for '%s', err %s", slug, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to complete save transaction for '%s', err %s", slug, err)
	}

	return ctx, nil
}

func findPlan(db ndb.QueryReceiver, size string) (*plan.Plan, error) {
	pl, err := plan.FindBySlug(db, size)
	if err != nil {
		return nil, fmt.Errorf("no matching plan for '%s', error %s", size, err)
	}
	if pl == nil {
		return nil, errPlanNotFound
	}
	return pl, nil
}

func findRegion(db ndb.QueryReceiver, regionSlug string) (*region.Region, error) {
	r, err := region.FindBySlug(db, regionSlug)
	if err != nil {
		return nil, fmt.Errorf("no matching region for '%s', error %s", regionSlug, err)
	}
	if r == nil {
		return nil, errRegionNotFound
	}
	return r, nil
}

func findTemplate(db ndb.QueryReceiver, pl *plan.Plan, rg *region.Region, defaultProvider string) (*providertemplate.ProviderTemplate, error) {
	pt, err := providertemplate.FindByRegionAndPlanAndProvider(db, rg.ID, pl.ID, defaultProvider)
	if err != nil {
		return nil, fmt.Errorf("no matching templates found, error %s", err)
	}
	if pt == nil {
		return nil, errProvidertemplateNotFound
	}
	return pt, nil
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

// generate cert with server certificate and key. The cert will be used for
// TLS encryption
func generateCerts(hostname string) (*cert.Cert, error) {
	caInfo, err := controllers.GetCAInfo(true)
	if err != nil {
		return nil, err
	}
	return cert.Generate(hostname, caInfo.X509Cert, caInfo.Key)
}

func createSharedVolume(env *middleware.Env, db *ndb.Database, h *host.Host) error {
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	vol := &volume.Volume{
		HostID: h.ID,
		Slug:   "shared",
		Name:   "Shared Volume",
		System: true,
	}

	if err := vol.Insert(db); err != nil {
		return err
	}

	if _, err := cl.CreateVolume(vol.Slug); err != nil {
		if e := vol.Delete(db); e != nil {
			env.Logger.Errorf("Unable to delete volume %d that failed on freighter: %v", vol.ID, e)
		}
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventVolumeCreated, map[string]*serializers.Volume{
			"volume": serializers.SerializeVolume(vol),
		}),
		env.User.Topic(),
	)

	return nil
}

func createContainers(containers []containerCreate, env *middleware.Env, h *host.Host, writer io.Writer) error {
	for index, container := range containers {
		options := url.Values{
			"name":        []string{container.Name},
			"slug":        []string{container.Slug},
			"image":       []string{container.Image},
			"env[]":       container.Env,
			"volume[]":    container.Volumes,
			"port[]":      container.Ports,
			"cmd":         []string{container.Cmd},
			"publish_all": []string{strconv.FormatBool(container.PublishAll)},
		}

		state := fmt.Sprintf(`{"status": "Creating container %d of %d"}`, index+1, len(containers))
		stream := flusher.NewWriteFlusher(writer)
		fmt.Fprintln(stream, state)
		_, err := PullImageFn(h, url.Values{"repository": options["image"]}, env, writer, nil, false)
		if err != nil {
			return err
		}

		_, err = CreateContainerFn(h, options, env)
		if err != nil {
			return errContainerCreationFailed
		}
	}
	return nil
}

func createVolumes(volumes []volumeCreate, env *middleware.Env, h *host.Host, writer io.Writer) error {
	for index, volume := range volumes {
		options := url.Values{
			"name": []string{volume.Name},
			"slug": []string{volume.Slug},
		}

		state := fmt.Sprintf(`{"status": "Creating volume %d of %d"}`, index+1, len(volumes))
		stream := flusher.NewWriteFlusher(writer)
		fmt.Fprintln(stream, state)
		_, err := CreateVolumeFn(h, options, env)

		if err != nil {
			return errVolumeCreationFailed
		}
	}
	return nil
}
