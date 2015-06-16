package volumes

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errSlugRequired = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
)

var (
	validations = map[string]error{
		"index_volumes_on_host_id_and_slug": httperror.New(http.StatusBadRequest, "validation_failed", "volume already exists on this host"),
	}
)

func CreateVolume(h *host.Host, options url.Values, env *middleware.Env) (*serializers.Volume, error) {
	db := env.DB

	slug := strings.ToLower(options.Get("slug"))
	if slug == "" {
		return nil, errSlugRequired
	}
	name := options.Get("name")

	cl, err := controllers.NewFreighterClient(db, h, env.UUID)
	if err != nil {
		return nil, err
	}

	vol := &volume.Volume{
		HostID: h.ID,
		Slug:   slug,
		Name:   name,
	}

	if err := vol.Insert(db); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return nil, validationErr
			}
		}
		return nil, err
	}

	if _, err := cl.CreateVolume(slug); err != nil {
		if e := vol.Delete(db); e != nil {
			logrus.Errorf("Unable to delete volume %d that failed on freighter: %v", vol.ID, e)
		}
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return nil, httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return nil, err
	}

	publishVolumeEvent(env, controllers.EventVolumeCreated, vol)
	return serializers.SerializeVolume(vol), nil
}
