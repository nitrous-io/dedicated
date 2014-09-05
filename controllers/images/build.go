package images

import (
	"archive/tar"
	"bufio"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/pkg/isarchive"
	"github.com/nitrous-io/nitrous/serializers"
)

type buildResult struct {
	*image.Image
	err error
}

type buildContext struct {
	env    *middleware.Env
	client *controllers.FreighterClient
	img    *image.Image
	slug   string
	input  io.Reader
}

// build sends the build context to freighter in a goroutine. If we want an
// async build, this returns immediately with the current stored image,
// otherwise, the image is updated with new data after the build is done.
func (c *buildContext) build(async bool) (*image.Image, error) {
	ch := make(chan *buildResult, 1)
	env, cl, img, slug, input := c.env, c.client, c.img, c.slug, c.input

	go func() {
		remoteImg, err := cl.BuildImage(slug, input)
		if err != nil {
			if delErr := img.Delete(env.DB); delErr != nil {
				env.Logger.Errorf("Unable to delete image %d that failed to build on freighter: %v", img.ID, delErr)
			}
			ch <- &buildResult{err: err}
			return
		}

		mergeImage(remoteImg, img)
		if err := img.Update(env.DB); err != nil { // log and continue
			env.Logger.Errorf("images.build: successfully built image '%+v' in Freighter but failed to update database with '%+v'", remoteImg, img)
		}

		// FIXME When BuildImage is changed to stream the build progress, this
		// should be changed to publish the build progress periodically.
		env.Events.Publish(
			controllers.NewEvent("image.built", serializers.SerializeImage(img)),
			env.User.Topic(),
		)

		ch <- &buildResult{Image: img}
	}()

	// if the build is async, we will just return the stored image immmediately
	// without waiting for result from freighter
	if async {
		return img, nil
	}
	r := <-ch
	if r.err != nil {
		return nil, r.err
	}
	return r.Image, nil
}

func Build(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	slug := r.FormValue("slug")
	if slug == "" {
		return errSlugRequired
	}
	slug = slugToLower(slug)

	var archive io.Reader

	code := r.FormValue("code")
	if code != "" {
		archive, err = makeArchive(bytes.NewBufferString(code))
	} else {
		_, fh, err := r.FormFile("file")
		if err != nil {
			return errFileRequired
		}

		f, err := fh.Open()
		if err != nil {
			return err
		}
		defer f.Close()
		archive, err = ensureArchive(f)
	}
	if err != nil {
		return err
	}

	repo, tag := image.ParseRepoAndTag(slug)
	if tag == "" {
		tag = image.DefaultTag
	}

	img := &image.Image{
		UserID: env.User.ID,
		HostID: h.ID,
		Name:   r.FormValue("name"),
		Slug:   repo,
		Tag:    tag,
	}
	if err := img.Insert(env.DB); err != nil {
		return err
	}

	ctx := &buildContext{env: env, client: cl, img: img, slug: slug, input: archive}
	async, _ := strconv.ParseBool(r.FormValue("async"))
	img, err = ctx.build(async)
	if err != nil {
		return err
	}
	if async {
		env.Status = http.StatusAccepted
	} else {
		env.Status = http.StatusOK
	}
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(img),
	}

	return nil
}

// mergeImage merge changes from remoteImg ( which is returned by freighter )
// to dbImg ( which is the record we stored in database )
func mergeImage(remoteImg *freighterclient.Image, dbImg *image.Image) {
	tag := remoteImg.Tag
	if tag == "" {
		tag = image.DefaultTag
	}
	var builtAt *time.Time
	if remoteImg.CreatedAt != 0 {
		t := time.Unix(remoteImg.CreatedAt, 0).UTC()
		builtAt = &t
	}
	dbImg.FreighterID = remoteImg.ID
	dbImg.Slug = remoteImg.Name
	dbImg.BuiltAt = builtAt
	dbImg.Tag = tag
	dbImg.ActualSize = remoteImg.ActualSize
	dbImg.VirtualSize = remoteImg.VirtualSize
}

func ensureArchive(f multipart.File) (io.Reader, error) {
	bufRdr := bufio.NewReader(f)

	header, err := bufRdr.Peek(isarchive.MaxHeaderLength)
	// Peek can return err even if header is read if read bytes is shorter than requested
	if err != nil && header == nil {
		return nil, err
	}

	// if not archive, make tarball
	if !isarchive.IsArchive(header) {
		b := &bytes.Buffer{}
		_, err := b.ReadFrom(bufRdr)
		if err != nil {
			return nil, err
		}
		return makeArchive(b)
	}

	// pass any other file directly to freighter
	// archive validation will be done in freighter end.
	return bufRdr, nil
}

func makeArchive(dockerfileBuf *bytes.Buffer) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)

	now := time.Now()
	hdr := &tar.Header{
		Name:       "Dockerfile",
		Size:       int64(dockerfileBuf.Len()),
		ModTime:    now,
		AccessTime: now,
		ChangeTime: now,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(dockerfileBuf.Bytes()); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}
