package hosts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
	"github.com/nitrous-io/nitrous-common/flusher"
)

var (
	CreateVolumeFn = volumes.CreateVolume

	errFreePlanNotAllowed = httperror.New(http.StatusForbidden, "free_plan_not_allowed",
		"You are not allowed to create a free tier workspace")
	errPaidPlanNotAllowed = httperror.New(http.StatusForbidden, "paid_plan_not_allowed",
		"You are not allowed to create a paid workspace")
	errMaxFreeHostReached = httperror.New(http.StatusForbidden, "max_free_host_reached",
		"You can't create any more free workspaces")
	errPhoneAuthRequired = httperror.New(http.StatusForbidden, "phone_auth_required",
		"You need to verify your phone before creating free workspace")
)

// hostCreateRequest describes the JSON request used to create a host
type hostCreateRequest struct {
	Opts *createOpts `json:"host"`
}

// createOpts described options that can be used to customize a host
type createOpts struct {
	Size   string `json:"size"`
	Region string `json:"region"`
	Slug   string `json:"slug"` // will be overriden by a generated slug based on name
	Name   string `json:"name"`
	Async  bool   `json:"async"`

	// Container-related fields for when we want to create a container on the
	// host as well.
	ContainerName         string `json:"container_name"`
	ContainerTemplateSlug string `json:"container_template_slug"`

	// the following fields are only available to admin for now
	Provider           string `json:"provider"`            // use to change provider. Also look at TemplateName
	BypassPreprovision bool   `json:"bypass_preprovision"` // use to ignore pre-provisioned host
	TemplateName       string `json:"template_name"`       // use to manually set provider_template
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	options, err := buildCreateOptions(r, u)
	if err != nil {
		return err
	}

	var (
		reg      *region.Region
		pl       *plan.Plan
		template *providertemplate.ProviderTemplate
	)

	// if template name is specified, we use it to locate the correct plan,
	// region and provider template. This is an admin-only option.
	if options.TemplateName != "" {
		template, err = providertemplate.FindByName(db, options.TemplateName)
		if err != nil {
			return err
		}
		reg, err = region.FindByID(db, template.RegionID)
		if err != nil {
			return err
		}
		pl, err = plan.FindByID(db, template.PlanID)
		if err != nil {
			return err
		}
	} else {
		reg, err = findRegion(db, options.Region)
		if err != nil {
			return err
		}
		pl, err = findPlan(db, options.Size)
		if err != nil {
			return err
		}
		template, err = findTemplate(db, pl, reg, options.Provider)
		if err != nil {
			return err
		}
	}

	err = freetier.CreateHostWithPlanAllowed(db, u, pl)
	if err != nil {
		switch err {
		case freetier.ErrFreePlanNotAllowed:
			return errFreePlanNotAllowed
		case freetier.ErrPaidPlanNotAllowed:
			return errPaidPlanNotAllowed
		case freetier.ErrMaxFreeHostReached:
			return errMaxFreeHostReached
		case freetier.ErrFreeTimeLimitExceeded:
			return errFreeTimeLimitExceeded
		case freetier.ErrPhoneAuthRequired:
			return errPhoneAuthRequired
		default:
			return err
		}
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
		if e, ok := err.(*provisioner.HostbuilderError); ok {
			logrus.Errorf("hostbuilder error provisioning host: %v", e)
			// Change error into something we can display to end users.
			err = errors.New("failed to create workspace")
		}

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
		Async:              options.Async,
		BypassPreprovision: options.BypassPreprovision,
		OutputStream:       outstream,
		Slug:               options.Slug,
		Name:               options.Name,
		User:               u,
		Region:             reg,
		Plan:               pl,
		Template:           template,
		ProvisionFailedFn:  publishProvisionFailure,
		UUID:               env.UUID,
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
			if err := postProvision(env, h); err != nil {
				publishProvisionFailure(h, err)
				return
			}
			PublishHostEvent(env, controllers.EventHostCreated, h)
		}()
		env.Status = http.StatusAccepted
	} else {
		if err := postProvision(env, h); err != nil {
			publishProvisionFailure(h, err)
			return err
		}
		PublishHostEvent(env, controllers.EventHostCreated, h)
		env.Status = http.StatusCreated
	}

	trackWorkspaces(env, h.UserID)

	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}

	env.AddEventData(map[string]interface{}{
		"plan":   pl.Slug,
		"region": reg.Slug,
	})

	return nil
}

