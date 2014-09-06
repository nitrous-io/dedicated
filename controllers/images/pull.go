package images

import (
	"fmt"
	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
)

func Pull(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	repo := r.FormValue("repository")
	if repo == "" {
		return errRepoRequired
	}

	var img *image.Image
	var isNew bool
	img, err = image.Find(db, env.User, h, repo)
	if err != nil {
		repo, tag := image.ParseRepoAndTag(repo)
		if tag == "" {
			tag = image.DefaultTag
		}
		img = &image.Image{
			UserID: env.User.ID,
			HostID: h.ID,
			Name:   r.FormValue("name"),
			Slug:   repo,
			Tag:    tag,
		}
		if err := img.Insert(db); err != nil {
			return fmt.Errorf("images/pull failed to insert new image %s", err)
		}
		isNew = true
	}

	remoteImg, err := cl.PullImage(repo)
	if err != nil {
		// if we failed to pull the image, and the image is new i.e. does not
		// exist in db before, we delete the image from db
		if isNew {
			if err := img.Delete(db); err != nil {
				return fmt.Errorf("images/pull rollback new image, error %s", err)
			}
		}
		return fmt.Errorf("images/pull failed to pull image %s", err)
	}

	mergeImage(remoteImg, img)
	if err := img.InsertOrUpdate(db); err != nil { // log and continue
		env.Logger.Errorf("images.pull: successfully pulled image '%+v' in Freighter but failed to update database with '%+v'", remoteImg, img)
	}

	env.Events.Publish(
		controllers.NewEvent("image.pulled", serializers.SerializeImage(img)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(img),
	}
	return nil
}
