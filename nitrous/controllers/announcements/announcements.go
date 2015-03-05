package announcements

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func ShowGlobalAnnouncements(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindAllForGlobal(env.DB)
	if err != nil {
		return err
	}

	env.Logger.Infof("global announcements: %+v", announcements)
	serialized := serializers.SerializeAnnouncements(announcements)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"announcements": serialized,
	}

	return nil
}
