package server

import (
	"database/sql"
	"encoding/json"
	"log"
	"strings"

	"github.com/nitrous-io/dedicated/event"
	hh "github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
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

func (s *Server) HostQueueHandler() func(qc *queue.QueueConsumer, m *queue.Message) error {
	return func(qc *queue.QueueConsumer, m *queue.Message) error {
		db := s.db
		if queue.Create.IsA(m.Action) {
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
			if eventName == controllers.EventHostCreated {
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
			} else if eventName == controllers.EventName(event.HostCreating) {
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
			} else if eventName == controllers.EventName(event.HostCreateFailed) {
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
			}
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
