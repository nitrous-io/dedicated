package maintenance

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errUserNotFound = httperror.New(http.StatusNotFound, "user_not_found", "user not found")
)

func UserRefresh(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	strID := mux.Vars(r)["id"]

	id, err := strconv.ParseInt(strID, 10, 64)
	if err != nil {
		return err
	}

	u, err := user.FindByID(env.DB, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	hosts, err := host.FindAllByUser(env.DB, id)
	if err != nil {
		return err
	}

	for _, h := range hosts {
		logrus.Infof("Host state: %v", h.State)
		var evState controllers.EventName
		switch h.State {
		case host.StateRunning:
			evState = controllers.EventHostUpdated
		case host.StateStopped:
			evState = controllers.EventHostStopped
		case host.StateStopping:
			evState = controllers.EventHostStopping
		case host.StateStarting:
			evState = controllers.EventHostStarting
		case host.StateRestarting:
			evState = controllers.EventHostRestarting
		case host.StateUpdating:
			evState = controllers.EventHostSoftwareUpdating
		case host.StateTerminated:
			evState = controllers.EventHostDestroyed
		}
		if evState == "" {
			continue
		}
		publishHostEvent(env, u, evState, h)

		if h.State == host.StateStopped {
			publishContainersEvent(env, controllers.EventContainerStopped, container.StateStopped, h)
		}

		if h.State == host.StateTerminated {
			publishContainersEvent(env, controllers.EventContainerDestroyed, container.StateTerminated, h)
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}

	return nil
}

func publishHostEvent(env *middleware.Env, u *user.User, name controllers.EventName, h *host.Host) {
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Host{
			"host": serializers.SerializeHost(h),
		}),
		u.Topic(),
	)
}

func publishContainersEvent(env *middleware.Env, name controllers.EventName, state container.State, h *host.Host) {
	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		logrus.Errorf("Unable to list host containers: %v", err)
		return
	}

	for _, cn := range cns {
		cn.State = state
		containers.PublishContainerEvent(env, name, cn, nil)
	}
}
