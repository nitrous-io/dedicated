package server

import (
	"database/sql"
	"encoding/json"

	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/queue"
)

func (s *Server) HostQueueHandler() func(qc *queue.QueueConsumer, m *queue.Message) error {
	return func(qc *queue.QueueConsumer, m *queue.Message) error {
		db := s.db
		if queue.Create.IsA(m.Action) {
			var queueInfo hh.QueueInfo
			err := json.Unmarshal([]byte(m.Payload), &queueInfo)
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

			eventName := controllers.EventName(m.Event)
			if eventName == controllers.EventHostCreated && h.State != host.StateRunning {
				err = s.activateHost(m.UUID, h, queueInfo.Host)
				if err != nil {
					return err
				}
			}

			s.Events.Publish(
				controllers.NewEvent(eventName, map[string]*serializers.Host{
					"host": serializers.SerializeHost(h),
				}),
				u.Topic(),
			)
		}
		return nil
	}
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
	p, err := plan.FindByID(s.db, h.PlanID)
	if err != nil {
		s.logger.Debugf("uuid=%s queueProvisionTracer: failed to get the plan. err: %+v h: %+v", uuid, err, h)
	}

	h.HourlyPrice = p.HourlyPrice()

	if err := h.Update(s.db); err != nil {
		s.logger.Debugf("uuid=%s queueProvisionTracer: update err: %+v h: %+v", uuid, err, h)
		return err
	}
	s.logger.Debugf("uuid=%s queueProvisionTracer: updated h: %+v", uuid, h)
	return nil
}
