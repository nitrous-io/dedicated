package images

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"

	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errIDOrNameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "id or name is required")
	errRepoRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "repository is required")
	errDestRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "destination is required")
	errTagRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "tag is required")
	errNameRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "name is required")
	errFileRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "file is required")
)

func Index(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	imgs, err := cl.ListImages()
	if err != nil {
		return err
	}

	images := make([]*image.Image, 0, len(imgs))
	for _, img := range imgs {
		i := convertImage(img)
		i.UserID = env.User.ID
		i.HostID = h.ID

		if iRec, err := image.Find(env.DB, env.User, h, img.ID); err == nil {
			i.ID = iRec.ID
			i.UpdatedAt = iRec.UpdatedAt
		} else { // log and continue
			env.Logger.Errorf("images.index: image '%+v' exists Freighter but not in database: %v", img, err)
		}

		images = append(images, i)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"images": serializers.SerializeImages(images),
	}

	return nil
}

func Build(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	_, fh, err := r.FormFile("file")
	if err != nil {
		return errFileRequired
	}

	name := r.FormValue("name")
	if name == "" {
		return errNameRequired
	}

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := cl.BuildImage(name, f)
	if err != nil {
		return err
	}

	i := convertImage(img)
	i.UserID = env.User.ID
	i.HostID = h.ID
	if err := i.Insert(env.DB); err != nil { // log and continue
		env.Logger.Errorf("images.build: successfully built image '%+v' in Freighter but failed to update database with '%+v'", img, i)
	}

	env.Events.Publish(
		controllers.NewEvent("image.built", serializers.SerializeImage(i)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(i),
	}

	return nil
}

func Remove(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	name := mux.Vars(r)["image"]
	if name == "" {
		return errIDOrNameRequired
	}

	if err := cl.RemoveImage(name); err != nil {
		return err
	}

	if i, err := image.Find(db, env.User, h, name); err == nil {
		if err := i.Delete(db); err != nil { // log and continue
			env.Logger.Errorf("images.remove: successfully removed image '%+v' in Freighter but failed to remove from database: %v", i, err)
		}
	} else { // log and continue
		env.Logger.Errorf("images.remove: successfully removed image '%s' in Freighter but it doesn't exist in database: %v", name, err)
	}

	env.Events.Publish(
		controllers.NewEvent("image.removed", name),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}

func Pull(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
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
	if err := i.Insert(env.DB); err != nil { // log and continue
		env.Logger.Errorf("images.pull: successfully pulled image '%+v' in Freighter but failed to update database with '%+v'", img, i)
	}

	env.Events.Publish(
		controllers.NewEvent("image.pulled", serializers.SerializeImage(i)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
		"image":  serializers.SerializeImage(i),
	}
	return nil
}

func Push(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	image := mux.Vars(r)["image"]
	if image == "" {
		return errIDOrNameRequired
	}

	dest := r.FormValue("destination")
	if dest == "" {
		return errDestRequired
	}

	if err := cl.PushImage(image, dest); err != nil {
		return err
	}

	type data struct {
		Image       string `json:"image"`
		Destination string `json:"destination"`
	}
	env.Events.Publish(
		controllers.NewEvent("image.pushed", data{Image: image, Destination: dest}),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}

func Tag(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	image := mux.Vars(r)["image"]
	if image == "" {
		return errIDOrNameRequired
	}

	tag := r.FormValue("tag")
	if tag == "" {
		return errTagRequired
	}

	force := r.FormValue("force") != ""

	if err := cl.TagImage(image, tag, force); err != nil {
		return err
	}

	// FIXME TagImage should return new image info. Or we should make another
	// request to Freighter to get the new image.

	type data struct {
		Image string `json:"image"`
		Tag   string `json:"tag"`
	}
	env.Events.Publish(
		controllers.NewEvent("image.tagged", data{Image: image, Tag: tag}),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}

func convertImage(img *freighterclient.Image) *image.Image {
	tag := img.Tag
	if tag == "" {
		tag = image.DefaultTag
	}
	createdAt := time.Unix(img.CreatedAt, 0)
	return &image.Image{
		FreighterID: img.ID,
		Name:        img.Name,
		CreatedAt:   &createdAt,
		Tag:         tag,
		ActualSize:  img.ActualSize,
		VirtualSize: img.VirtualSize,
	}
}
