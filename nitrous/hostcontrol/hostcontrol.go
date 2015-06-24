package hostcontrol

import (
	"net"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
)

type EventType int64
type EventFunc func(ev EventType)

const (
	EventHostStopping EventType = iota + 1
	EventHostStopped
	EventHostUpdated
)

var (
	HostbuilderClient = provisioner.HostbuilderClient
	errHostLocked     = httperror.New(http.StatusConflict, "workspace_locked", "workspace is locked by another request")
)

func Stop(db *db.Database, h *host.Host, headers http.Header, ev EventFunc, initiatorID int64) error {
	origState := h.State
	err := beforeStop(db, h, ev, initiatorID, origState)
	if err != nil {
		return err
	}

	urlArgs := host.URLArgsFromHost(h)
	if _, stopErr := HostbuilderClient.Stop(h.Provider, h.MachineConfig(), headers, urlArgs); stopErr != nil {
		if e, ok := stopErr.(net.Error); ok && e.Timeout() {
			logrus.Errorf("hostbuilder timeout stopping host: %v", e)
		}

		if err := OnStopFailed(db, h, initiatorID, origState); err != nil {
			return err
		}

		if ev != nil {
			ev(EventHostUpdated)
		}

		return stopErr
	}

	if err := OnStopped(db, h, initiatorID); err != nil {
		return err
	}

	if ev != nil {
		ev(EventHostStopped)
	}

	return nil
}

func StopWithQueue(db *db.Database, h *host.Host, headers http.Header, ev EventFunc, initiatorID int64) error {
	origState := h.State
	err := beforeStop(db, h, ev, initiatorID, origState)
	if err != nil {
		return err
	}

	urlArgs := host.URLArgsFromHost(h)
	urlArgs.Set("requester_id", strconv.FormatInt(initiatorID, 10))
	if _, sErr := HostbuilderClient.Stop(h.Provider, h.MachineConfig(), headers, urlArgs); sErr != nil {
		return sErr
	}

	return nil
}

func beforeStop(db *db.Database, h *host.Host, ev EventFunc, initiatorID int64, origState string) error {
	if err := RecordNewState(db, h, host.StateStopping, initiatorID); err != nil {
		return err
	}

	if ev != nil {
		ev(EventHostStopping)
	}

	return nil
}

func OnStopFailed(db *db.Database, h *host.Host, initiatorID int64, origState string) error {
	err := RecordNewState(db, h, origState, initiatorID)
	if err != nil {
		logrus.Errorf("hosts.Stop: Unable to restore host state: %v", err)
	}
	return err
}

func OnStopped(db *db.Database, h *host.Host, initiatorID int64) error {
	return RecordNewState(db, h, host.StateStopped, initiatorID)
}

func Terminate(tx *sqlx.Tx, h *host.Host, headers http.Header, initiatorID int64) error {
	// If provider identifier is empty, don't attempt to delete the host in hostbuilder
	// Provider identifier can be empty for hosts that are dangling in born or orphan states.
	if h.ProviderIdentifier.String != "" {
		urlArgs := host.URLArgsFromHost(h)
		_, err := HostbuilderClient.Delete(h.Provider, h.MachineConfig(), headers, urlArgs)
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Timeout() {
				logrus.Errorf("hostbuilder timeout deleting host: %v", e)
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

func RecordNewState(db *db.Database, h *host.Host, newState string, initiatorID int64) error {
	return db.WithTransaction(func(tx *sqlx.Tx) error {
		origState := h.State
		if err := h.UpdateState(tx, newState); err != nil {
			if err == host.ErrUpdateFailed {
				return errHostLocked
			}
			return err
		}

		return hosttransition.Record(tx, h, origState, *h.UpdatedAt, initiatorID)
	})
}
