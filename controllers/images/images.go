package images

import (
	"net/http"
	"strconv"
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
	errIDOrSlugRequired = httperror.New(http.StatusBadRequest, "invalid_request", "id or slug is required")
	errRepoRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "repository is required")
	errDestRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "destination is required")
	errTagRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "tag is required")
	errSlugRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errFileRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "file is required")
	errImageMissing     = httperror.New(http.StatusNotFound, "not_found", "image does not exist")
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
			i.Name = iRec.Name
			i.CreatedAt = iRec.CreatedAt
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
	slug := r.FormValue("slug")
	if slug == "" {
		return errSlugRequired
	}

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := cl.BuildImage(slug, f)
	if err != nil {
		return err
	}

	i := convertImage(img)
	i.UserID = env.User.ID
	i.HostID = h.ID
	i.Name = r.FormValue("name")
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

	slug := mux.Vars(r)["image"]
	if slug == "" {
		return errIDOrSlugRequired
	}

	i, err := image.Find(db, env.User, h, slug)
	if err != nil {
		return errImageMissing
	}

	if err := cl.RemoveImage(i.FullRepository()); err != nil {
		return err
	}

	if err := i.Delete(db); err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent("image.removed", i.FullRepository()),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}

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
		return errIDOrSlugRequired
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
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["image"]
	if slug == "" {
		return errIDOrSlugRequired
	}
	newRepo := r.FormValue("tag")
	if newRepo == "" {
		return errTagRequired
	}
	force, _ := strconv.ParseBool(r.FormValue("force"))

	srcImg, err := image.Find(db, env.User, h, slug)
	if err != nil {
		return errImageMissing
	}

	if err := cl.TagImage(srcImg.FullRepository(), newRepo, force); err != nil {
		return err
	}

	repo, tag := image.ParseRepoAndTag(newRepo)
	if tag == "" {
		tag = image.DefaultTag
	}
	taggedImg := &image.Image{
		UserID:      env.User.ID,
		HostID:      h.ID,
		FreighterID: srcImg.FreighterID,
		Name:        r.FormValue("name"),
		Slug:        repo,
		Tag:         tag,
	}
	// If image record already exists, update it.
	if existing, err := image.Find(db, env.User, h, newRepo); err == nil {
		taggedImg.ID = existing.ID
		if taggedImg.Name == "" {
			taggedImg.Name = existing.Name
		}
	}
	if err := taggedImg.InsertOrUpdate(db); err != nil { // log and continue
		env.Logger.Errorf("images.tag: successfully tagged image '%+v' as '%s' in Freighter but failed to update database with '%+v'", srcImg, newRepo, taggedImg)
	}

	env.Events.Publish(
		controllers.NewEvent("image.tagged", serializers.SerializeImage(taggedImg)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
		"image":  serializers.SerializeImage(taggedImg),
	}
	return nil
}

func convertImage(img *freighterclient.Image) *image.Image {
	tag := img.Tag
	if tag == "" {
		tag = image.DefaultTag
	}
	var builtAt *time.Time
	if img.CreatedAt != 0 {
		t := time.Unix(img.CreatedAt, 0)
		builtAt = &t
	}
	return &image.Image{
		FreighterID: img.ID,
		Slug:        img.Name,
		BuiltAt:     builtAt,
		Tag:         tag,
		ActualSize:  img.ActualSize,
		VirtualSize: img.VirtualSize,
	}
}
