package hostcontrol

import (
	"net"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/nitrous-common/logger"
)

type EventType int64
type eventFunc func(ev EventType)

const (
	EventHostStopping EventType = iota + 1
	EventHostStopped
	EventHostUpdated
)

var HostbuilderClient = provisioner.HostbuilderClient

func Stop(db *db.Database, l *logger.Logger, h *host.Host, headers http.Header, ev eventFunc, initiatorID int64) error {
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
	if err := hosttransition.Record(tx, h, origState, *h.UpdatedAt, initiatorID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if ev != nil {
		ev(EventHostStopping)
	}

	urlArgs := host.URLArgsFromHost(h)
	if _, stopErr := HostbuilderClient.Stop(h.Provider, h.MachineConfig(), headers, urlArgs); stopErr != nil {
		if e, ok := stopErr.(net.Error); ok && e.Timeout() {
			l.Errorf("hostbuilder timeout stopping host: %v", e)
		}

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
		if err := hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt, initiatorID); err != nil {
			tx.Rollback()
			l.Errorf("hosts.Stop: Unable to record transition: %v", err)
		}
		if err := tx.Commit(); err != nil {
			l.Errorf("hosts.Stop: Failed to commit transaction: %v", err)
		}

		if ev != nil {
			ev(EventHostUpdated)
		}
		return stopErr
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
	if err := hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt, initiatorID); err != nil {
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

func Terminate(tx *sqlx.Tx, l *logger.Logger, h *host.Host, headers http.Header, initiatorID int64) error {
	// If provider identifier is empty, don't attempt to delete the host in hostbuilder
	// Provider identifier can be empty for hosts that are dangling in born or orphan states.
	if h.ProviderIdentifier.String != "" {
		urlArgs := host.URLArgsFromHost(h)
		_, err := HostbuilderClient.Delete(h.Provider, h.MachineConfig(), headers, urlArgs)
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Timeout() {
				l.Errorf("hostbuilder timeout deleting host: %v", e)
			}
			return err
		}
	}

	origState := h.State

	if err := h.Delete(tx); err != nil {
		return err
	}
	if err := hosttransition.Record(tx, h, origState, *h.TerminatedAt, initiatorID); err != nil {
		return err
	}
	if err := container.DeleteAllByHostID(tx, h.ID); err != nil {
		return err
	}
	if err := keypairs.Delete(tx, h.ID); err != nil {
		return err
	}
	if err := cert.Delete(tx, h.ID); err != nil {
		return err
	}

	return nil
}
