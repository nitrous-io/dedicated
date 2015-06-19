package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/queue"
)

// progress event to be compatible with the progress writer
type progress struct {
	Stream string `json:"stream"`
}

var errHostIsNil = errors.New("Host is nil")

func (s *Server) HostQueueHandler() func(m *queue.Message) error {
	return func(m *queue.Message) error {
		var err error

		switch {
		case queue.Create.IsA(m.Action):
			err = s.handleCreateMessage(m)
		case queue.Stop.IsA(m.Action):
			err = s.handleStopMessage(m)
		default:
			s.logger.Debugf("uuid=%s queueHandlerTracer: unexpected message: %+v", m.UUID, m)
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
			log.Printf("uuid=%s host running but received created event %+v", m.UUID, queueInfo)
			return nil
		}

		err = s.activateHost(m.UUID, h, queueInfo.Host)
		if err != nil {
			return err
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
			log.Printf("uuid=%s failed to encode progress event %+v", m.UUID, queueInfo)
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
			s.logger.Debugf("uuid=%s queueProvisionTracer: update err: %+v h: %+v", m.UUID, err, h)
			return err
		}
		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostCreateFailed, map[string]interface{}{
				"host":  serializers.SerializeHost(h),
				"error": queueInfo.Message,
			}),
			u.Topic(),
		)
	default:
		s.logger.Debugf("uuid=%s queueTracer: unexpected event for stopping a host: queueInfo(%+v) message(%+v)", m.UUID, queueInfo, m)
	}
	return nil
}

func (s *Server) activateHost(uuid string, h *host.Host, hostInfo *hh.Info) error {
	s.logger.Debugf("uuid=%s queueProvisionTracer: queue: %+v", uuid, hostInfo)
	if hostInfo != nil {
		h.IPAddress = sql.NullString{hostInfo.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{hostInfo.ID, true}
		h.RegionIdentifier = sql.NullString{hostInfo.Region, true}
	}

	h.FreighterdPort = sql.NullInt64{int64(hostInfo.FreighterdPort), true}
	h.State = host.StateRunning

	nstmt, err := s.db.PrepareNamed(`UPDATE hosts
		SET (ip_address, provider_identifier, region_identifier, freighterd_port, state, updated_at)
		= (:ip_address, :provider_identifier, :region_identifier, :freighterd_port, :state, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}

	if err := nstmt.QueryRowx(h).StructScan(h); err != nil {
		s.logger.Debugf("uuid=%s queueProvisionTracer: update (provider) err: %+v h: %+v", uuid, err, h)
		return err
	}

	p, err := plan.FindByID(s.db, h.PlanID)
	if err != nil {
		s.logger.Debugf("uuid=%s queueProvisionTracer: failed to get the plan. err: %+v h: %+v", uuid, err, h)
	}

	_, err = s.db.Exec(`UPDATE hosts SET hourly_price=$1, free=$2, updated_at=now() WHERE id=$3 AND plan_id=$4;`,
		p.HourlyPrice(), p.IsFree(), h.ID, p.ID)
	if err != nil {
		s.logger.Debugf("uuid=%s queueProvisionTracer: update err: %+v h: %+v ", err, h)
		return err
	}
	return nil
}

func (s *Server) updateHostDetails(h *host.Host) error {
	nstmt, err := s.db.PrepareNamed(`UPDATE hosts
		SET (ip_address, provider_identifier, region_identifier, freighterd_port, state, updated_at)
		= (:ip_address, :provider_identifier, :region_identifier, :freighterd_port, :state, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
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
		s.logger.Debugf("uuid=%s queueTracer: stopped a host(%s) with message(%+v)", m.UUID, queueInfo.Slug, m)
		h, err := host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}

		cns, err := container.FindAllByHostID(db, h.ID)
		if err != nil {
			return err
		}

		if err := hostcontrol.OnStopped(s.db, h, queueInfo.RequesterID); err != nil {
			return err
		}

		for _, cn := range cns {
			state := container.State(cn.TempState.Int64)
			if !cn.TempState.Valid || (cn.TempState.Valid && state != container.StateStopping) {
				cn.State = container.StateStopped
				err := s.PublishContainerEvent(controllers.EventContainerStopped, cn, &queueInfo)
				if err != nil {
					s.logger.Debugf("uuid=%s queueTracer: failed to send an event(%s) for container(%+v)", m.UUID, controllers.EventContainerStopped, cn)
				}
			}
		}

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostStopped, map[string]*serializers.Host{
				"host": serializers.SerializeHostWithUser(h, u),
			}),
			u.Topic(),
		)
		s.logger.Debugf("uuid=%s queueTracer: published host stopped event", m.UUID)
	case controllers.EventHostStopping:
		s.logger.Debugf("uuid=%s queueTracer: stopping a host(%s) with message(%+v)", m.UUID, queueInfo.Slug, m)
	case controllers.EventHostStopFailed:
		s.logger.Debugf("uuid=%s queueTracer: failed to stop a host(%s) with message(%+v) due to %s", m.UUID, queueInfo.Slug, m, queueInfo.Message)
		h, err := host.Find(db, u, queueInfo.Slug)
		if err != nil {
			return err
		}

		if queueInfo.Host == nil {
			s.logger.Debugf("uuid=%s queueTracer: host(%s) is nil", m.UUID, queueInfo.Slug)
			return errHostIsNil
		}

		state := strings.ToLower(queueInfo.Host.State.String())
		if err := hostcontrol.OnStopFailed(s.db, s.logger, h, queueInfo.RequesterID, state); err != nil {
			return err
		}

		s.Events.Publish(
			controllers.NewEvent(controllers.EventHostUpdated, map[string]*serializers.Host{
				"host": serializers.SerializeHostWithUser(h, u),
			}),
			u.Topic(),
		)
	default:
		s.logger.Debugf("uuid=%s queueTracer: unexpected event for stapping a host: queueInfo(%+v) message(%+v)", m.UUID, queueInfo, m)
	}

	return nil
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
