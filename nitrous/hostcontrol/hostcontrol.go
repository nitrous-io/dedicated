package hostcontrol

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous-common/logger"
)

type EventType int64
type eventFunc func(ev EventType)

const (
	EventHostStopping EventType = iota + 1
	EventHostStopped
	EventHostUpdated
)

var HostbuilderClient *hostbuilderclient.Hostbuilder

func init() {
	hostbuilderURL := envvars.HostBuilderURL
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)
}

func Stop(db *db.Database, l *logger.Logger, h *host.Host, ev eventFunc) error {
	origState := h.State
	h.State = host.StateStopping

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, origState, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if ev != nil {
		ev(EventHostStopping)
	}

	headers := map[string]string{
		constants.NitrousUUIDHeader: "hostcontrol",
	}
	if _, sErr := HostbuilderClient.Stop(h.Provider, h.MachineConfig(), headers); sErr != nil {
		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		h.State = origState
		tx, err := db.Beginx()
		if err != nil {
			return err
		}
		if err := h.Update(tx); err != nil {
			tx.Rollback()
			l.Errorf("hosts.Stop: Unable to reset host state: %v", err)
		}
		if err := hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt); err != nil {
			tx.Rollback()
			l.Errorf("hosts.Stop: Unable to record transition: %v", err)
		}
		if err := tx.Commit(); err != nil {
			l.Errorf("hosts.Stop: Failed to commit transaction: %v", err)
		}

		if ev != nil {
			ev(EventHostUpdated)
		}
		return sErr
	}

	h.State = host.StateStopped

	tx, err = db.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if ev != nil {
		ev(EventHostStopped)
	}
	return nil
}
