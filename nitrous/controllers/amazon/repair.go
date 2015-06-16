package amazon

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var FixFromBorn = admin.VerifyAdmin(fixFromBorn)

var (
	idRe     = regexp.MustCompile(`ID:([\w-]+)`)
	regionRe = regexp.MustCompile(`Region:([\w-]+)`)
	ipRe     = regexp.MustCompile(`IPAddress:(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
	frPortRe = regexp.MustCompile(`FreighterdPort:(\d+)`)
	stateRe  = regexp.MustCompile(`State:(\w+)`)

	errHBMissing     = httperror.New(http.StatusBadRequest, "missing_host_builder", "`host_builder` must be provided")
	errIDMissing     = httperror.New(http.StatusBadRequest, "missing_id", "ID: could not be located")
	errRegionMissing = httperror.New(http.StatusBadRequest, "missing_region", "Region: could not be located")
	errIPMissing     = httperror.New(http.StatusBadRequest, "missing_ip", "IP: could not be located")
	errFPortMissing  = httperror.New(http.StatusBadRequest, "missing_freighterd_port", "FreighterdPort: could not be located")
	errStateMissing  = httperror.New(http.StatusBadRequest, "missing_state", "State: could not be located")
	errHostMissing   = httperror.New(http.StatusNotFound, "host_not_found", "host not found")
)

func fixFromBorn(env *middleware.Env, w http.ResponseWriter, r *http.Request) (err error) {
	db := env.DB

	id := mux.Vars(r)["id"]
	var h *host.Host
	h, err = host.FindByIdentifier(env.DB, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return errHostMissing
		}
		return err
	}

	hb := r.FormValue("host_builder")
	if hb == "" {
		return errHBMissing
	}

	parsedHost, err := parseHostbuilder(hb)
	if err != nil {
		return err
	}

	// sanity check to ensure we don't conflict
	var found int64
	err = db.QueryRow(`SELECT
    id
    FROM hosts
    WHERE id != $1 AND (ip_address = $2 OR provider_identifier = $3) LIMIT 1;`,
		h.ID, parsedHost.IPAddress.String, parsedHost.ProviderIdentifier.String).Scan(&found)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if found != 0 {
		return httperror.New(http.StatusConflict, "existing_host", fmt.Sprintf("IP or Identifier collides with host %d", found))
	}

	pl, err := plan.FindByID(db, h.PlanID)
	if err != nil {
		return err
	}

	hT := &hosttransition.HostTransition{}
	err = db.Get(hT, `SELECT * FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at ASC LIMIT 1;`, id)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		hT = nil
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			if err := tx.Rollback(); err != nil {
				logrus.Errorf("Error on tx Rollback, err: %+v", err)
			}
			return
		}
	}()

	h.ProviderIdentifier = parsedHost.ProviderIdentifier
	h.RegionIdentifier = parsedHost.RegionIdentifier
	h.IPAddress = parsedHost.IPAddress
	h.FreighterdPort = parsedHost.FreighterdPort
	h.State = parsedHost.State
	h.HourlyPrice = pl.HourlyPrice()
	h.Free = pl.IsFree()
	h.MaxContainers = sql.NullInt64{pl.MaxContainers, true}
	h.Size = sql.NullString{pl.Slug, true}

	nstmt, err := tx.PrepareNamed(`UPDATE hosts
		SET (provider_identifier, region_identifier, ip_address, freighterd_port,
			state, hourly_price, free, max_containers, size)
		= (:provider_identifier, :region_identifier, :ip_address, :freighterd_port,
			:state, :hourly_price, :free, :max_containers, :size)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}

	if err := nstmt.QueryRowx(h).StructScan(h); err != nil {
		return err
	}

	if hT == nil || hT.PreviousState.String != host.StateBorn {
		var endedAt pq.NullTime
		if hT != nil {
			endedAt = pq.NullTime{*hT.CreatedAt, true}
		}
		// create host transition
		_, err = tx.Exec(`INSERT INTO host_transitions
      (host_id, previous_state, state, created_at, ended_at) VALUES
      ($1, $2, $3, $4, $5);`, h.ID, host.StateBorn, host.StateRunning, h.CreatedAt, endedAt)

		if err != nil {
			return err
		}
	}

	sVol, err := volume.Find(db, h.ID, "shared")
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if sVol == nil || err == sql.ErrNoRows {
		sVol = &volume.Volume{
			HostID: h.ID,
			Slug:   "shared",
			Name:   "Shared Volume",
			System: true,
		}

		if err := sVol.Insert(tx); err != nil {
			return err
		}
	}

	cl, err := controllers.NewFreighterClient(tx, h, env.UUID)
	if err != nil {
		return err
	}

	vols, err := cl.ListVolumes()
	if err != nil {
		return err
	}

	foundShared := false
	for _, vol := range vols {
		if vol == "shared" {
			foundShared = true
			break
		}
	}

	if !foundShared {
		_, err = cl.CreateVolume(sVol.Slug)
		if err != nil {
			return httperror.New(http.StatusInternalServerError, "create_volume_failed", err.Error())
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	u, err := user.FindByID(db, h.UserID)
	if err != nil {
		logrus.Errorf("Error locating user %d, err: %v", h.UserID, err)
	}

	if u != nil {
		env.Events.Publish(
			controllers.NewEvent(controllers.EventHostUpdated, map[string]*serializers.Host{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}

	return nil
}

func parseHostbuilder(hbs string) (*host.Host, error) {
	idMatches := idRe.FindStringSubmatch(hbs)
	if len(idMatches) <= 1 {
		return nil, errIDMissing
	}

	regionMatches := regionRe.FindStringSubmatch(hbs)
	if len(regionMatches) <= 1 {
		return nil, errRegionMissing
	}

	ipMatches := ipRe.FindStringSubmatch(hbs)
	if len(ipMatches) <= 1 {
		return nil, errIPMissing
	}

	frPMatches := frPortRe.FindStringSubmatch(hbs)
	if len(frPMatches) <= 1 {
		return nil, errFPortMissing
	}

	fPrt, err := strconv.ParseInt(frPMatches[1], 10, 64)
	if err != nil {
		return nil, err
	}

	stateMatches := stateRe.FindStringSubmatch(hbs)
	if len(stateMatches) == 0 {
		return nil, errStateMissing
	}

	return &host.Host{
		ProviderIdentifier: sql.NullString{idMatches[1], true},
		RegionIdentifier:   sql.NullString{regionMatches[1], true},
		IPAddress:          sql.NullString{ipMatches[1], true},
		FreighterdPort:     sql.NullInt64{fPrt, true},
		State:              strings.ToLower(stateMatches[1]),
	}, nil
}
