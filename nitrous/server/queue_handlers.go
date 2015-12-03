package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	"github.com/nitrous-io/dedicated/pkg/queue"
)

type SubscriptionCreateError struct {
	Err string
}

func (e SubscriptionCreateError) Error() string {
	return e.Err
}

// progress event to be compatible with the progress writer
type progress struct {
	Stream string `json:"stream"`
}

const freighterRetryCount int = 5

var (
	errUnableToCreateSubscription = SubscriptionCreateError{Err: "Failed to create subscription, please try again."}
)

func (s *Server) HostQueueHandler() func(m *queue.Message) error {
	return func(m *queue.Message) error {
		var err error

		logrus.Debugf("uuid=%s queueHandlerTracer: incoming message: %+v", m.UUID, m)
		switch {
		case queue.Create.IsA(m.Action):
			err = s.handleCreateMessage(m)
		case queue.Stop.IsA(m.Action):
			err = s.handleStopMessage(m)
		case queue.Respec.IsA(m.Action):
			err = s.handleRespecMessage(m)
		default:
			logrus.Debugf("uuid=%s queueHandlerTracer: unexpected message: %+v", m.UUID, m)
		}
		return err
	}
}

func (s *Server) handleCreateMessage(m *queue.Message) error {
	db := s.db
	var queueInfo hh.QueueInfo
	err := json.Unmarshal(m.Payload, &queueInfo)
	if err != nil {
		return err
	}

	u, err := user.FindByID(db, queueInfo.UserID)
	if err != nil {
		return err
	}

	eventName := controllers.EventName(m.Event)
	switch eventName {
	case controllers.EventHostCreated:
		h, err := host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}

		if h.State == host.StateRunning {
			logrus.Printf("uuid=%s host running but received created event %+v", m.UUID, queueInfo)
		}

		err = s.activateHost(m.UUID, u, h, queueInfo.Host)
		if err != nil {
			switch e := err.(type) {
			case SubscriptionCreateError:
				if err := s.db.WithTransaction(func(tx *sqlx.Tx) error {
					hdrs := http.Header{}
					hdrs.Set(constants.NitrousUUIDHeader, m.UUID)
					return hostcontrol.Terminate(tx, h, hdrs, u.ID, h.State)
				}); err != nil {
					logrus.Errorf("Unable to terminate host %d, will modify DB: %v", h.ID, err)
					// attempting to mark host as terminated
					// at DB level
					h.ProviderIdentifier = sql.NullString{Valid: false}
					err := s.db.WithTransaction(func(tx *sqlx.Tx) error {
						hdrs := http.Header{}
						hdrs.Set(constants.NitrousUUIDHeader, m.UUID)
						return hostcontrol.Terminate(tx, h, hdrs, u.ID, h.State)
					})
					if err != nil {
						logrus.Errorf("Unable to terminate host at DB level %d: %v", h.ID, err)
						s.markHostAsTerminated(h)
					}
				} else {
					s.publishProvisionFailure(u, h, e.Error())
				}

				return err
			default:
				s.publishProvisionFailure(u, h, err.Error())
				return err
			}
		}

		// underlying host data may have changed, ensure we send refreshed copy
		h, err = host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}
		s.Events.Publish(
			controllers.NewEvent(eventName, map[string]*serializers.Host{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)
	case controllers.EventHostCreating:
		p := &progress{Stream: strings.TrimSpace(queueInfo.Message)}
		d, err := json.Marshal(p)
		if err != nil {
			logrus.Printf("uuid=%s failed to encode progress event %+v", m.UUID, queueInfo)
			return err
		}
		s.Events.Publish(
			controllers.NewEvent(eventName, &controllers.ProgressEvent{
				ID:       queueInfo.ID,
				Slug:     queueInfo.Slug,
				Progress: string(d),
			}),
			u.Topic(),
		)
	case controllers.EventHostCreateFailed:
		h, err := host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}
		h.State = host.StateOrphan
		if err := h.Update(s.db); err != nil {
			logrus.Debugf("uuid=%s queueProvisionTracer: update err: %+v h: %+v", m.UUID, err, h)
			return err
		}
		s.publishProvisionFailure(u, h, queueInfo.Message)
	default:
		logrus.Debugf("uuid=%s queueTracer: unexpected event for stopping a host: queueInfo(%+v) message(%+v)", m.UUID, queueInfo, m)
	}
	return nil
}

