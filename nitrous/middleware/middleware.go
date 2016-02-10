package middleware

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	gh "github.com/nitrous-io/dedicated/nitrous/models/github"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mergemap"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

var (
	errInvalidRequest      = httperror.New(http.StatusBadRequest, "invalid_request", "auth token is required")
	errInvalidToken        = httperror.New(http.StatusUnauthorized, "invalid_token", "auth token is invalid")
	errRateLimitExceeded   = httperror.New(http.StatusForbidden, "request_forbidden", "you have exceeded the number of GitHub accesses. Please try again later")
	ErrHostNotFound        = httperror.New(http.StatusNotFound, "host_not_found", "host does not exist")
	ErrOutstandingInvoices = httperror.New(http.StatusBadRequest, "outstanding_invoices", "you have outstanding invoices, please check your billing settings")
	ErrNotFound            = httperror.New(http.StatusNotFound, "not_found", "not found")

	splitIPRe   = regexp.MustCompile(`[,\s]+`)
	trustedIPRe = regexp.MustCompile(`\A127\.0\.0\.1\z|\A(10|172\.(1[6-9]|2[0-9]|30|31)|192\.168)\.|\A::1\z|\Afd[0-9a-f]{2}:.+|\Alocalhost\z|\Aunix\z|\Aunix:`)

	allowedUnpaidResources = map[string]int{
		"users.GetAuthenticatedUser": 1,
		"userprofiles.Show":          1,
		"userprofiles.Update":        1,
		"hosts.Index":                1,
		"hosts.Show":                 1,
		"hosts.Stats":                1,
		"containers.Index":           1,
		"containers.Show":            1,
		"containers.Config":          1,
		"containers.Shared":          1,
		"volumes.Index":              1,
		"images.Index":               1,
		"registries.Index":           1,
		"billing.Invoices":           1,
		"billing.CurrentUsage":       1,
		"billing.PayInvoice":         1,
		"billingprofiles.Activate":   1,
		"billingprofiles.Update":     1,
		"billingprofiles.Show":       1,
		"subscriptions.Index":        1,
		"oauth.Logout":               1,
		"feedback.Send":              1,
		"orgs.Index":                 1,
		"plans.Index":                1,
	}
)

type Env struct {
	Root           string
	DB             *db.Database
	Events         *pubsub.Pubsub
	Provisioner    *provisioner.Provisioner
	Status         int
	User           *user.User
	Token          *token.Token
	Data           map[string]interface{}
	TrackEventName string
	trackEventData map[string]interface{}
	RemoteAddr     string
	Tracker        analytics.Tracker
	UUID           string
}

func (env *Env) AddEventData(data map[string]interface{}) {
	if env.trackEventData == nil {
		env.trackEventData = map[string]interface{}{}
	}

	mergemap.Merge(data, env.trackEventData)
}

func (env *Env) Login(u *user.User, w http.ResponseWriter) error {
	t := &token.Token{
		UserID: u.ID,
		Type:   "login",
		Scope:  token.ScopeFull,
	}

	err := t.Insert(env.DB)
	if err != nil {
		return err
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "Bearer",
	}

	return nil
}

func (env *Env) LoginWithOauthClient(u *user.User, w http.ResponseWriter, client *oauthclient.Client) error {
	t := &token.Token{
		UserID:   u.ID,
		Type:     "login",
		Scope:    token.ScopeFull,
		ClientID: sql.NullInt64{client.ID, true},
	}

	err := t.Insert(env.DB)
	if err != nil {
		return err
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "Bearer",
	}

	return nil
}
func (env *Env) SetEmptyResponseBody() {
	env.Status = http.StatusAccepted
	env.Data = map[string]interface{}{}
}

type Middleware func(env *Env, w http.ResponseWriter, r *http.Request) error