func postProvision(env *middleware.Env, h *host.Host) error {
	if err := createSharedVolume(env, env.DB, h); err != nil {
		return err
	}
	if err := recordTransition(env, h); err != nil {
		logrus.Errorf("Unable to record transition for host %d: %v", h.ID, err)
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
	if err := hosttransition.Record(tx, h, host.StateBorn, *timing, env.User.ID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func buildCreateOptions(r *http.Request, u *user.User) (opts *createOpts, err error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		j := &hostCreateRequest{}
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			return nil, err
		}
		opts = j.Opts
	} else {
		opts = &createOpts{
			Size:                  r.FormValue("size"),
			Region:                r.FormValue("region"),
			Name:                  r.FormValue("name"),
			Provider:              r.FormValue("provider"),
			TemplateName:          r.FormValue("template_name"),
			ContainerTemplateSlug: r.FormValue("container_template_slug"),
			ContainerName:         r.FormValue("container_name"),
		}
		opts.Async, _ = strconv.ParseBool(r.FormValue("async"))
		opts.BypassPreprovision, _ = strconv.ParseBool(r.FormValue("bypass_preprovision"))
	}

	if opts.Size == "" {
		return nil, errSizeRequired
	}
	if opts.Region == "" {
		return nil, errRegionRequired
	}
	if opts.Name == "" {
		return nil, errNameRequired
	}

	// ignore user-provided slug. Use generated slug from name instead
	opts.Slug = slugbuilder.Build(opts.Name)
	if !u.Admin {
		// if user is not admin, always enforce bypass_preprovision
		opts.BypassPreprovision = false
		// if user is not admin, reject custom template name
		opts.TemplateName = ""
	}
	// if user is not admin, rejects the custom provider
	if opts.Provider == "" || !u.Admin {
		opts.Provider = defaultProvider
	}
	return opts, nil
}

func findPlan(db ndb.QueryReceiver, size string) (*plan.Plan, error) {
	pl, err := plan.FindBySlug(db, size)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errPlanNotFound
		}
		return nil, fmt.Errorf("no matching plan for '%s', error %s", size, err)
	}
	return pl, nil
}

func findRegion(db ndb.QueryReceiver, regionSlug string) (*region.Region, error) {
	r, err := region.FindBySlug(db, regionSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errRegionNotFound
		}
		return nil, fmt.Errorf("no matching region for '%s', error %s", regionSlug, err)
	}
	if r.Disabled() {
		return nil, errRegionDisabled
	}
	return r, nil
}

func findTemplate(db ndb.QueryReceiver, pl *plan.Plan, rg *region.Region, provider string) (*providertemplate.ProviderTemplate, error) {
	pt, err := providertemplate.Find(db, rg, pl, provider)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errProvidertemplateNotFound
		}
		return nil, fmt.Errorf("no matching templates found, error %s", err)
	}
	return pt, nil
}

// generate cert with server certificate and key. The cert will be used for
// TLS encryption
func generateCerts(hostname string) (*cert.Cert, error) {
	caInfo, err := certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath)
	if err != nil {
		return nil, err
	}
	return cert.Generate(hostname, caInfo.X509Cert, caInfo.Key)
}

func createSharedVolume(env *middleware.Env, db *ndb.Database, h *host.Host) error {
	cl, err := controllers.NewFreighterClient(db, h, env.UUID)
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
			logrus.Errorf("Unable to delete volume %d that failed on freighter: %v", vol.ID, e)
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
