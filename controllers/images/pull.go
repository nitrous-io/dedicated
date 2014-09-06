package images

import (
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

	img, err := cl.PullImage(repo)
	if err != nil {
		return err
	}

	i := convertImage(img)
	i.UserID = env.User.ID
	i.HostID = h.ID
	i.Name = r.FormValue("name")
	// If image record already exists, update it.
	if existing, err := image.Find(db, env.User, h, repo); err == nil {
		i.ID = existing.ID
	}
	if err := i.InsertOrUpdate(db); err != nil { // log and continue
		env.Logger.Errorf("images.pull: successfully pulled image '%+v' in Freighter but failed to update database with '%+v'", img, i)
	}

	env.Events.Publish(
		controllers.NewEvent("image.pulled", serializers.SerializeImage(i)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(i),
	}
	return nil
}
