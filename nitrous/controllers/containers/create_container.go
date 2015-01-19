package containers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/lib/pq"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

type CreateResult struct {
	Container        *serializers.Container
	ErrorMessage     string
	ErrorDescription string
}

var (
	errSlugRequired  = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errImageNotFound = httperror.New(http.StatusNotFound, "image_not_found", "image does not exist")
)

func CreateContainer(h *host.Host, options url.Values, env *middleware.Env) (*CreateResult, error) {
	db := env.DB
	log := env.Logger

	slug := strings.ToLower(options.Get("slug"))
	name := options.Get("name")
	imageSlug := options.Get("image")
	templateSlug := options.Get("template")

	if imageSlug == "" {
		imageSlug = controllers.DefaultImage
	}

	if slug == "" {
		return nil, errSlugRequired
	}

	if name == "" {
		name = slug
	}

	hv, err := hostVolumes(db, h)
	if err != nil {
		return nil, err
	}
	config, err := prepareConfig(options, hv)
	if err != nil {
		return nil, err
	}

	cn := &container.Container{
		UserID: env.User.ID,
		HostID: h.ID,
		Slug:   slug,
		Image:  imageSlug,
		Name:   sql.NullString{name, true},
	}

	if templateSlug != "" {
		tmpl, err := template.Find(env.DB, templateSlug)
		if err != nil {
			return nil, err
		}

		cfg, err := tmpl.ParsedConfig()
		if err != nil {
			return nil, err
		}

		if len(cfg.Containers) == 0 {
			return nil, fmt.Errorf("Supplied template `%s` is missing containers", templateSlug)
		}

		primary := cfg.Containers[0]
		cn.Image = primary.Image
		config.Image = cn.Image
		imageSlug = config.Image
		priv := primary.Privileged
		config.Privileged = &priv
		if primary.Cmd != "" {
			config.Cmd = []string{primary.Cmd}
		}
		for _, e := range primary.Env {
			config.Env = append(config.Env, e)
		}
		for _, p := range primary.Ports {
			config.PortBindings = append(config.PortBindings, p)
		}
		pubAll := primary.PublishAll
		config.PublishAll = &pubAll

		if len(cfg.Containers) > 1 {
			env.Logger.Warnf("Started `%s` has multiple containers, unsupported.", templateSlug)
		}
	}

	if err := cn.Validate(); err != nil {
		return nil, httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}

	// Save record to db first to reserve name (since it's globally unique).
	if err := cn.Insert(db); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return nil, validationErr
			}
		}
		return nil, err
	}

	deleteContainer := func(cn *container.Container) {
		err := cn.DeletePermanently(db)
		if err != nil {
			log.Errorf("Unable to delete container %d that failed on freighter: %v", cn.ID, err)
		}
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		deleteContainer(cn)
		return nil, err
	}

	res := &CreateResult{}

	created, err := cl.Create(slug, imageSlug, cn.Secret, config)
	if err != nil {
		switch e := err.(type) {
		case *freighterclient.StartFailedError:
			// A start failure is fine so we don't delete the container record.
			res.ErrorMessage = "start_failed"
			res.ErrorDescription = e.Error()
		case *freighterclient.InvalidParamError:
			deleteContainer(cn)
			return nil, httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		default:
			deleteContainer(cn)
			if err == freighterclient.ErrImageNotFound {
				return nil, errImageNotFound
			}
			return nil, err
		}
	}

	// Update existing record with new info from Freighter.
	cn.CID = created.ID
	cn.Image = created.Image
	cn.IPAddress = created.Host
	cn.GatewayIP = created.Gateway
	cn.Cmd = created.Cmd
	cn.Entrypoint = created.Entrypoint
	cn.ExitCode = created.ExitCode
	cn.StartedAt = timePtr(created.StartedAt)
	cn.FinishedAt = timePtr(created.FinishedAt)

	if ok := cn.SetState(created.State); !ok { // log and continue
		log.Errorf("containers.create: Freighter returned container with unexpected state: %v", created.State)
	}

	if err := cn.Update(db); err != nil { // log and continue
		log.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to update database with '%+v'", created, cn)
	}

	for _, v := range volumeSlugs(config.Mounts) {
		cv := cvolume.ContainerVolume{ContainerID: cn.ID, VolumeID: hv[v].ID}
		if err := cv.Insert(db); err != nil { // log and continue
			log.Errorf("containers.create: failed to attach volume %s to container %s", hv[v].Slug, cn.CID)
		}
	}

	publishContainerEvent(env, controllers.EventContainerCreated, cn)

	res.Container = serializers.SerializeContainer(cn)

	env.AddEventData(map[string]interface{}{
		"slug":         cn.Slug,
		"image":        cn.Image,
		"host_id":      cn.HostID,
		"container_id": cn.ID,
	})

	return res, nil
}
