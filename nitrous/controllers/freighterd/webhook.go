package freighterd

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

type message struct {
	Event             string `json:"event"`
	DockerVersion     string `json:"docker_version"`
	FreighterdVersion string `json:"freighterd_version"`
	Error             string `json:"error"`
}

var errNotFound = httperror.New(http.StatusNotFound, "not_found", "not found")

func Webhook(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hostSlug := mux.Vars(r)["slug"]
	h, err := host.FindByIdentifier(env.DB, hostSlug)
	if err != nil {
		return middleware.ErrHostNotFound
	}

	u, err := user.FindByID(env.DB, h.UserID)
	if err != nil {
		return errNotFound
	}

	// Verify that msg was generated with host's secret.
	msg := r.PostFormValue("msg")
	verifier := msgverifier.New(h.Secret)
	var m message
	if err := verifier.Verify(msg, &m); err != nil {
		env.Logger.Errorf("expecting webhook message for host ID %d to verify but didn't, request: %+v", h.ID, r)
		return errNotFound
	}

	switch m.Event {
	case "ping":
		if err := h.UpdateColumn(env.DB, "last_pinged_at", pq.NullTime{time.Now(), true}); err != nil {
			return err
		}

		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"status": "ok",
		}
		return nil

	case "started":
		if h.State != host.StateUpgrading {
			return errNotFound
		}

		if err := h.UpdateColumn(env.DB, "state", host.StateRunning); err != nil {
			return err
		}

		env.Events.Publish(
			controllers.NewEvent(controllers.EventHostUpgraded, map[string]*serializers.Host{
				"host": serializers.SerializeHost(h),
			}),
			u.Topic(),
		)

		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"status": "ok",
		}
		return nil

	case "upgrade":
		if h.State != host.StateUpgrading {
			return errNotFound
		}

		h.State = host.StateRunning
		if err := h.Update(env.DB); err != nil {
			return err
		}

		env.Logger.Errorf("freighter upgrade failed with err: %s", m.Error)
		env.Events.Publish(
			controllers.NewEvent(controllers.EventHostUpgradeFailed, map[string]interface{}{
				"host":  serializers.SerializeHost(h),
				"error": m.Error,
			}),
			u.Topic(),
		)

		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"status": "ok",
		}
		return nil
	}

	return errNotFound
}
