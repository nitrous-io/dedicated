package dockerregistry

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/notifications"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errTokenMissing = httperror.New(http.StatusUnauthorized, "invalid_token", "auth token is required")
	errInvalidToken = httperror.New(http.StatusUnauthorized, "invalid_token", "auth token is invalid")
)

func NotificationWebhook(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	token, err := middleware.GetBearerToken(r)
	if err != nil {
		return errTokenMissing
	}
	if token != envvars.DockerRegistryWebhookToken {
		return errInvalidToken
	}

	j := &struct {
		Events []*notifications.Event `json:"events"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(j); err != nil {
		return err
	}

	for _, event := range j.Events {
		if err := processEvent(event, env.DB); err != nil {
			logrus.Errorf("error processing Docker Registry notification event %v, err: %v", event, err)
			if err == sql.ErrNoRows {
				// If we can't find a snapshot, we return an error so that the Docker
				// Registry will re-attempt delivery. This is because the Docker
				// Registry can potentially deliver the webhook notification before we
				// actually save a snapshot to the database.
				env.Status = http.StatusNotFound
				env.Data = map[string]interface{}{}
				return nil
			}
			// Continue processing other events, and never return an error, so
			// that Docker Registry will not re-attempt delivery.
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{}

	return nil
}

func processEvent(event *notifications.Event, db ndb.QueryReceiver) error {
	switch event.Action {
	case "push":
		logrus.Debugf("received push event from Docker Registry for %q", event.Target.Repository)
	case "pull":
		logrus.Debugf("received pull event from Docker Registry for %q", event.Target.Repository)
	default:
		logrus.Debugf("received %q event from Docker Registry, ignoring", event.Action)
	}
	return nil
}