func Authenticate(tok string, env *Env, w http.ResponseWriter, r *http.Request) error {
	label := "nitrous.authenticate"
	start := time.Now()
	db := env.DB

	t, err := token.FindByToken(db, tok)
	if err != nil {
		if err == token.ErrInvalidToken {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			return mh.Error(errInvalidToken, label)
		}
		return err
	}

	u, err := user.FindByID(db, t.UserID)
	if err != nil && err != sql.ErrNoRows {
		return mh.Error(err, label)
	}
	if err == sql.ErrNoRows {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
		return mh.Error(errInvalidToken, label)
	}
	if u == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
		return mh.Error(err, label)
	}
	if err := t.Touch(db, GetRemoteAddr(r)); err != nil {
		logrus.Errorf("Unable to touch token %d, err: %v", t.ID, err)
	}

	if !permission.Login(t, u) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="confirmation_lapsed"`)
		return mh.Error(httperror.New(http.StatusBadRequest, "confirmation_lapsed", "confirmation token has lapsed"), label)
	}

	mh.UpdateSince(label, start)
	env.User = u
	env.Token = t

	return nil
}

func Auth(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		tok, err := GetBearerToken(r)
		if err != nil {
			if err == errInvalidRequest {
				w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			}
			return err
		}

		if err := Authenticate(tok, env, w, r); err != nil {
			return err
		}

		return mw(env, w, r)
	}
}

func VerifyUserBilling(resource string, mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		_, allowed := allowedUnpaidResources[resource]

		if !allowed {
			bp, err := env.User.BillingProfile(env.DB)
			if err != nil {
				logrus.Errorf("BlockDelinquentUser: error locating billingprofile for %+v err: %+v", env.User, err)
				return err
			} else {
				if bp.HasBillingIssues() {
					return ErrOutstandingInvoices
				}
			}
		}

		return mw(env, w, r)
	}
}

func Track(event string, mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		env.TrackEventName = event
		if err := mw(env, w, r); err != nil {
			return err
		}

		// Do not track if token holder is admin switching to user
		if env.Token != nil && env.Token.HasScope(token.ScopeAdmin) {
			return nil
		}

		// via Mixpanel HTTP API reference:
		// 'If $ip is set to "0", Mixpanel will ignore IP information.'
		env.AddEventData(map[string]interface{}{"$ip": "0"})

		bp, err := env.User.BillingProfile(env.DB)
		if err != nil {
			logrus.Errorf("Track: error locating billingprofile for %+v err: %+v", env.User, err)
		}
		if bp != nil {
			env.AddEventData(map[string]interface{}{"billing_state": bp.State})
		}

		if env.TrackEventName != "" {
			env.Tracker.TrackEvent(event, env.User, env.trackEventData)
		}

		return nil
	}
}

func VerifyTrust(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		remote := r.Header.Get("X-Forwarded-For")
		if remote == "" {
			remote = r.RemoteAddr
		}
		logrus.Infof("VerifyTrust: Incoming trust request from %s", remote)

		trustKey := envvars.TrustKey
		if trustKey == "" || trustKey != r.FormValue("trust_key") {
			logrus.Warnf("VerifyTrust: %s provided an invalid trust_key", remote)
			return ErrNotFound
		}

		return mw(env, w, r)
	}
}

func AuthQS(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") != "" {
			return Auth(mw)(env, w, r)
		}
		tok := r.FormValue("token")
		if tok == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			return errInvalidRequest
		}
		err := Authenticate(tok, env, w, r)
		if err != nil {
			return err
		}

		return mw(env, w, r)
	}
}

type hostHandlerFn func(h *host.Host, env *Env, w http.ResponseWriter, r *http.Request) error

func HostViaForm(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("form", srcVar, fn, false)
}

func HostViaPath(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("path", srcVar, fn, false)
}

func SharedHostViaPath(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("path", srcVar, fn, true)
}

func GetBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			return "", errInvalidRequest
		}

		return authHeader[len(prefix):], nil
	}

	if r.Method == "POST" && r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		token := r.PostFormValue("access_token")
		if token == "" {
			return "", errInvalidRequest
		}

		return token, nil
	}

	return "", errInvalidRequest
}

// Stolen from Rack:
// https://github.com/rack/rack/blob/dc53a8c26dc55d21240233b3d83d36efdef6e924/lib/rack/request.rb#L348-L357
func GetRemoteAddr(r *http.Request) string {
	remoteAddr := ""

	ipAddresses := filterIPAddresses(splitIPAddresses(r.RemoteAddr))
	if len(ipAddresses) > 0 {
		remoteAddr = ipAddresses[0]
	}

	if remoteAddr == "" {
		ipAddresses = filterIPAddresses(splitIPAddresses(r.Header.Get("X-Forwarded-For")))
		if len(ipAddresses) > 0 {
			remoteAddr = ipAddresses[0]
		}
	}

	return remoteAddr
}

func splitIPAddresses(ip string) []string {
	ipAddresses := splitIPRe.Split(ip, -1)
	sanitizedIPAddresses := make([]string, len(ipAddresses))

	for i, ip := range ipAddresses {
		splitIP := strings.Split(ip, ":")[0]
		sanitizedIPAddresses[i] = splitIP
	}

	return sanitizedIPAddresses
}

func filterIPAddresses(ipAddresses []string) []string {
	filteredIPAddresses := []string{}

	for _, ip := range ipAddresses {
		if !trustedIPRe.MatchString(ip) {
			filteredIPAddresses = append(filteredIPAddresses, ip)
		}
	}

	return filteredIPAddresses
}

// withHost takes a src ("form" or "path") and the formvalue/pattern to look
// for to retrieve host slug.
func withHost(src string, srcVar string, fn hostHandlerFn, allowCollab bool) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		label := "nitrous.withHost"
		start := time.Now()

		var param string
		switch src {
		case "form":
			param = r.FormValue(srcVar)
		case "path":
			param = mux.Vars(r)[srcVar]
		default:
			return errors.New("Invalid host source: " + src)
		}

		// param can either be a slug or an ID
		var h *host.Host
		var err error
		if env.User.Admin {
			h, err = host.FindByIdentifier(env.DB, param)

			// allow admins to still receive `shared` flag
			if h != nil && h.UserID != env.User.ID {
				_, err := host.FindShared(env.DB, env.User, param)
				if err == nil {
					h.Shared = true
				}
			}

		} else {
			h, err = host.Find(env.DB, env.User, param)
		}
		if allowCollab && err == sql.ErrNoRows {
			h, err = host.FindShared(env.DB, env.User, param)
			if err == nil {
				logrus.Infof("Collaborator user %d accessed host %d", env.User.ID, h.ID)
			}
			if h != nil {
				h.Shared = true
			}
		}
		if err != nil {
			if err == sql.ErrNoRows {
				return mh.Error(ErrHostNotFound, label)
			}
			return mh.Error(err, label)
		}

		mh.UpdateSince(label, start)
		return fn(h, env, w, r)
	}
}

func TouchHost(fn hostHandlerFn) hostHandlerFn {
	return func(h *host.Host, env *Env, w http.ResponseWriter, r *http.Request) error {
		h.Touch(env.DB)
		return fn(h, env, w, r)
	}
}

// Call is a helper that execute the middleware and also records its metrics.
func Call(label string, mw Middleware) Middleware {
	rlabel := "requests." + label
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		mh.Meter(rlabel).Mark(1)
		defer mh.UpdateSince(label, time.Now())
		err := mw(env, w, r)
		if err == nil {
			return nil
		}
		// distinguish in Librato between server errors and user-facing errors (invalid input)
		if _, ok := err.(*httperror.Error); ok {
			return mh.Invalid(err, label)
		} else {
			return mh.Error(err, label)
		}
	}
}

func RateLimit(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {

		exceeded, err := gh.Exceeded(env.DB, env.User.ID)
		if err != nil {
			logrus.Errorf("Error occurred while rate limiting github access: %v", err)
		}
		if exceeded {
			return errRateLimitExceeded
		}

		return mw(env, w, r)
	}
}