func (s *Server) activateHost(uuid string, u *user.User, h *host.Host, hostInfo *hh.Info) error {
	db := s.db
	logData := logrus.WithFields(logrus.Fields{
		"uuid":    uuid,
		"host_id": h.ID,
		"user_id": u.ID,
	})
	logData.Debugf("queueProvisionTracer: queue: %+v", uuid, hostInfo)
	if hostInfo != nil {
		h.IPAddress = sql.NullString{hostInfo.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{hostInfo.ID, true}
		h.RegionIdentifier = sql.NullString{hostInfo.Region, true}

		if hostInfo.VolumeID != "" {
			h.ProviderStorageIdentifier = sql.NullString{hostInfo.VolumeID, true}
		} else if hostInfo.Provider == "amazon" {
			return fmt.Errorf("did not receive volume ID for host: %+v", hostInfo)
		}
	}

	h.FreighterdPort = sql.NullInt64{int64(hostInfo.FreighterdPort), true}
	h.State = host.StateRunning

	nstmt, err := db.PrepareNamed(`UPDATE hosts
		SET (ip_address, provider_identifier, region_identifier, freighterd_port, state, provider_storage_identifier, updated_at)
		= (:ip_address, :provider_identifier, :region_identifier, :freighterd_port, :state, :provider_storage_identifier, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}

	if err := nstmt.QueryRowx(h).StructScan(h); err != nil {
		logData.Debugf("queueProvisionTracer: update (provider) err: %+v h: %+v", uuid, err, h)
		return err
	}

	p, err := plan.FindByID(db, h.PlanID)
	if err != nil {
		logData.Debugf("queueProvisionTracer: failed to get the plan. err: %+v h: %+v", uuid, err, h)
	}

	if p.IsSubscription() && !h.SubscriptionID.Valid {
		var o *org.Org
		if h.OrgID.Valid {
			o, err = org.Find(db, h.OrgID.Int64)
			if err != nil {
				if err == sql.ErrNoRows {
					return SubscriptionCreateError{Err: "Unable to locate organization."}
				}
				logData.Errorf("Unable to locate org: %d, err: %v", h.OrgID.Int64)
				return errUnableToCreateSubscription
			}
			_, err := orgmembership.FindByUsernameAndOrgSlug(db, u.Username, o.Slug)
			if err != nil {
				if err == sql.ErrNoRows {
					return SubscriptionCreateError{Err: "You do not have access to this organization."}
				}
				logData.Errorf("Unable to locate org membership: %d, err: %v", h.OrgID.Int64)
				return errUnableToCreateSubscription
			}
		}
		opts := &subscriptions.SubscribeOptions{
			Host: h,
			Plan: p,
			User: u,
			Org:  o,
		}
		if o == nil {
			bp, err := u.BillingProfile(db)
			if err != nil {
				return err
			}
			if cl, err := promocode.FindPendingClaim(db, u.ID, p.ID); err == nil {
				opts.PromoCodeUse = cl.Use
				opts.PromoCode = cl.Code
			} else if err != sql.ErrNoRows {
				logrus.Errorf("Error finding pending claim for BP %d, err: %v", bp.ID, err)
			}
		}

		_, err := s.SubscriptionProvider.Subscribe(db, opts)
		if err != nil {
			logData.WithFields(logrus.Fields{
				"alert":    "nosubscription",
				"err":      err.Error(),
				"username": u.Username,
				"email":    u.Email,
				"host":     h,
			}).Errorf("queueProvisionTracer: SubscriptionCreateError")
			if err == subscriptions.ErrCardExpired || err == subscriptions.ErrCardDeclined {
				return SubscriptionCreateError{Err: err.Error()}
			}
			return errUnableToCreateSubscription
		}
	}

	_, err = db.Exec(`UPDATE hosts SET hourly_price=$1, free=$2, updated_at=now() WHERE id=$3 AND plan_id=$4;`,
		p.HourlyPrice(), p.IsFree(), h.ID, p.ID)
	if err != nil {
		logrus.Debugf("uuid=%s queueProvisionTracer: update err: %+v h: %+v ", uuid, err, h)
		return err
	}
	logrus.Debugf("uuid=%s queueProvisionTracer: updated h: %+v", uuid, h)

	if err := s.createSharedVolume(uuid, u, h); err != nil {
		return err
	}

	if err := s.recordTransition(u, h); err != nil {
		return err
	}

	return nil
}

func (s *Server) recordTransition(u *user.User, h *host.Host) error {
	timing := h.AssignedAt
	if timing == nil {
		timing = h.CreatedAt
	}

	return s.db.WithTransaction(func(tx *sqlx.Tx) error {
		if err := hosttransition.Record(tx, h, host.StateBorn, *timing, u.ID); err != nil {
			return err
		}
		return nil
	})
}

func (s *Server) createSharedVolume(uuid string, u *user.User, h *host.Host) error {
	cl, err := controllers.NewFreighterClient(s.db, h, uuid)
	if err != nil {
		return err
	}

	vol := &volume.Volume{
		HostID: h.ID,
		Slug:   "shared",
		Name:   "Shared Volume",
		System: true,
	}

	return s.db.WithTransaction(func(tx *sqlx.Tx) error {
		if err := vol.Insert(tx); err != nil {
			return err
		}

		// Freighter could be unavailable due to restarting or temporal network issue.
		// To solve it, it retries 5 times until we get a connection to it
		attempts := 0
		sleepDuration := 100 * time.Millisecond
		for attempts < freighterRetryCount {
			if _, err := cl.CreateVolume(vol.Slug); err != nil {
				// Only retry when it is connection error
				if strings.Contains(err.Error(), "connection reset by peer") || err == io.EOF {
					logrus.Infof("uuid=%s retrying to connect to freighter to create a volume. Sleeping for %0.2fs", uuid, sleepDuration.Seconds())
					attempts++
					time.Sleep(sleepDuration)
					sleepDuration *= 2
					continue
				}

				return err
			} else {
				s.Events.Publish(
					controllers.NewEvent(controllers.EventVolumeCreated, map[string]*serializers.Volume{
						"volume": serializers.SerializeVolume(vol),
					}),
					u.Topic(),
				)
				return nil
			}
		}

		return fmt.Errorf("failed to connect to fregither to create shared volume")
	})
}

func (s *Server) publishProvisionFailure(u *user.User, h *host.Host, message string) {
	s.Events.Publish(
		controllers.NewEvent(controllers.EventHostCreateFailed, map[string]interface{}{
			"host":  serializers.SerializeHost(h),
			"error": message,
		}),
		u.Topic(),
	)
}

func (s *Server) handleStopMessage(m *queue.Message) error {
	db := s.db
	var queueInfo hh.QueueInfo
	err := json.Unmarshal(m.Payload, &queueInfo)
	if err != nil {
		return err
	}

	u, err := user.FindByID(db, queueInfo.UserID)
	if err != nil {
		return err
	}

	eventName := controllers.EventName(m.Event)

	switch eventName {
	case controllers.EventHostStopped:
		logrus.Debugf("uuid=%s queueTracer: stopped a host(%s) with message(%+v)", m.UUID, queueInfo.Slug, m)
		h, err := host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}

		cns, err := container.FindAllByHostID(db, h.ID)
		if err != nil {
			return err
		}

		info := &hostcontrol.HostInfo{ID: queueInfo.Host.ID}
		if err := hostcontrol.OnStopped(s.db, h, info, queueInfo.RequesterID); err != nil {
			return err
		}

		for _, cn := range cns {
			state := container.State(cn.TempState.Int64)
			if !cn.TempState.Valid || (cn.TempState.Valid && state != container.StateStopping) {
				cn.State = container.StateStopped
				err := s.PublishContainerEvent(controllers.EventContainerStopped, cn, &queueInfo)
				if err != nil {
					logrus.Debugf("uuid=%s queueTracer: failed to send an event(%s) for container(%+v)", m.UUID, controllers.EventContainerStopped, cn)
				}
			}
		}

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostStopped, map[string]*serializers.Host{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)
		logrus.Debugf("uuid=%s queueTracer: published host stopped event", m.UUID)
	case controllers.EventHostStopping:
		logrus.Debugf("uuid=%s queueTracer: stopping a host(%s) with message(%+v)", m.UUID, queueInfo.Slug, m)
	case controllers.EventHostStopFailed:
		logrus.Debugf("uuid=%s queueTracer: failed to stop a host(%s) with message(%+v) due to ", m.UUID, queueInfo.Slug, m, queueInfo.Message)
		if queueInfo.Host == nil {
			return fmt.Errorf("uuid=%s queueTracer: host is nil", m.UUID)
		}
		h, err := host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}

		state := strings.ToLower(queueInfo.Host.State.String())
		if err := hostcontrol.OnStopFailed(s.db, h, queueInfo.RequesterID, state); err != nil {
			return err
		}

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostUpdated, map[string]*serializers.Host{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)
	default:
		logrus.Debugf("uuid=%s queueTracer: unexpected event for stopping a host: queueInfo(%+v) message(%+v)", m.UUID, queueInfo, m)
	}

	return nil
}

func (s *Server) handleRespecMessage(m *queue.Message) error {
	db := s.db
	var queueInfo hh.QueueInfo
	err := json.Unmarshal(m.Payload, &queueInfo)
	if err != nil {
		return err
	}

	u, err := user.FindByID(db, queueInfo.UserID)
	if err != nil {
		return err
	}

	h, err := host.Find(db, u, queueInfo.Slug)
	if err != nil {
		return err
	}

	rs, err := respec.FindProcessing(db, h.ID)
	if err != nil {
		return err
	}

	logData := logrus.WithFields(logrus.Fields{
		"uuid":    m.UUID,
		"host_id": h.ID,
	})

	eventName := controllers.EventName(m.Event)
	switch eventName {
	case controllers.EventHostRespecced:
		logData.Debug("respecced host")
		if queueInfo.Host == nil {
			return fmt.Errorf("uuid=%s queueTracer: host is nil", m.UUID)
		}

		info := queueInfo.Host
		h.ProviderTemplateID = rs.ProviderTemplateID
		switch info.State {
		case state.Running:
			h.State = host.StateRunning
		case state.Stopped:
			h.State = host.StateStopped
		default:
			logData.Warnf("unexpected state %v from hostbuilder after respec", info.State)
		}
		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{info.ID, true}
		h.ProviderStorageIdentifier = sql.NullString{info.VolumeID, true}

		if err := h.UpdateAfterRespec(db); err != nil {
			logData.Errorf("error updating host after respec completed: %v", err)
		}
		if err := rs.UpdateState(db, respec.StateCompleted); err != nil {
			logData.Errorf("error updating respec state to completed: %v", err)
		}

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostRespecced, map[string]interface{}{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)
	case controllers.EventHostRespeccing:
		logData.Debug("respeccing host")

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostRespeccing, map[string]interface{}{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)
	case controllers.EventHostRespecFailed:
		logData.Debugf("respeccing failed due to %s", queueInfo.Message)

		if err := h.UpdateState(db, host.StateRespecFailed); err != nil {
			logData.Errorf("error updating host after respec failed: %v", err)
		}

		if err := rs.UpdateState(db, respec.StateFailed); err != nil {
			logData.Errorf("error updating respec state to failed: %v", err)
		}

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostRespecFailed, map[string]interface{}{
				"host":  serializers.SerializeHost(h),
				"error": queueInfo.Message,
			}),
			u.Topic(),
		)
	default:
		logData.Debug("unexpected event for respeccing a host: queueInfo(%+v)", queueInfo)
	}

	return nil
}

func (s *Server) markHostAsTerminated(h *host.Host) {
	_, err := s.db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1 AND terminated_at IS NULL;", h.ID)
	if err != nil {
		logrus.Errorf("Failed to set `terminated_at` on host %d: %v", h.ID, err)
	}
}

func (s *Server) PublishContainerEvent(name controllers.EventName, c *container.Container, queueInfo *hh.QueueInfo) error {
	userIDs, err := containercollab.GetCollaboratorIDs(s.db, c.ID)
	if err != nil {
		return fmt.Errorf("Error retrieving collab user IDs for publishing container events, err: %v", err)
	}
	userIDs = append(userIDs, c.UserID)
	data := map[string]interface{}{}

	if queueInfo.RequesterID > 0 {
		user, err := user.FindByID(s.db, queueInfo.RequesterID)
		if err != nil {
			return fmt.Errorf("Failed to get requester(%d): %+v", queueInfo.RequesterID, err)
		}

		data["initiated_by"] = map[string]interface{}{
			"username": user.Username,
			"id":       user.ID,
		}
	}

	containers.PublishContainerEventToUsers(s.Events, name, c, userIDs, data)
	return nil
}
