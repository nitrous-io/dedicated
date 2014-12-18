package hosts

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nitrous-io/nitrous-common/flusher"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/images"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
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

type createOpts struct {
	Name       string               `json:"name"`
	Size       string               `json:"size"`
	Region     string               `json:"region"`
	Template   string               `json:"template"`
	Slug       string               `json:"slug"`
	Async      bool                 `json:"async"`
	Containers []template.Container `json:"containers"`
	Volumes    []template.Volume    `json:"volumes"`
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

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

	region, err := findRegion(db, options.Region)
	if err != nil {
		return err
	}
	plan, err := findPlan(db, options.Size)
	if err != nil {
		return err
	}
	template, err := findTemplate(db, plan, region, defaultProvider)
	if err != nil {
		return err
	}
	t, err := token.FindByUser(db, u.ID)
	if err != nil {
		return err
	}

	// Progress data from host provisioning is written to the events stream.
	// For synchronous requests, progress data is also written to the
	// ResponseWriter.
	var outstream io.Writer
	outstream = &controllers.ProgressPublisher{
		EventName: controllers.EventHostCreating,
		Topic:     u.Topic(),
		Events:    env.Events,
		Slug:      options.Slug,
	}
	if !options.Async {
		// Turn off nginx buffer caching to flush output to clients immediately.
		w.Header().Set("X-Accel-Buffering", "no")
		outstream = io.MultiWriter(outstream, flusher.NewWriteFlusher(w))
	}

	prov := env.Provisioner
	publishProvisionFailure := func(h *host.Host, err error) {
		env.Events.Publish(
			controllers.NewEvent(controllers.EventHostCreateFailed, map[string]interface{}{
				"host":  serializers.SerializeHost(h),
				"error": err.Error(),
			}),
			u.Topic(),
		)
	}
	provisioned := make(chan *host.Host, 1)
	h, err := prov.ProvisionHost(provisioner.ProvisionOptions{
		Async:             options.Async,
		OutputStream:      outstream,
		Slug:              options.Slug,
		Name:              options.Name,
		User:              u,
		Token:             t,
		Region:            region,
		Plan:              plan,
		Template:          template,
		ProvisionFailedFn: publishProvisionFailure,
		ProvisionSucceededFn: func(h *host.Host) {
			provisioned <- h
		},
	})
	if err != nil {
		if e, ok := err.(*provisioner.InvalidHostError); ok {
			return httperror.New(http.StatusBadRequest, "invalid_request", e.Err.Error())
		}
		switch err {
		case provisioner.ErrSlugIsTaken:
			return errHostSlugTaken
		case provisioner.ErrMultipleHostCreationNotAllowed:
			return errMultipleRequests
		case provisioner.ErrHostCreationNotAllowed:
			return errHostCreationNotAllowed
		}
		return err
	}

	if options.Async {
		go func() {
			h := <-provisioned
			if err := postProvision(options, env, h, outstream); err != nil {
				publishProvisionFailure(h, err)
				return
			}
			publishHostEvent(env, controllers.EventHostCreated, h)
		}()
		env.Status = http.StatusAccepted
	} else {
		if err := postProvision(options, env, h, outstream); err != nil {
			publishProvisionFailure(h, err)
			return err
		}
		publishHostEvent(env, controllers.EventHostCreated, h)
		env.Status = http.StatusCreated
	}
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}

	env.AddEventData(map[string]interface{}{
		"slug":    h.Slug,
		"plan":    plan.Name,
		"region":  region.Name,
		"host_id": h.ID,
	})

	return nil
}

func postProvision(options *createOpts, env *middleware.Env, h *host.Host, outstream io.Writer) error {
	if err := createSharedVolume(env, env.DB, h); err != nil {
		return err
	}

	// Setup host according to given template, if required.
	// We might want to consider moving this into the provisioner and also
	// removing support for the volumes and containers options.
	if err := setupFromTemplate(options.Template, env, h, outstream); err != nil {
		return err
	}

	if err := createVolumes(options.Volumes, env, h, outstream); err != nil {
		return err
	}
	if err := createContainers(options.Containers, env, h, outstream); err != nil {
		return err
	}
	if err := recordTransition(env, h); err != nil {
		env.Logger.Errorf("Unable to record transition for host %d: %v", h.ID, err)
	}
	return nil
}

