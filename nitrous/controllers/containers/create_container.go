package containers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
)

type CreateResult struct {
	Container        *serializers.Container
	ErrorMessage     string
	ErrorDescription string
}

var (
	errNameRequired                        = httperror.New(http.StatusBadRequest, "invalid_request", "name is required")
	errImageNotFound                       = httperror.New(http.StatusNotFound, "image_not_found", "image does not exist")
	errMissingCmd                          = httperror.New(http.StatusBadRequest, "missing_cmd", "missing CMD in Docker image")
	errMaxContainersReached                = httperror.New(http.StatusBadRequest, "max_containers_reached", "workspace has exceeded the plan's maximum containers")
	errMultipleContainerCreationNotAllowed = httperror.New(http.StatusConflict, "conflict", "another container creation request is currently being processed")
)

func CreateContainer(h *host.Host, options url.Values, env *middleware.Env) (res *CreateResult, err error) {
	db := env.DB

	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err == nil || res != nil {
			return
		}
		tx.Rollback()
	}()

	// Don't allow containers to be created concurrently.
	locked, err := ndb.TransactionLock(tx, ndb.CONTAINER_CREATE_LOCK, int(env.User.ID))
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errMultipleContainerCreationNotAllowed
	}

	cnCreate, err := h.CanCreateMoreContainers(tx, env.User)
	if err != nil {
		logrus.Errorf("Error checking CanCreateMoreContainers, err: %v", err)
	} else {
		if !cnCreate {
			return nil, errMaxContainersReached
		}
	}

	name := options.Get("name")
	imageSlug := options.Get("image")
	templateSlug := options.Get("template")

	if imageSlug == "" {
		imageSlug = controllers.DefaultImage
	}

	if name == "" {
		return nil, errNameRequired
	}

	var containerSequence int
	err = tx.QueryRow("SELECT NEXTVAL('containers_slug_seq')").Scan(&containerSequence)
	if err != nil {
		logrus.Errorf("Failed to get next value of container slug sequense")
		return nil, err
	}
	slug := slugbuilder.BuildWithNumber(name, containerSequence)
	cn := &container.Container{
		UserID: env.User.ID,
		HostID: h.ID,
		Slug:   slug,
		Name:   sql.NullString{name, true},
		Image:  imageSlug,
	}
	// populate config with template-predefined settings
	config := &freighterclient.ContainerConfig{}
	if templateSlug != "" {
		tmpl, err := template.Find(env.DB, templateSlug)
		if err != nil {
			return nil, err
		}

		cfg, err := tmpl.ParsedConfig()
		if err != nil {
			return nil, err
		}

		cn.Image = tmpl.Image
		config.Image = cn.Image
		imageSlug = config.Image
		priv := cfg.Privileged
		config.Privileged = &priv
		for _, e := range cfg.Env {
			config.Env = append(config.Env, e)
		}
		for _, p := range cfg.Ports {
			config.PortBindings = append(config.PortBindings, p)
		}
		pubAll := cfg.PublishAll
		config.PublishAll = &pubAll
	}
	// populate config with user-predefined settings, override template settings if necessary
	hv, err := hostVolumes(tx, h)
	if err != nil {
		return nil, err
	}
	if err := prepareConfig(config, options, hv); err != nil {
		return nil, err
	}

	if err := cn.Validate(); err != nil {
		return nil, httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}

	// Save record to db first to reserve name (since it's globally unique).
	if err := cn.Insert(tx); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return nil, validationErr
			}
		}
		return nil, err
	}

	cl, err := controllers.NewFreighterClient(tx, h, env.UUID)
	if err != nil {
		return nil, err
	}

	res = &CreateResult{}

	created, err := cl.Create(slug, imageSlug, cn.Secret, config)
	if err != nil {
		switch e := err.(type) {
		case *freighterclient.StartFailedError:
			// A start failure is fine so we don't delete the container record.
			res.ErrorMessage = "start_failed"
			res.ErrorDescription = e.Error()
		case *freighterclient.InvalidParamError:
			res = nil
			return nil, httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		default:
			res = nil
			if err == freighterclient.ErrMissingCmd {
				return nil, errMissingCmd
			}
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
		logrus.Errorf("containers.create: Freighter returned container with unexpected state: %v", created.State)
	}

	if err := cn.Update(tx); err != nil { // log and continue
		logrus.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to update database with '%+v'", created, cn)
	}

	if len(created.Volumes) > 0 {
		// resync volumes definition returned from the host's daemon
		// This is key-value pairs of the mount destination in the container
		// and the directory name in the host's volumes path. The directory
		// name is unique for a single host and is suitable to be used as a
		// unique volume identifier for this host.
		for vname, vslug := range created.Volumes {
			// create a new volume definition if it's a newly created volume
			vol, err := volume.Find(tx, cn.HostID, vslug)
			if err != nil {
				if err != sql.ErrNoRows {
					return nil, fmt.Errorf("failed to retrieve volume %s on host %d: %v", vol, cn.HostID, err)
				}
				vol = &volume.Volume{
					HostID: cn.HostID,
					Slug:   vslug,
					Name:   fmt.Sprintf("%s:%s", cn.Slug, vname),
				}
				if err := vol.Insert(tx); err != nil {
					return nil, fmt.Errorf("failed to insert volume %v: %v", vol, err)
				}
			}

			// associate the volume with current container if it's not
			// associated yet
			if _, err := cvolume.Find(tx, cn, vol); err == sql.ErrNoRows {
				cv := &cvolume.ContainerVolume{ContainerID: cn.ID, VolumeID: vol.ID}
				if err := cv.Insert(tx); err != nil { // log and continue
					logrus.Errorf("containers.create: failed to attach volume %#v: %v ", cv, err)
				}
			}
		}
	} else {
		// legacy code, we keep this to make sure that it still accounts for old
		// daemons where the response doesnt include volumes
		// TODO: remove this when all freighter has been updated with the volume
		// changes
		for _, v := range volumeSlugs(config.Mounts) {
			cv := cvolume.ContainerVolume{ContainerID: cn.ID, VolumeID: hv[v].ID}
			if err := cv.Insert(tx); err != nil { // log and continue
				logrus.Errorf("containers.create: failed to attach volume %s to container %s", hv[v].Slug, cn.CID)
			}
		}
	}

	if _, err := tx.Exec("UPDATE users SET first_time = FALSE WHERE id = $1", env.User.ID); err != nil {
		logrus.Errorf("Failed to disable first_time for user %d, err: %v", env.User.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	PublishContainerEvent(env, controllers.EventContainerCreated, cn, nil)

	res.Container = serializers.SerializeContainer(cn)

	env.AddEventData(map[string]interface{}{
		"image": cn.Image,
	})

	return res, nil
}
