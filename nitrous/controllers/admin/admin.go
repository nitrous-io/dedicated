package admin

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errHostNotFound = httperror.New(http.StatusNotFound, "host_not_found", "cannot find host with given slug and state")
	errUserNotFound = httperror.New(http.StatusNotFound, "user_not_found", "cannot find a user for the given email or username")

	errHostSlugRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "host slug required")
	errProviderRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "provider is required")
	errProviderIdentifierRequired = httperror.New(http.StatusBadRequest, "invalid_request", "provider_identifier is required")
	errHostIsNotUnaccounted       = httperror.New(http.StatusBadRequest, "invalid_request", "host is known in DB, cannot terminate directly")
	errOldStateRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "old-state required")
	errNewStateRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "new-state required")
	errNewStateInvalid            = httperror.New(http.StatusBadRequest, "invalid_request", "new-state invalid")

	errUsernameRequired   = httperror.New(http.StatusBadRequest, "invalid_request", "username is required")
	errCreditTypeRequired = httperror.New(http.StatusBadRequest, "invalid_request", "credit_type is required")
	errAmountRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "amount is required")
	errCreditTypeNotFound = httperror.New(http.StatusNotFound, "credit_type_not_found", "cannot find specified credit type")
	errAmountZero         = httperror.New(http.StatusBadRequest, "amount_zero", "amount must be greater than zero")
	errAmountTooLarge     = httperror.New(http.StatusBadRequest, "amount_too_large", "amount cannot be greater than credit type")
)

var (
	SwitchUser               = VerifyAdmin(switchUser)
	Hosts                    = VerifyAdmin(hosts)
	ChangeHostState          = VerifyAdmin(changeHostState)
	TerminateUnaccountedHost = VerifyAdmin(terminateUnaccountedHost)
	CreditUser               = VerifyAdmin(creditUser)
	CreditTypes              = VerifyAdmin(creditTypes)
)

var HostbuilderClient = provisioner.HostbuilderClient

func switchUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	login := r.FormValue("login") // this can be either username or email
	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	t := &token.Token{
		UserID: u.ID,
		Type:   "login",
		Scope:  token.ScopeAdmin,
	}

	err = t.Insert(env.DB)
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

type liveHost struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort uint16 `json:"freighterd_port"`
	State          string `json:"state"`
}

func hosts(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	provider := r.FormValue("provider")
	if provider == "" {
		return errProviderRequired
	}

	dbHosts, err := host.FindAllByProvider(env.DB, provider)
	if err != nil {
		return err
	}

	hostbuilderHosts, err := HostbuilderClient.List(provider)
	if err != nil {
		return err
	}

	liveHosts := []*liveHost{}
	for _, h := range hostbuilderHosts {
		lh := &liveHost{
			Provider:       h.Provider,
			ID:             h.ID,
			Name:           h.Name,
			IPAddress:      h.IPAddress,
			FreighterdPort: h.FreighterdPort,
			State:          h.State,
		}
		liveHosts = append(liveHosts, lh)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"db_hosts":   serializers.SerializeHosts(dbHosts),
		"live_hosts": liveHosts,
	}

	return nil
}

