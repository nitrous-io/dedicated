package freighterd

import (
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

type message struct {
	Event             string `json:"event"`
	DockerVersion     string `json:"docker_version"`
	FreighterdVersion string `json:"freighterd_version"`
	Error             string `json:"error"`
}

var (
	errNotFound    = httperror.New(http.StatusNotFound, "not_found", "not found")
	errNotLoggedIn = httperror.New(http.StatusUnauthorized, "unauthorized", "unauthorized")
)

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
		logrus.Errorf("expecting webhook message for host ID %d to verify but didn't, request: %+v", h.ID, r)
		return errNotFound
	}

	switch m.Event {
	case "ping":
		// check for token only on ping
		if _, err := token.FindByUser(env.DB, u.ID); err != nil {
			return errNotLoggedIn
		}

		if err := h.UpdateColumn(env.DB, "last_pinged_at", pq.NullTime{time.Now(), true}); err != nil {
			return err
		}

		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"status": "ok",
		}
		return nil
	}

	return errNotFound
}
