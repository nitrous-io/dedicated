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
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
	"github.com/nitrous-io/nitrous-common/flusher"
)

var (
	CreateVolumeFn = volumes.CreateVolume

	errFreePlanNotAllowed = httperror.New(http.StatusForbidden, "free_plan_not_allowed", "You are not allowed to create a free workstation")
	errPaidPlanNotAllowed = httperror.New(http.StatusForbidden, "paid_plan_not_allowed", "You are not allowed to create a paid workstation")
	errMaxFreeHostReached = httperror.New(http.StatusForbidden, "max_free_host_reached", "You can't create any more free workstations")

	errSuspiciousUser = httperror.New(http.StatusForbidden, "invalid_request", "Unexpected error")

	errUserMaxHostsReached   = httperror.New(http.StatusForbidden, "max_hosts_reached", "you have reached the maximum number of workstations")
	errUserNoCard            = httperror.New(http.StatusForbidden, "request_not_allowed", "you have not added a credit card")
	errUserEmailNotConfirmed = httperror.New(http.StatusForbidden, "request_not_allowed", "please confirm your email address before proceeding")

	errOrgMaxHostsReached = httperror.New(http.StatusForbidden, "max_hosts_reached", "your organization has reached the maximum number of workstations")
	errOrgNoCard          = httperror.New(http.StatusForbidden, "request_not_allowed", "your organization has not added a credit card")
	errOrgDelinquent      = httperror.New(http.StatusForbidden, "outstanding_invoices", "your organization has outstanding invoices")

	errOrgPermissionDenied       = httperror.New(http.StatusForbidden, "org_forbidden", "You do not have access to this organization")
	errOrgCreatePermissionDenied = httperror.New(http.StatusForbidden, "org_forbidden", "You do not have permission to create workstations under this organization")
	errOrgNotFound               = httperror.New(http.StatusBadRequest, "invalid_request", "No matching org found")
	errNotOrgPlan                = httperror.New(http.StatusBadRequest, "invalid_request", "Not an org plan")

	errPlanMaxHostsReached = httperror.New(http.StatusForbidden, "max_hosts_reached", "You have exceeded the number of hosts for this plan, please upgrade")
)

// hostCreateRequest describes the JSON request used to create a host
type hostCreateRequest struct {
	Opts *createOpts `json:"host"`
}

