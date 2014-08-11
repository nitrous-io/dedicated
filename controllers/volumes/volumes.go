package volumes

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lib/pq"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	errSlugRequired = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errNotFound     = httperror.New(http.StatusNotFound, "not_found", "volume could not be found")
)

var (
	validations = map[string]error{
		"index_volumes_on_host_id_and_slug": httperror.New(http.StatusBadRequest, "validation_failed", "volume already exists on this host"),
	}
)

func Index(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	vols, err := volume.FindAllByHostID(c.DB(), h.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status":  "ok",
		"volumes": serializers.SerializeVolumes(vols),
	}
	return nil
}

func Create(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	slug := r.FormValue("slug")
	if slug == "" {
		return errSlugRequired
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	vol := &volume.Volume{
		HostID: h.ID,
		Slug:   slug,
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	err = vol.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	_, err = cl.CreateVolume(slug)
	if err != nil {
		desErr := vol.Delete(db)
		if desErr != nil {
			c.Logger().Error("Unable to delete volume %d that failed on freighter: %v", vol.ID, desErr)
		}
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["vslug"]

	vol, err := getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	err = cl.RemoveVolume(slug)
	if err != nil {
		return err
	}

	err = vol.Delete(db)
	if err != nil {
		c.Logger().Error("Unable to delete volume %d that was destroyed on freighter: %v", vol.ID, err)
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Show(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	cl, err := controllers.NewFreighterClient(db, h)
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
		"status": "ok",
		"volume": serializers.SerializeFreighterVolume(vi),
	}
	return nil
}

func getVolume(db ndb.QueryReceiver, hostID int64, slug string) (*volume.Volume, error) {
	if slug == "" {
		return nil, errSlugRequired
	}
	vol, err := volume.FindByHostIDAndSlug(db, hostID, slug)
	if err != nil {
		return nil, err
	}
	if vol == nil {
		return nil, errNotFound
	}
	return vol, nil
}