func changeHostState(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	slug := r.FormValue("slug")
	if slug == "" {
		return errHostSlugRequired
	}

	oldState := r.FormValue("old_state")
	if oldState == "" {
		return errOldStateRequired
	}

	newState := r.FormValue("new_state")
	if newState == "" {
		return errNewStateRequired
	}

	if newState != host.StateStopped && newState != host.StateRunning {
		return errNewStateInvalid
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	h, err := host.FindByIdentifier(tx, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errHostNotFound
		} else {
			return err
		}
	}
	if h.State != oldState {
		return errHostNotFound
	}

	if newState == host.StateStopped {
		logrus.WithFields(logrus.Fields{
			"slug": h.Slug,
			"old_provider_identifier": h.ProviderIdentifier,
			"old_ip_address":          h.IPAddress,
		}).Infof("Updating host state")
		// healthy hotswap hosts in stopped state should not have a
		// provider_identifier or ip_address
		err = tx.QueryRowx(`UPDATE hosts
			SET state=$1, provider_identifier = '', ip_address = null
			WHERE slug=$2
			RETURNING *;`,
			newState, slug,
		).StructScan(h)
	} else {
		err = tx.QueryRowx(`UPDATE hosts
			SET state=$1 WHERE slug=$2
			RETURNING *;`,
			newState, slug,
		).StructScan(h)
	}
	if err != nil {
		return err
	}

	if err := hosttransition.Record(tx, h, oldState, *h.UpdatedAt, env.User.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
	return nil
}

func terminateUnaccountedHost(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	provider := r.FormValue("provider")
	if provider == "" {
		return errProviderRequired
	}
	provID := r.FormValue("provider_identifier")
	if provID == "" {
		return errProviderIdentifierRequired
	}

	config := hostbuilderclient.Config{"id": provID}

	if region := r.FormValue("region"); region != "" {
		config["region_id"] = region
	}

	if ip := r.FormValue("ip_address"); ip != "" {
		config["ip_address"] = ip
	}

	var count int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts
		INNER JOIN provider_templates pt
			ON hosts.provider_template_id = pt.id
		WHERE
			hosts.provider_identifier = $1 AND hosts.terminated_at IS NULL
			AND pt.provider = $2`,
		provID, provider).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return errHostIsNotUnaccounted
	}

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, env.UUID)
	urlArgs := url.Values{}
	urlArgs.Set("uid", "admin")
	resp, err := HostbuilderClient.Delete(provider, config, headers, urlArgs)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": resp.Status,
	}
	return nil
}

func creditTypes(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cts := []*credittype.CreditType{}
	err := env.DB.Select(&cts, `
		SELECT * FROM credit_types
		WHERE (expires_at IS NULL OR expires_at >= now());
	`)

	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"credit_types": serializers.SerializeCreditTypes(cts),
	}
	return nil
}

func creditUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	username := r.FormValue("username")
	if username == "" {
		return errUsernameRequired
	}
	cType := r.FormValue("credit_type")
	if cType == "" {
		return errCreditTypeRequired
	}
	amountStr := r.FormValue("amount")
	if amountStr == "" {
		return errAmountRequired
	}

	u, err := user.FindByUsernameOrEmail(db, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	ct := &credittype.CreditType{}
	err = db.Get(ct, `
		SELECT * FROM credit_types
		WHERE name = $1
		AND (expires_at IS NULL OR expires_at >= now());
	`, cType)
	if err != nil {
		if err == sql.ErrNoRows {
			return errCreditTypeNotFound
		}
		return err
	}

	amount, err := strconv.ParseInt(amountStr, 0, 64)
	if err != nil {
		return err
	}
	if amount == 0 {
		return errAmountZero
	}
	if amount > ct.DefaultValue {
		return errAmountTooLarge
	}

	c := &credit.Credit{
		UserID:       u.ID,
		CreditTypeID: sql.NullInt64{ct.ID, true},
		Amount:       amount,
		AddedByID:    sql.NullInt64{env.User.ID, true},
	}
	err = c.Insert(db)
	if err != nil {
		return err
	}

	available, err := credit.AvailableCredits(db, u.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"available_credits": available,
	}
	return nil
}

func VerifyAdmin(fn middleware.Middleware) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if !permission.Admin(env.Token, env.User) {
			return httperror.New(http.StatusNotFound, "not_found", "not found")
		}

		return fn(env, w, r)
	}
}

func VerifyAdminOrService(fn middleware.Middleware) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if !permission.Service(env.Token, env.User) {
			return httperror.New(http.StatusNotFound, "not_found", "not found")
		}

		return fn(env, w, r)
	}
}
