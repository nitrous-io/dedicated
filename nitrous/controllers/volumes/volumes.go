package volumes

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/nitrous/controllers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errNotFound                 = httperror.New(http.StatusNotFound, "volume_not_found", "volume could not be found")
	errCannotDeleteSystemVolume = httperror.New(http.StatusForbidden, "forbidden", "system volume cannot be deleted")
	errCannotDeleteVolumeInUse  = httperror.New(http.StatusForbidden, "forbidden", "cannot delete volume that's being used by a container")
)

var (
	CreateVolumeFn = CreateVolume
)

func Index(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	vols, err := volume.FindAll(env.DB, h.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"volumes": serializers.SerializeVolumes(vols),
	}
	return nil
}

func Create(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	vol, err := CreateVolumeFn(h, r.Form, env)
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"volume": vol,
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	slug := mux.Vars(r)["vslug"]

	vol, err := getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	if vol.System {
		return errCannotDeleteSystemVolume
	}

	belongsToContainer, err := cvolume.BelongsToContainer(db, vol)
	if belongsToContainer {
		return errCannotDeleteVolumeInUse
	}

	cl, err := controllers.NewFreighterClient(db, h, env.UUID)
	if err != nil {
		return err
	}

	if err := cl.RemoveVolume(slug); err != nil {
		return err
	}

	if err := vol.Delete(db); err != nil {
		env.Logger.Errorf("Unable to delete volume %d that was destroyed on freighter: %v", vol.ID, err)
		return err
	}

	publishVolumeEvent(env, controllers.EventVolumeDestroyed, vol)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Show(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	slug := mux.Vars(r)["vslug"]

	vol, err := getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Info(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	cl, err := controllers.NewFreighterClient(db, h, env.UUID)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["vslug"]

	_, err = getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	vi, err := cl.GetVolumeInfo(slug)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"info": serializers.SerializeVolumeInfo(vi),
	}
	return nil
}

func getVolume(db ndb.QueryReceiver, hostID int64, slug string) (*volume.Volume, error) {
	if slug == "" {
		return nil, errSlugRequired
	}
	vol, err := volume.Find(db, hostID, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errNotFound
		}
		return nil, err
	}
	return vol, nil
}

func publishVolumeEvent(env *middleware.Env, name controllers.EventName, v *volume.Volume) {
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Volume{
			"volume": serializers.SerializeVolume(v),
		}),
		env.User.Topic(),
	)
}
