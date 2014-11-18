package server

import (
	"net/http/pprof"
	"os"

	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/controllers/appsettings"
	"github.com/nitrous-io/dedicated/nitrous/controllers/baseimages"
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
	"github.com/nitrous-io/dedicated/nitrous/controllers/tokens"
	"github.com/nitrous-io/dedicated/nitrous/controllers/userprofiles"
	"github.com/nitrous-io/dedicated/nitrous/controllers/users"
	"github.com/nitrous-io/dedicated/nitrous/controllers/volumes"
	mw "github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
)

type routeData struct {
	mw    mw.Middleware
	track string
}

// r is a helper that returns routeData
func r(mw mw.Middleware, args ...string) routeData {
	var track string
	if len(args) > 0 {
		track = args[0]
	}
	return routeData{mw: mw, track: track}
}

// drawRouteData draw the route with `routeJSON`.
// if `auth` is set then we setup the auth middleware for it
// if `track` for a route is set then we setup the track middleware for it
func (s *Server) drawRouteData(method string, auth bool, routes map[string]routeData) {
	for path, r := range routes {
		var final mw.Middleware
		final = mw.Call(r.mw)
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
	s.route("/", mw.Call(home.Index), "GET")

	if os.Getenv("DEBUG") == "true" {
		s.router.HandleFunc("/debug/pprof/{path:.*}", pprof.Index).Methods("GET")
		s.routeJSON("/debug", s.debugHandler, "GET")
	}

	s.namespace("/api/v0", func() {
		s.route("/events", mw.AuthQS(mw.Call(events.Stream)), "GET")

		nonAuth := map[string]map[string]routeData{
			"GET": {
				"/plans":        r(plans.Index),
				"/regions":      r(regions.Index),
				"/base_images":  r(baseimages.Index),
				"/app_settings": r(appsettings.Index),
			},
			"POST": {
				"/oauth/token":               r(tokens.Post, analytics.UserSignedIn),
				"/users":                     r(users.Create, analytics.UserSignedUp),
				"/users/confirm":             r(users.Confirm),
				"/users/resend_confirmation": r(users.ResendConfirmation),
				"/send_reset_password_token": r(passwords.SendResetPasswordToken),
				"/reset_password":            r(passwords.ResetPassword),
			},
		}

		for method, routes := range nonAuth {
			s.drawRouteData(method, false, routes)
		}

		authed := map[string]map[string]routeData{
			"GET": {
				"/user":                              r(users.GetAuthenticatedUser),
				"/user/profile":                      r(userprofiles.Show),
				"/hosts":                             r(hosts.Index),
				"/images":                            r(images.Index),
				"/hosts/{slug}/images":               r(images.Index),
				"/hosts/{slug}":                      r(mw.HostViaPath("slug", hosts.Show)),
				"/hosts/{slug}/stats":                r(mw.HostViaPath("slug", hosts.Stats)),
				"/hosts/{slug}/volumes":              r(mw.HostViaPath("slug", volumes.Index)),
				"/hosts/{slug}/volumes/{vslug}":      r(mw.HostViaPath("slug", volumes.Show)),
				"/hosts/{slug}/volumes/{vslug}/info": r(mw.HostViaPath("slug", volumes.Info)),
				"/hosts/{slug}/images/{image:.*}":    r(mw.HostViaPath("slug", images.Inspect)),
				"/containers":                        r(containers.Index),
				"/containers/{slug}":                 r(containers.Show),
				"/containers/{slug}/config":          r(containers.Config),
				"/containers/{slug}/keys":            r(keys.Index),
			},
			"POST": {
				"/user/profile":                        r(userprofiles.Update),
				"/hosts":                               r(hosts.Create, analytics.HostCreated),
				"/hosts/{slug}/registries":             r(registries.Login),
				"/hosts/{slug}/restart":                r(mw.HostViaPath("slug", hosts.Restart)),
				"/hosts/{slug}/volumes":                r(mw.HostViaPath("slug", volumes.Create)),
				"/hosts/{slug}/images":                 r(mw.HostViaPath("slug", images.Build)),
				"/hosts/{slug}/images/pull":            r(mw.HostViaPath("slug", images.Pull)),
				"/hosts/{slug}/images/{image:.*}/push": r(mw.HostViaPath("slug", images.Push)),
				"/hosts/{slug}/images/{image:.*}/tag":  r(mw.HostViaPath("slug", images.Tag)),
				"/containers":                          r(containers.Create, analytics.ContainerCreated),
				"/containers/{slug}/commit":            r(containers.Commit),
				"/containers/{slug}/start":             r(containers.Start),
				"/containers/{slug}/stop":              r(containers.Stop),
				"/containers/{slug}/restart":           r(containers.Restart),
				"/containers/{slug}/reconfig":          r(containers.Reconfig),
				"/containers/{slug}/token":             r(containers.Token, analytics.ContainerAccessed),
				"/containers/{slug}/keys":              r(keys.Create),
				"/feedback":                            r(feedback.Send),
				"/admin/switch_user":                   r(admin.SwitchUser),
			},
			"PUT": {
				"/users":                  r(users.Update),
				"/hosts/{slug}":           r(mw.HostViaPath("slug", hosts.Update)),
				"/containers/{slug}/keys": r(keys.Create),
			},
			"DELETE": {
				"/hosts/{slug}":                    r(mw.HostViaPath("slug", hosts.Destroy)),
				"/hosts/{slug}/registries/{rslug}": r(registries.Logout),
				"/hosts/{slug}/volumes/{vslug}":    r(mw.HostViaPath("slug", volumes.Destroy)),
				"/hosts/{slug}/images/{image:.*}":  r(mw.HostViaPath("slug", images.Remove)),
				"/containers/{slug}":               r(containers.Destroy),
				"/containers/{slug}/keys/{kslug}":  r(keys.Delete),
				"/containers/{slug}/keys":          r(keys.DeleteAll),
			},
		}

		for method, routes := range authed {
			s.drawRouteData(method, true, routes)
		}
	})

	s.static("/", "public") // catch-all, must come last
}
