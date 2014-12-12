package server

import (
	"net/http/pprof"
	"os"

	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/controllers/appsettings"
	"github.com/nitrous-io/dedicated/nitrous/controllers/baseimages"
	"github.com/nitrous-io/dedicated/nitrous/controllers/billingprofiles"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/events"
	"github.com/nitrous-io/dedicated/nitrous/controllers/feedback"
	"github.com/nitrous-io/dedicated/nitrous/controllers/home"
	"github.com/nitrous-io/dedicated/nitrous/controllers/hosts"
	"github.com/nitrous-io/dedicated/nitrous/controllers/images"
	"github.com/nitrous-io/dedicated/nitrous/controllers/keys"
	"github.com/nitrous-io/dedicated/nitrous/controllers/passwords"
	"github.com/nitrous-io/dedicated/nitrous/controllers/plans"
	"github.com/nitrous-io/dedicated/nitrous/controllers/regions"
	"github.com/nitrous-io/dedicated/nitrous/controllers/registries"
	"github.com/nitrous-io/dedicated/nitrous/controllers/stripehooks"
	"github.com/nitrous-io/dedicated/nitrous/controllers/templates"
	"github.com/nitrous-io/dedicated/nitrous/controllers/tokens"
	"github.com/nitrous-io/dedicated/nitrous/controllers/userprofiles"
	"github.com/nitrous-io/dedicated/nitrous/controllers/users"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	mw "github.com/nitrous-io/dedicated/nitrous/middleware"
)

type routeData struct {
	mw    mw.Middleware
	track string
	label string
}

// r is a helper that returns routeData. A route should always have label which
// acts as a metric label for that route
func r(label string, mw mw.Middleware, args ...string) routeData {
	var track string
	if len(args) > 0 {
		track = args[0]
	}
	return routeData{mw: mw, track: track, label: label}
}

// drawRouteData draw the route with `routeJSON`.
// if `auth` is set then we setup the auth middleware for it
// if `track` for a route is set then we setup the track middleware for it
func (s *Server) drawRouteData(method string, auth bool, routes map[string]routeData) {
	for path, r := range routes {
		var final mw.Middleware
		// prefix all label with `nitrous.`
		final = mw.Call("nitrous."+r.label, r.mw)
		if r.track != "" {
			final = mw.Track(r.track, final)
		}
		if auth {
			final = mw.Auth(final)
		}
		s.routeJSON(path, final, method)
	}
}

