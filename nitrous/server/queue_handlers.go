package server

import (
	"database/sql"
	"encoding/json"

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
			var hostInfo queue.HostQueuePublisher
			err := json.Unmarshal([]byte(m.Payload), &hostInfo)
			if err != nil {
				return err
			}
			uid := hostInfo.UserID
			slug := hostInfo.Slug

			u, err := user.FindByID(db, uid)
			if err != nil {
				return err
			}

			h, err := host.Find(db, u, slug)
			if err != nil {
				return err
			}

			eventName := controllers.EventName(m.Event)
			if eventName == controllers.EventHostCreated && h.State != host.StateRunning {
				err = s.activateHost(h, &hostInfo)
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

func (s *Server) activateHost(h *host.Host, hostinfo *queue.HostQueuePublisher) error {
	s.logger.Debugf("queueProvisionTracer: hostinfo: %+v", hostinfo)
	if hostinfo != nil {
		h.IPAddress = sql.NullString{hostinfo.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{hostinfo.ID, true}
		h.RegionIdentifier = sql.NullString{hostinfo.Region, true}
	}

	h.FreighterdPort = sql.NullInt64{int64(hostinfo.FreighterdPort), true}
	h.State = host.StateRunning
	p, err := plan.FindByID(s.db, h.PlanID)
	if err != nil {
		s.logger.Debugf("queueProvisionTracer: failed to get the plan. err: %+v h: %+v ", err, h)
	}

	h.HourlyPrice = p.HourlyPrice()

	if err := h.Update(s.db); err != nil {
		s.logger.Debugf("queueProvisionTracer: update err: %+v h: %+v ", err, h)
		return err
	}
	s.logger.Debugf("qeueuProvisionTracer: updated h: %+v", h)

	return nil
}
