package announcements

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
)

func ShowGlobalAnnouncements(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindForGlobal(env.DB)
	if err != nil {
		return err
	}

	env.Logger.Infof("global announcements: %+v", announcements)

	env.Status = http.StatusOK
	return nil
}