func (s *Server) drawRoutes() {
	s.route("/", mw.Call("home.Index", home.Index), "GET")

	if os.Getenv("DEBUG") == "true" {
		s.router.HandleFunc("/debug/pprof/{path:.*}", pprof.Index).Methods("GET")
		s.routeJSON("/debug", s.debugHandler, "GET")
	}

	s.namespace("/api/v0", func() {
		s.route("/events", mw.AuthQS(mw.Call("events.Stream", events.Stream)), "GET")

		nonAuth := map[string]map[string]routeData{
			"GET": {
				"/plans":        r("plans.Index", plans.Index),
				"/regions":      r("regions.Index", regions.Index),
				"/base_images":  r("baseimages.Index", baseimages.Index),
				"/templates":    r("templates.Index", templates.Index),
				"/app_settings": r("appsettings.Index", appsettings.Index),
			},
			"POST": {
				"/oauth/token":               r("tokens.Post", tokens.Post, UserSignedIn),
				"/users":                     r("users.Create", users.Create, UserSignedUp),
				"/user/confirm":              r("users.Confirm", users.Confirm),
				"/user/resend_confirm":       r("users.ResendConfirmation", users.ResendConfirmation),
				"/send_reset_password_token": r("passwords.SendResetPasswordToken", passwords.SendResetPasswordToken),
				"/reset_password":            r("passwords.ResetPassword", passwords.ResetPassword),
				"/stripe_hook":               r("stripehooks.ReceiveHook", stripehooks.ReceiveHook),
			},
		}

		for method, routes := range nonAuth {
			s.drawRouteData(method, false, routes)
		}

		authed := map[string]map[string]routeData{
			"GET": {
				"/user":                              r("users.GetAuthenticatedUser", users.GetAuthenticatedUser),
				"/user/profile":                      r("userprofiles.Show", userprofiles.Show),
				"/hosts":                             r("hosts.Index", hosts.Index),
				"/images":                            r("images.IndexAll", images.Index),
				"/hosts/{slug}/images":               r("images.Index", images.Index),
				"/hosts/{slug}":                      r("hosts.Show", mw.HostViaPath("slug", hosts.Show)),
				"/hosts/{slug}/stats":                r("hosts.Stats", mw.HostViaPath("slug", hosts.Stats)),
				"/hosts/{slug}/volumes":              r("volumes.Index", mw.HostViaPath("slug", volumes.Index)),
				"/hosts/{slug}/volumes/{vslug}":      r("volumes.Show", mw.HostViaPath("slug", volumes.Show)),
				"/hosts/{slug}/volumes/{vslug}/info": r("volumes.Info", mw.HostViaPath("slug", volumes.Info)),
				"/hosts/{slug}/images/{image:.*}":    r("images.Inspect", mw.HostViaPath("slug", images.Inspect)),
				"/hosts/{slug}/registries":           r("registries.Index", registries.Index),
				"/containers":                        r("containers.Index", containers.Index),
				"/containers/{slug}":                 r("containers.Show", containers.Show),
				"/containers/{slug}/config":          r("containers.Config", containers.Config),
				"/containers/{slug}/keys":            r("keys.Index", keys.Index),
			},
			"POST": {
				"/user/profile":                        r("userprofiles.Update", userprofiles.Update),
				"/hosts":                               r("hosts.Create", hosts.Create, HostCreated),
				"/user/billing_profile":                r("billingprofiles.Activate", billingprofiles.Activate),
				"/hosts/{slug}/registries":             r("registries.Login", registries.Login),
				"/hosts/{slug}/restart":                r("hosts.Restart", mw.HostViaPath("slug", hosts.Restart)),
				"/hosts/{slug}/volumes":                r("volumes.Create", mw.HostViaPath("slug", volumes.Create)),
				"/hosts/{slug}/images":                 r("images.Build", mw.HostViaPath("slug", images.Build)),
				"/hosts/{slug}/images/pull":            r("images.Pull", mw.HostViaPath("slug", images.Pull)),
				"/hosts/{slug}/images/{image:.*}/push": r("images.Push", mw.HostViaPath("slug", images.Push)),
				"/hosts/{slug}/images/{image:.*}/tag":  r("images.Tag", mw.HostViaPath("slug", images.Tag)),
				"/containers":                          r("containers.Create", containers.Create, ContainerCreated),
				"/containers/{slug}/commit":            r("containers.Commit", containers.Commit),
				"/containers/{slug}/start":             r("containers.Start", containers.Start),
				"/containers/{slug}/stop":              r("containers.Stop", containers.Stop),
				"/containers/{slug}/restart":           r("containers.Restart", containers.Restart),
				"/containers/{slug}/reconfig":          r("containers.Reconfig", containers.Reconfig, ContainerReconfigured),
				"/containers/{slug}/token":             r("containers.Token", containers.Token, ContainerAccessed),
				"/containers/{slug}/keys":              r("keys.Create", keys.Create, ContainerKeyAdded),
				"/containers/{slug}/start_app":         r("containers.StartApp", containers.StartApp),
				"/containers/{slug}/stop_app":          r("containers.StopApp", containers.StopApp),
				"/feedback":                            r("feedback.Send", feedback.Send),
				"/admin/switch_user":                   r("admin.SwitchUser", admin.SwitchUser),
			},
			"PUT": {
				"/users":                  r("users.Update", users.Update),
				"/hosts/{slug}":           r("hosts.Update", mw.HostViaPath("slug", hosts.Update)),
				"/containers/{slug}/keys": r("keys.Create", keys.Create),
				"/user/billing_profile":   r("billingprofiles.Update", billingprofiles.Update),
			},
			"DELETE": {
				"/hosts/{slug}":                    r("hosts.Destroy", mw.HostViaPath("slug", hosts.Destroy), HostDestroyed),
				"/hosts/{slug}/registries/{rslug}": r("registries.Logout", registries.Logout),
				"/hosts/{slug}/volumes/{vslug}":    r("volumes.Destroy", mw.HostViaPath("slug", volumes.Destroy)),
				"/hosts/{slug}/images/{image:.*}":  r("images.Remove", mw.HostViaPath("slug", images.Remove)),
				"/containers/{slug}":               r("containers.Destroy", containers.Destroy, ContainerDestroyed),
				"/containers/{slug}/keys/{kslug}":  r("keys.Delete", keys.Delete),
				"/containers/{slug}/keys":          r("keys.DeleteAll", keys.DeleteAll),
			},
		}

		for method, routes := range authed {
			s.drawRouteData(method, true, routes)
		}
	})

	s.static("/", "public") // catch-all, must come last
}
