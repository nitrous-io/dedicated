package server

import (
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/controllers/amazon"
	"github.com/nitrous-io/dedicated/nitrous/controllers/announcements"
	"github.com/nitrous-io/dedicated/nitrous/controllers/appsettings"
	"github.com/nitrous-io/dedicated/nitrous/controllers/billing"
	"github.com/nitrous-io/dedicated/nitrous/controllers/billingprofiles"
	"github.com/nitrous-io/dedicated/nitrous/controllers/bonuses"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containercollabs"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containerkeys"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/coupons"
	"github.com/nitrous-io/dedicated/nitrous/controllers/dockerregistry"
	"github.com/nitrous-io/dedicated/nitrous/controllers/events"
	"github.com/nitrous-io/dedicated/nitrous/controllers/feedback"
	"github.com/nitrous-io/dedicated/nitrous/controllers/freighterd"
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/controllers/home"
	"github.com/nitrous-io/dedicated/nitrous/controllers/hosts"
	"github.com/nitrous-io/dedicated/nitrous/controllers/images"
	"github.com/nitrous-io/dedicated/nitrous/controllers/mailchimphooks"
	"github.com/nitrous-io/dedicated/nitrous/controllers/maintenance"
	"github.com/nitrous-io/dedicated/nitrous/controllers/oauth"
	"github.com/nitrous-io/dedicated/nitrous/controllers/orgs"
	"github.com/nitrous-io/dedicated/nitrous/controllers/passwords"
	"github.com/nitrous-io/dedicated/nitrous/controllers/pendingmembers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/pinger"
	"github.com/nitrous-io/dedicated/nitrous/controllers/plans"
	"github.com/nitrous-io/dedicated/nitrous/controllers/promocodes"
	"github.com/nitrous-io/dedicated/nitrous/controllers/providertemplates"
	"github.com/nitrous-io/dedicated/nitrous/controllers/quickstart"
	"github.com/nitrous-io/dedicated/nitrous/controllers/regions"
	"github.com/nitrous-io/dedicated/nitrous/controllers/registries"
	"github.com/nitrous-io/dedicated/nitrous/controllers/snapshots"
	"github.com/nitrous-io/dedicated/nitrous/controllers/stripehooks"
	"github.com/nitrous-io/dedicated/nitrous/controllers/subscriptions"
	"github.com/nitrous-io/dedicated/nitrous/controllers/templates"
	"github.com/nitrous-io/dedicated/nitrous/controllers/twitter"
	"github.com/nitrous-io/dedicated/nitrous/controllers/usercontainerdata"
	"github.com/nitrous-io/dedicated/nitrous/controllers/userkeys"
	"github.com/nitrous-io/dedicated/nitrous/controllers/userprofiles"
	"github.com/nitrous-io/dedicated/nitrous/controllers/users"
	"github.com/nitrous-io/dedicated/nitrous/controllers/usersettings"
	"github.com/nitrous-io/dedicated/nitrous/controllers/usersurvey"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	mw "github.com/nitrous-io/dedicated/nitrous/middleware"
)

type routeData struct {
	mw        mw.Middleware
	track     string
	label     string
	logParams bool
}

// r is a helper that returns routeData. A route should always have label which
// acts as a metric label for that route
func r(label string, mw mw.Middleware, args ...string) routeData {
	var track string
	if len(args) > 0 {
		track = args[0]
	}
	return routeData{mw: mw, track: track, label: label, logParams: true}
}

func rNoLog(label string, mw mw.Middleware, args ...string) routeData {
	data := r(label, mw, args...)
	data.logParams = false
	return data
}

// drawRouteData draw the route with `route`.
// if `auth` is set then we setup the auth middleware for it
// if `track` for a route is set then we setup the track middleware for it
func (s *Server) drawRouteData(method string, auth bool, routes map[string]routeData) {
	for path, r := range routes {
		s.route(path, routeStrictJSON, chainMiddleware(auth, r), RouteOpts{Methods: []string{method}, LogParams: r.logParams})
	}
}

