package hosts

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/serializers"
)

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