func recordTransition(env *middleware.Env, h *host.Host) error {
	timing := h.AssignedAt
	if timing == nil {
		timing = h.CreatedAt
	}
	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := hosttransition.Record(tx, h, host.StateBorn, *timing); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func buildCreateOptions(r *http.Request) (*createOpts, error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var j struct {
			Opts createOpts `json:"host"`
		}
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			return nil, err
		}
		j.Opts.Slug = strings.ToLower(j.Opts.Slug)
		return &j.Opts, nil
	}

	opts := &createOpts{
		Size:     r.FormValue("size"),
		Region:   r.FormValue("region"),
		Template: r.FormValue("template"),
		Slug:     strings.ToLower(r.FormValue("slug")),
		Name:     r.FormValue("name"),
	}
	opts.Async, _ = strconv.ParseBool(r.FormValue("async"))
	return opts, nil
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
	caInfo, err := certauth.GetCAInfo(true)
	if err != nil {
		return nil, err
	}
	return cert.Generate(hostname, caInfo.X509Cert, caInfo.Key)
}

func setupFromTemplate(slug string, env *middleware.Env, h *host.Host, w io.Writer) error {
	if slug == "" {
		return nil // Ignore if slug is blank.
	}

	tmpl, err := template.Find(env.DB, slug)
	if err != nil {
		return err
	}

	cfg, err := tmpl.ParsedConfig()
	if err != nil {
		return err
	}

	if err := createVolumes(cfg.Volumes, env, h, w); err != nil {
		return err
	}

	containers := []template.Container{}
	for _, c := range cfg.Containers {
		suffix, err := securerandom.Base64(6)
		if err != nil {
			return err
		}
		containers = append(containers, template.Container{
			Name:       c.Name,
			Slug:       fmt.Sprintf("%s-%s", c.Slug, suffix),
			Image:      c.Image,
			Cmd:        c.Cmd,
			Privileged: c.Privileged,
			PublishAll: c.PublishAll,
			Env:        c.Env,
			Ports:      c.Ports,
			Volumes:    c.Volumes,
		})
	}

	if err := createContainers(containers, env, h, w); err != nil {
		return err
	}

	return nil
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

func createContainers(containers []template.Container, env *middleware.Env, h *host.Host, w io.Writer) error {
	for i, c := range containers {
		options := url.Values{
			"name":        []string{c.Name},
			"slug":        []string{c.Slug},
			"image":       []string{c.Image},
			"env[]":       c.Env,
			"volume[]":    c.Volumes,
			"port[]":      c.Ports,
			"cmd":         []string{c.Cmd},
			"publish_all": []string{strconv.FormatBool(c.PublishAll)},
		}

		state := fmt.Sprintf(`{"status": "Creating container %d of %d"}`, i+1, len(containers))
		stream := flusher.NewWriteFlusher(w)
		fmt.Fprintln(stream, state)

		// FIXME Get rid of this awkward func that we introduced for tests.
		if _, err := PullImageFn(h, url.Values{"repository": options["image"]}, env, w, nil, false); err != nil {
			env.Logger.Errorf("host.Create: error pulling container image: %v", err)
			return err
		}

		// FIXME Get rid of this awkward func that we introduced for tests.
		if _, err := CreateContainerFn(h, options, env); err != nil {
			env.Logger.Errorf("host.Create: error creating container: %v", err)
			return errContainerCreationFailed
		}
	}
	return nil
}

func createVolumes(volumes []template.Volume, env *middleware.Env, h *host.Host, w io.Writer) error {
	for i, v := range volumes {
		options := url.Values{
			"name": []string{v.Name},
			"slug": []string{v.Slug},
		}

		state := fmt.Sprintf(`{"status": "Creating volume %d of %d"}`, i+1, len(volumes))
		stream := flusher.NewWriteFlusher(w)
		fmt.Fprintln(stream, state)

		// FIXME Get rid of this awkward func that we introduced for tests.
		_, err := CreateVolumeFn(h, options, env)
		if err != nil {
			env.Logger.Errorf("host.Create: error creating volume: %v", err)
			return errVolumeCreationFailed
		}
	}
	return nil
}