func chainMiddleware(auth bool, r routeData) mw.Middleware {
	final := mw.Call("nitrous."+r.label, r.mw)
	if r.track != "" {
		final = mw.Track(r.track, final)
	}
	if auth {
		final = mw.VerifyUserBilling(r.label, final)
		final = mw.Auth(final)
	}
	return final
}

func (s *Server) drawRoutes() {
	s.route("/", routeWritesResponse, mw.Call("home.Index", home.Index), RouteOpts{Methods: []string{"GET"}, LogParams: true})
	s.route("/api/v0/github/oauth_cb", routeWritesResponse, mw.Call("github.OAuthCallback", github.OAuthCallback), RouteOpts{Methods: []string{"GET"}, LogParams: true})
	s.route("/api/v0/twitter/oauth_cb", routeWritesResponse, mw.Call("twitter.OAuthCallback", twitter.OAuthCallback), RouteOpts{Methods: []string{"GET"}, LogParams: true})
	s.route("/quickstart", routeWritesResponse, mw.Call("quickstart.New", quickstart.New), RouteOpts{Methods: []string{"GET"}, LogParams: true})
	s.route("/api/v0/usersurvey/webhook_cb", routeWritesResponse, mw.Call("usersurvey.WebhookCallback", usersurvey.WebhookCallback), RouteOpts{Methods: []string{"POST"}, LogParams: true})

	if envvars.Debug == "true" {
		s.router.HandleFunc("/sleep", s.sleep).Methods("GET")
	}

	s.namespace("/api/v0", func() {
		s.route("/events", routeWritesResponse, mw.AuthQS(mw.Call("events.Stream", events.Stream)), RouteOpts{Methods: []string{"GET"}, LogParams: true})
		s.route("/hosts/{id}/activity", routeWritesResponse, mw.AuthQS(mw.Call("pinger.ForHost", pinger.ForHost)), RouteOpts{Methods: []string{"GET"}, LogParams: true})
		s.route("/oauth/authorize", routeJSON, chainMiddleware(false, r("oauth.GetAuthorize", oauth.GetAuthorize)), RouteOpts{Methods: []string{"GET"}, LogParams: true})

		nonAuth := map[string]map[string]routeData{
			"GET": {
				"/plans/all":                        r("plans.All", plans.All),
				"/regions":                          r("regions.Index", regions.Index),
				"/templates":                        r("templates.Index", templates.Index),
				"/app_settings":                     r("appsettings.Index", appsettings.Index),
				"/promo_codes/{code}":               r("promocodes.Show", promocodes.Show),
				"/hosts/{slug}/apt_allowed":         r("hosts.AptAllowed", hosts.AptAllowed),
				"/mailchimp_hook":                   r("mailchimphooks.VerifyWebhook", mailchimphooks.VerifyWebhook),
				"/subscriptions/promo_codes/{slug}": r("promocodes.ShowSubscriptionPromocode", promocodes.ShowSubscriptionPromocode),
				"/pending_members/{code}":           r("pendingmembers.Show", pendingmembers.Show),
			},
			"POST": {
				"/oauth/token":                   r("oauth.IssueToken", oauth.IssueToken),
				"/users/email_signup":            r("users.EmailCapture", users.EmailCapture),
				"/users":                         r("users.Create", users.Create),
				"/user/confirm":                  r("users.Confirm", users.Confirm),
				"/user/resend_confirm":           r("users.ResendConfirmation", users.ResendConfirmation),
				"/send_reset_password_token":     r("passwords.SendResetPasswordToken", passwords.SendResetPasswordToken),
				"/reset_password":                r("passwords.ResetPassword", passwords.ResetPassword),
				"/stripe_hook":                   r("stripehooks.ReceiveHook", stripehooks.ReceiveHook),
				"/hosts/{slug}/freighterd_hook":  r("freighterd.Webhook", freighterd.Webhook),
				"/mailchimp_hook":                r("mailchimphooks.ReceiveHook", mailchimphooks.ReceiveHook),
				"/docker/registry/notifications": r("dockerregistry.NotificationWebhook", dockerregistry.NotificationWebhook),
				"/github/authorize_to_login":     r("github.AuthorizeToLogin", github.AuthorizeToLogin),
				"/github/authorize_to_signup":    r("github.AuthorizeToSignUp", github.AuthorizeToSignUp),
			},
		}

		for method, routes := range nonAuth {
			s.drawRouteData(method, false, routes)
		}

		authed := map[string]map[string]routeData{
			"GET": {
				"/user":                                    r("users.GetAuthenticatedUser", users.GetAuthenticatedUser),
				"/user/exists":                             r("users.Exists", users.Exists),
				"/user/profile":                            r("userprofiles.Show", userprofiles.Show),
				"/user/settings":                           r("usersettings.Show", usersettings.Show),
				"/user/billing_profile":                    r("billingprofiles.Show", billingprofiles.Show),
				"/user/can_delete":                         r("users.CheckCanDelete", users.CheckCanDelete),
				"/user/login_token":                        r("users.GetLoginToken", users.GetLoginToken),
				"/user/promo_codes":                        r("promocodes.PendingPromoCodes", promocodes.PendingPromoCodes),
				"/user/earned_bonuses":                     r("bonuses.EarnedByUser", bonuses.EarnedByUser),
				"/bonuses":                                 r("bonuses.Index", bonuses.Index),
				"/plans":                                   r("plans.Index", plans.Index),
				"/hosts":                                   r("hosts.Index", hosts.Index),
				"/images":                                  r("images.IndexAll", images.Index),
				"/hosts/{slug}/images":                     r("images.Index", images.Index),
				"/hosts/{slug}":                            r("hosts.Show", mw.SharedHostViaPath("slug", hosts.Show)),
				"/hosts/{slug}/stats":                      r("hosts.Stats", mw.HostViaPath("slug", hosts.Stats)),
				"/hosts/{slug}/announcements":              r("hosts.Announcements", mw.HostViaPath("slug", hosts.Announcements)),
				"/hosts/{slug}/can_create_containers":      r("hosts.CanCreateContainers", mw.HostViaPath("slug", hosts.CanCreateContainers)),
				"/hosts/{slug}/volumes":                    r("volumes.Index", mw.HostViaPath("slug", volumes.Index)),
				"/hosts/{slug}/volumes/{vslug}":            r("volumes.Show", mw.HostViaPath("slug", volumes.Show)),
				"/hosts/{slug}/volumes/{vslug}/info":       r("volumes.Info", mw.HostViaPath("slug", volumes.Info)),
				"/hosts/{slug}/images/{image:.*}":          r("images.Inspect", mw.HostViaPath("slug", images.Inspect)),
				"/hosts/{slug}/registries":                 r("registries.Index", registries.Index),
				"/containers":                              r("containers.Index", containers.Index),
				"/shared_containers":                       r("containers.Shared", containers.Shared),
				"/containers/{slug}":                       r("containers.Show", containers.Show),
				"/containers/{slug}/config":                r("containers.Config", containers.Config),
				"/containers/{slug}/keys":                  r("userkeys.Index", userkeys.Index),
				"/containers/{slug}/container_keys":        r("containerkeys.Index", containerkeys.Index, EventContainerKeyListed),
				"/containers/{slug}/container_keys/github": r("containerkeys.GetGitHubSSHKeys", containerkeys.GetGitHubSSHKeys, EventContainerGitHubKeyListed),
				"/containers/{slug}/container_keys/status": r("containerkeys.Status", containerkeys.Status),
				"/containers/{slug}/collaborators":         r("containercollabs.Index", containercollabs.Index),
				"/billing/invoices":                        r("billing.Invoices", billing.Invoices),
				"/billing/current_usage":                   r("billing.CurrentUsage", billing.CurrentUsage),
				"/subscriptions":                           r("subscriptions.Index", subscriptions.Index),
				"/subscriptions/{slug}/upgrade":            r("subscriptions.UpgradeEstimate", subscriptions.UpgradeEstimate),
				"/subscriptions/{slug}/hosts":              r("subscriptions.Hosts", subscriptions.Hosts),
				"/admin/announcements/global":              r("announcements.ShowGlobalAnnouncements", announcements.ShowGlobal),
				"/admin/announcements/all":                 r("announcements.ShowAll", announcements.ShowAll),
				"/admin/amazon/healthcheck":                r("amazon.Healthcheck", amazon.Healthcheck),
				"/admin/amazon/hosts":                      r("amazon.Hosts", amazon.Hosts),
				"/admin/hosts":                             r("admin.Hosts", admin.Hosts),
				"/admin/provider_templates":                r("providertemplates.Index", providertemplates.Index),
				"/admin/provider_templates/{ptid}":         r("providertemplates.Show", providertemplates.Show),
				"/admin/template/{slug}":                   r("admin.TemplateShow", admin.TemplateShow),
				"/admin/credit_types":                      r("admin.CreditTypes", admin.CreditTypes),
				"/admin/users/{username}":                  r("admin.ShowUser", admin.ShowUser),
				"/orgs":                                    r("orgs.Index", orgs.Index),
				"/orgs/{slug}":                             r("orgs.Show", orgs.Show),
				"/orgs/{slug}/members":                     r("orgs.Show", orgs.ShowMembers),
				"/orgs/{org_slug}/billing_profile":         r("billingprofiles.Show", billingprofiles.Show),
				"/orgs/{org_slug}/subscriptions":           r("subscriptions.Index", subscriptions.Index),
				"/orgs/{org_slug}/invoices":                r("billing.Invoices", billing.Invoices),
				"/orgs/{org_slug}/snapshots":               r("snapshots.ForOrg", snapshots.ForOrg),
				"/github/repos":                            r("github.Repos", github.Repos),
				"/quickstart/repo_info":                    r("quickstart.RepoInfo", quickstart.RepoInfo),
			},
			"POST": {
				"/oauth/authorize":                         r("oauth.PostAuthorize", oauth.PostAuthorize),
				"/github/authorize":                        r("github.Authorize", github.Authorize),
				"/user/profile":                            r("userprofiles.Update", userprofiles.Update),
				"/user/settings":                           r("usersettings.Update", usersettings.Update),
				"/bonuses/earn":                            r("bonuses.Earn", bonuses.Earn),
				"/coupons":                                 r("coupon.Create", coupons.Create),
				"/user/dismiss_first_time":                 r("users.DismissFirstTime", users.DismissFirstTime),
				"/hosts":                                   r("hosts.Create", hosts.Create, EventHostCreated),
				"/hosts/refresh":                           r("hosts.Refresh", hosts.Refresh),
				"/user/billing_profile":                    r("billingprofiles.Activate", billingprofiles.Activate, EventBillingProfileActivated),
				"/hosts/{slug}/registries":                 r("registries.Login", registries.Login, EventRegistryAdded),
				"/hosts/{slug}/start":                      r("hosts.Start", mw.HostViaPath("slug", mw.TouchHost(hosts.Start)), EventHostStarted),
				"/hosts/{slug}/stop":                       r("hosts.Stop", mw.HostViaPath("slug", mw.TouchHost(hosts.Stop)), EventHostStopped),
				"/hosts/{slug}/restart":                    r("hosts.Restart", mw.HostViaPath("slug", mw.TouchHost(hosts.Restart)), EventHostRestarted),
				"/hosts/{slug}/soft_restart":               r("hosts.Restart", mw.HostViaPath("slug", mw.TouchHost(hosts.SoftRestart)), EventHostSoftRestarted),
				"/hosts/{slug}/respec":                     r("hosts.Respec", mw.HostViaPath("slug", mw.TouchHost(hosts.Respec)), EventHostRespecced),
				"/hosts/{slug}/volumes":                    r("volumes.Create", mw.HostViaPath("slug", mw.TouchHost(volumes.Create)), EventVolumeCreated),
				"/hosts/{slug}/images":                     r("images.Build", mw.HostViaPath("slug", mw.TouchHost(images.Build)), EventImageBuilt),
				"/hosts/{slug}/images/pull":                r("images.Pull", mw.HostViaPath("slug", mw.TouchHost(images.Pull)), EventImagePulled),
				"/hosts/{slug}/images/{image:.*}/push":     r("images.Push", mw.HostViaPath("slug", mw.TouchHost(images.Push)), EventImagePushed),
				"/hosts/{slug}/images/{image:.*}/tag":      r("images.Tag", mw.HostViaPath("slug", mw.TouchHost(images.Tag)), EventImageTagged),
				"/containers":                              r("containers.Create", containers.Create, EventContainerCreated),
				"/containers/create_atomic":                r("containers.Create", containers.CreateAtomic),
				"/containers/{slug}/commit":                r("containers.Commit", containers.Commit, EventContainerCommitted),
				"/containers/{slug}/start":                 r("containers.Start", containers.Start, EventContainerStarted),
				"/containers/{slug}/stop":                  r("containers.Stop", containers.Stop, EventContainerStopped),
				"/containers/{slug}/restart":               r("containers.Restart", containers.Restart, EventContainerRestarted),
				"/containers/{slug}/reconfig":              r("containers.Reconfig", containers.Reconfig, EventContainerReconfigured),
				"/containers/{slug}/token":                 r("containers.Token", containers.Token),
				"/containers/{slug}/keys":                  r("userkeys.Create", userkeys.Create, EventContainerKeyAdded),
				"/containers/{slug}/container_keys/github": r("containerkeys.AddToGitHub", containerkeys.AddToGitHub, EventContainerKeyAddedToGitHub),
				"/containers/{slug}/collaborators":         r("containercollabs.Add", containercollabs.Add, EventContainerCollabAdded),
				"/feedback":                                r("feedback.Send", feedback.Send),
				"/invoices/{id}/pay":                       r("billing.PayInvoice", billing.PayInvoice, EventInvoiceManuallyPaid),
				"/subscriptions/{slug}/upgrade":            r("subscriptions.Upgrade", subscriptions.Upgrade),
				"/subscriptions/{slug}/reactivate":         r("subscriptions.Reactivate", subscriptions.Reactivate),
				"/admin/announcements/new":                 r("announcements.SendAnnouncement", announcements.SendAnnouncement),
				"/admin/switch_user":                       r("admin.SwitchUser", admin.SwitchUser),
				"/admin/provider_templates/{ptid}":         r("providertemplates.Update", providertemplates.Update),
				"/admin/templates":                         r("admin.TemplatesCreate", admin.TemplatesCreate),
				"/admin/terminate_unaccounted_host":        r("admin.TerminateUnaccountedHost", admin.TerminateUnaccountedHost),
				"/admin/change_host_state":                 r("admin.ChangeHostState", admin.ChangeHostState),
				"/admin/amazon/fix_born/{id}":              r("amazon.FixFromBorn", amazon.FixFromBorn),
				"/admin/credit":                            r("admin.CreditUser", admin.CreditUser),
				"/orgs":                                    r("orgs.Create", orgs.Create, EventOrgCreated),
				"/orgs/{org_slug}/billing_profile":   r("billingprofiles.Activate", billingprofiles.Activate, EventBillingProfileActivated),
				"/orgs/{org_slug}/subscriptions":     r("subscriptions.Subscribe", subscriptions.Subscribe, EventOrgSubscriptionSubscribed),
				"/orgs/{org_slug}/snapshots/publish": r("snapshots.Publish", snapshots.Publish),
				"/twitter/connect":                   r("twitter.Connect", twitter.Connect),
				"/twitter/follow":                    r("twitter.Follow", twitter.Follow),
				"/twitter/tweet":                     r("twitter.Tweet", twitter.Tweet),
				"/quickstart":                        r("quickstart.Create", quickstart.Create),
			},
			"PUT": {
				"/users":                            r("users.Update", users.Update, EventUserUpdatedProfile),
				"/hosts/{slug}":                     r("hosts.Update", mw.HostViaPath("slug", hosts.Update)),
				"/containers/{slug}/keys":           r("userkeys.Create", userkeys.Create, EventContainerKeyAdded),
				"/user/billing_profile":             r("billingprofiles.Update", billingprofiles.Update, EventUserUpdatedBillingProfile),
				"/containers/{slug}/metadata":       rNoLog("containers.Metadata", containers.Metadata),
				"/containers/{slug}/user_data":      rNoLog("usercontainerdata.SetData", usercontainerdata.SetData),
				"/admin/template/{slug}":            r("admin.TemplateUpdate", admin.TemplateUpdate),
				"/orgs/{slug}/members/{identifier}": r("orgs.PutMember", orgs.PutMember, EventOrgMemberUpdated),
				"/orgs/{org_slug}/billing_profile":  r("billingprofiles.Update", billingprofiles.Update, EventUserUpdatedBillingProfile),
			},
			"DELETE": {
				"/hosts/{slug}":                            r("hosts.Destroy", mw.HostViaPath("slug", hosts.Destroy), EventHostTerminated),
				"/hosts/{slug}/registries/{rslug}":         r("registries.Logout", registries.Logout, EventRegistryRemoved),
				"/hosts/{slug}/volumes/{vslug}":            r("volumes.Destroy", mw.HostViaPath("slug", volumes.Destroy), EventVolumeRemoved),
				"/hosts/{slug}/images/{image:.*}":          r("images.Remove", mw.HostViaPath("slug", images.Remove), EventImageRemoved),
				"/containers/{slug}":                       r("containers.Destroy", containers.Destroy, EventContainerTerminated),
				"/containers/{slug}/keys/{kslug}":          r("userkeys.Delete", userkeys.Delete, EventContainerKeyRemoved),
				"/containers/{slug}/keys":                  r("userkeys.DeleteAll", userkeys.DeleteAll, EventContainerKeyRemoved),
				"/containers/{slug}/collaborators/{ccid}":  r("containercollabs.Remove", containercollabs.Remove, EventContainerCollabRemoved),
				"/subscriptions/{slug}/cancel":             r("subscriptions.Cancel", subscriptions.Cancel),
				"/logout":                                  r("oauth.Logout", oauth.Logout),
				"/user":                                    r("users.Delete", users.Delete, EventUserDeleted),
				"/admin/users/{id}/ban":                    r("admin.BanUser", admin.BanUser),
				"/orgs/{slug}/members/{identifier}":        r("orgs.RemoveMember", orgs.RemoveMember, EventOrgMemberRemoved),
				"/orgs/{org_slug}/snapshots/{snapshot_id}": r("snapshots.Delete", snapshots.Delete),
				"/containers/{slug}/container_keys/github": r("containerkeys.RemoveFromGitHub", containerkeys.RemoveFromGitHub, EventContainerGitHubKeyRemoved),
			},
		}

		for method, routes := range authed {
			s.drawRouteData(method, true, routes)
		}

		trustJsonRoutes := map[string]map[string]routeData{
			"GET": {
				"/debug": r("s.debugHandler", s.debugHandler),
			},
			"POST": {
				"/maintenance/user_refresh/{id}": r("maintenance.UserRefresh", maintenance.UserRefresh),
			},
		}

		for method, routes := range trustJsonRoutes {
			for path, r := range routes {
				final := mw.Call("nitrous."+r.label, r.mw)
				final = mw.VerifyTrust(final)
				s.route(path, routeStrictJSON, final, RouteOpts{Methods: []string{method}, LogParams: true})
			}
		}

		trustRoutes := map[string]map[string]routeData{
			"GET": {
				"/debug/stack":           r("s.pprofHandler", s.stackHandler),
				"/debug/pprof/{path:.*}": r("s.pprofHandler", s.pprofHandler),
				"/debug/profile":         r("s.profileHandler", s.profileHandler),
			},
		}

		for method, routes := range trustRoutes {
			for path, r := range routes {
				final := mw.Call("nitrous."+r.label, r.mw)
				final = mw.VerifyTrust(final)
				s.route(path, routeWritesResponse, final, RouteOpts{Methods: []string{method}, LogParams: true})
			}
		}
	})

	s.static("/", "public") // catch-all, must come last
}