// createOpts described options that can be used to customize a host
type createOpts struct {
	Size      string `json:"size"`
	Region    string `json:"region"`
	Slug      string `json:"slug"` // will be overriden by a generated slug based on name
	Name      string `json:"name"`
	Async     bool   `json:"async"`
	Org       string `json:"org"` // org slug
	PromoCode string `json:"promo_code"`

	// Container-related fields for when we want to create a container on the
	// host as well.
	ContainerName         string `json:"container_name"`
	ContainerTemplateSlug string `json:"container_template_slug"`

	// the following fields are only available to admin for now
	Provider     string `json:"provider"`      // use to change provider. Also look at TemplateName
	TemplateName string `json:"template_name"` // use to manually set provider_template
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	if !u.ConfirmedAt.Valid {
		return errUserEmailNotConfirmed
	}

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
		if options.Size != "" {
			pl, err = findPlan(db, options.Size)
			if err != nil {
				return err
			}
		}
	}

	var o *org.Org
	if options.Org != "" {
		o, err = org.FindBySlug(db, options.Org)
		if err != nil {
			if err == sql.ErrNoRows {
				return errOrgNotFound
			}
			return err
		}
	}

	bp, err := findBillingProfile(db, u, o)
	if err != nil {
		return err
	}

	var sub *subscription.Subscription
	if pl == nil {
		sub, err = subscription.FindMultiHostSubscription(db, bp.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				return errSizeRequired
			}
			return err
		}

		pl, err = plan.FindByID(db, sub.PlanID)
		if err != nil {
			if err == sql.ErrNoRows {
				return errSizeRequired
			}
			return err
		}
		// prevent misconfiguration issues
		if !pl.MaxHosts.Valid {
			return fmt.Errorf("The plan %d is linked to a MultiHost subscription (%d) but does not define MaxHosts.", pl.ID, sub.ID)
		}
	}

	if o != nil && !pl.Org {
		return errNotOrgPlan
	}

	// allocate a promo code claim for consumption on successful host creation
	// (where subscription is actually created)
	var pcUse *promocode.PromoCodeUse
	if sub == nil && options.PromoCode != "" {
		pcUse, err = promocode.Claim(db, options.PromoCode, u.ID, pl.ID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
	}

	if template == nil {
		template, err = findTemplate(db, pl, reg, options.Provider)
		if err != nil {
			return err
		}
	}

	// Check if user has any existing hosts.
	hosts, err := host.FindAllWithoutOrphaned(db, u.ID)
	if err != nil {
		return err
	}

	isOrg := (o != nil)
	hasHosts := len(hosts) > 0
	if err := hostCreateCheck(pl, bp.State, isOrg, hasHosts); err != nil {
		return err
	}

	// suspicious users cannot create workstations
	if u.Suspicious.Valid {
		return errSuspiciousUser
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
	handleProvisionFailure := func(h *host.Host, err error) {
		if pcUse != nil {
			if err := pcUse.Delete(db); err != nil {
				logrus.Errorf("Unable to delete promo code claim %d, err: %v", pcUse.ID, err)
			}
		}

		if e, ok := err.(*provisioner.HostbuilderError); ok {
			logrus.Errorf("hostbuilder error provisioning host: %v", e)
			// Change error into something we can display to end users.
			err = errors.New("failed to create workstation")
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
		Async:             options.Async,
		OutputStream:      outstream,
		Slug:              options.Slug,
		Name:              options.Name,
		User:              u,
		Region:            reg,
		Plan:              pl,
		Org:               o,
		Subscription:      sub,
		Template:          template,
		ProvisionFailedFn: handleProvisionFailure,
		UUID:              env.UUID,
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
		case org.ErrMaxHostsReached:
			return errOrgMaxHostsReached
		case org.ErrNoCard:
			return errOrgNoCard
		case org.ErrDelinquent:
			return errOrgDelinquent
		case provisioner.ErrOrgPermissionDenied:
			return errOrgPermissionDenied
		case provisioner.ErrOrgCreatePermissionDenied:
			return errOrgCreatePermissionDenied
		case user.ErrMaxHostsReached:
			return errUserMaxHostsReached
		case provisioner.ErrExceededPlanMaxHosts:
			return errPlanMaxHostsReached
		// Delinquent users are blocked by middleware,
		// and users with no card are directed to add
		// card page when attempting to create host.
		// Block no_card users in the backend anyway.
		case user.ErrNoCard:
			return errUserNoCard
		}
		return err
	}

	if options.Async {
		go func() {
			h := <-provisioned
			PublishHostEvent(env, controllers.EventHostCreated, h)
		}()
		env.Status = http.StatusAccepted
	} else {
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
			Org:                   r.FormValue("org"),
			PromoCode:             r.FormValue("promo_code"),
		}
		opts.Async, _ = strconv.ParseBool(r.FormValue("async"))
	}

	if opts.Region == "" {
		return nil, errRegionRequired
	}
	if opts.Name == "" {
		return nil, errNameRequired
	}

	// ignore user-provided slug. Use generated slug from name instead
	opts.Slug = slugbuilder.Build(opts.Name)
	if _, err := strconv.Atoi(opts.Slug); err == nil {
		// purely digits, use username instead
		opts.Slug = fmt.Sprintf("%s-%s", u.Username, opts.Slug)
	}

	if !u.Admin {
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

func findBillingProfile(db ndb.QueryReceiver, u *user.User, o *org.Org) (*billingprofile.BillingProfile, error) {
	if o != nil {
		return billingprofile.FindForOrg(db, o.ID)
	}

	return u.BillingProfile(db)
}

type condition struct {
	freePlan            bool
	billingProfileState string
	isOrg               bool
	hasHosts            bool
}

var (
	_freePlan, _paidPlan      = true, false
	_noCard, _isFree, _isPaid = billingprofile.StateNoCard, billingprofile.StateFree, billingprofile.StatePaid
	_isOrg, _indiv            = true, false
	_zeroHosts, _someHosts    = false, true

	decisionTable = map[condition]error{
		condition{_freePlan, _noCard, _indiv, _zeroHosts}: errFreePlanNotAllowed,
		condition{_freePlan, _isFree, _indiv, _zeroHosts}: nil,
		condition{_freePlan, _isPaid, _indiv, _zeroHosts}: nil,

		condition{_freePlan, _noCard, _indiv, _someHosts}: errMaxFreeHostReached,
		condition{_freePlan, _isFree, _indiv, _someHosts}: errMaxFreeHostReached,
		condition{_freePlan, _isPaid, _indiv, _someHosts}: errMaxFreeHostReached,

		condition{_freePlan, _noCard, _isOrg, _zeroHosts}: errFreePlanNotAllowed,
		condition{_freePlan, _isFree, _isOrg, _zeroHosts}: errFreePlanNotAllowed,
		condition{_freePlan, _isPaid, _isOrg, _zeroHosts}: errFreePlanNotAllowed,

		condition{_freePlan, _noCard, _isOrg, _someHosts}: errFreePlanNotAllowed,
		condition{_freePlan, _isFree, _isOrg, _someHosts}: errFreePlanNotAllowed,
		condition{_freePlan, _isPaid, _isOrg, _someHosts}: errFreePlanNotAllowed,

		condition{_paidPlan, _noCard, _indiv, _zeroHosts}: errUserNoCard,
		condition{_paidPlan, _isFree, _indiv, _zeroHosts}: errPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _indiv, _zeroHosts}: nil,

		condition{_paidPlan, _noCard, _indiv, _someHosts}: errUserNoCard,
		condition{_paidPlan, _isFree, _indiv, _someHosts}: errPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _indiv, _someHosts}: nil,

		condition{_paidPlan, _noCard, _isOrg, _zeroHosts}: errOrgNoCard,
		condition{_paidPlan, _isFree, _isOrg, _zeroHosts}: errPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _isOrg, _zeroHosts}: nil,

		condition{_paidPlan, _noCard, _isOrg, _someHosts}: errOrgNoCard,
		condition{_paidPlan, _isFree, _isOrg, _someHosts}: errPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _isOrg, _someHosts}: nil,
	}
)

func hostCreateCheck(pl *plan.Plan, bpState string, isOrg, hasHosts bool) error {
	cond := condition{
		freePlan:            pl.IsFree(),
		billingProfileState: bpState,
		isOrg:               isOrg,
		hasHosts:            hasHosts,
	}

	if e, ok := decisionTable[cond]; ok {
		return e
	}

	// TODO We should return an error for an unhandled condition after we
	// refactor so that "delinquent" and "payment_due" billing profile states
	// are also handled here.
	logrus.Warnf("unhandled condition determining if user can create host: %+v", cond)
	return nil
}
