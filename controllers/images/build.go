package images

import (
	"io"
	"net/http"
	"strconv"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
)

type result struct {
	*image.Image
	err error
}

func build(
	env *middleware.Env,
	cl *controllers.FreighterClient,
	h *host.Host,
	intermediateImg *image.Image,
	slug string,
	f io.Reader,
) <-chan result {
	ch := make(chan result)
	go func() {
		img, err := cl.BuildImage(slug, f)
		if err != nil {
			delErr := intermediateImg.Delete(env.DB)
			if delErr != nil {
				env.Logger.Errorf("Unable to delete image %d that failed to build on freighter: %v", intermediateImg.ID, delErr)
			}
			ch <- result{err: err}
			return
		}

		i := convertImage(img)
		i.ID = intermediateImg.ID
		i.UserID = env.User.ID
		i.HostID = h.ID
		i.Name = intermediateImg.Name
		if err := i.Update(env.DB); err != nil { // log and continue
			env.Logger.Errorf("images.build: successfully built image '%+v' in Freighter but failed to update database with '%+v'", img, i)
		}

		// FIXME When BuildImage is changed to stream the build progress, this
		// should be changed to publish the build progress periodically.
		env.Events.Publish(
			controllers.NewEvent("image.built", serializers.SerializeImage(i)),
			env.User.Topic(),
		)

		ch <- result{Image: i}
	}()
	return ch
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
	slug = slugToLower(slug)

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	repo, tag := image.ParseRepoAndTag(slug)
	if tag == "" {
		tag = image.DefaultTag
	}

	var img *image.Image

	img = &image.Image{
		UserID: env.User.ID,
		HostID: h.ID,
		Name:   r.FormValue("name"),
		Slug:   repo,
		Tag:    tag,
	}
	if err := img.Insert(env.DB); err != nil {
		return err
	}

	async, _ := strconv.ParseBool(r.FormValue("async"))
	if async {
		go build(env, cl, h, img, slug, f)
		env.Status = http.StatusAccepted
		env.Data = map[string]interface{}{
			"image": serializers.SerializeImage(img),
		}
	} else {
		result := <-build(env, cl, h, img, slug, f)
		if result.err != nil {
			return err
		}
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"image": serializers.SerializeImage(result.Image),
		}
	}

	return nil
}
