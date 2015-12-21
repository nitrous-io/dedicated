package containers

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
)

// CreateAtomic is the handler for the new create endpoint that does not require
// callers to check whether the user can create a host, nor to pull the Docker
// image first.
var CreateAtomic = middleware.HostViaForm("host", middleware.TouchHost(requiresRunningHost(createAtomic)))

var (
	errImageParamsRequired    = httperror.New(http.StatusBadRequest, "invalid_request", `either "image", "template" or "snapshot_id" must be specified`)
	errMaxProjects            = httperror.New(http.StatusForbidden, "forbidden", "you have reached the maximum number of projects allowed by your plan")
	errInvalidSnapshot        = httperror.New(http.StatusBadRequest, "invalid_request", "snapshot is invalid")
	errSnapshotNoOrg          = httperror.New(http.StatusForbidden, "forbidden", "you are not allowed to use this snapshot")
	errNotOrgMember           = httperror.New(http.StatusForbidden, "forbidden", "you are not allowed to use this snapshot because you're not a member of the organization")
	errReadSnapshotNotAllowed = httperror.New(http.StatusForbidden, "forbidden", "you are not allowed to use this snapshot")
)

// createAtomic atomically (to callers) creates a container.
func createAtomic(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if err := validateCreateAtomicForm(r); err != nil {
		return err
	}

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}

	// Don't allow containers to be created concurrently.
	locked, err := ndb.TransactionLock(tx, ndb.CONTAINER_CREATE_LOCK, int(env.User.ID))
	if err != nil {
		tx.Rollback()
		return err
	}
	if !locked {
		tx.Rollback()
		return errMultipleContainerCreationNotAllowed
	}

	allowed, err := h.CanCreateMoreContainers(tx, env.User)
	if err != nil {
		tx.Rollback()
		return err
	}
	if !allowed {
		tx.Rollback()
		return errMaxProjects
	}

	cl, err := controllers.NewFreighterClient(tx, h, env.UUID)
	if err != nil {
		tx.Rollback()
		return err
	}

	imageSlug := r.PostFormValue("image")
	templateSlug := r.PostFormValue("template")
	snapshotIDStr := r.PostFormValue("snapshot_id")
	skipPull, _ := strconv.ParseBool(r.PostFormValue("skip_pull"))

	repository := "" // The container will be created using this Docker image.
	containerCfg := &freighterclient.ContainerConfig{}

	switch {
	case imageSlug != "":
		repository = imageSlug

	case templateSlug != "":
		var err error
		repository, containerCfg, err = fromTemplate(tx, templateSlug)
		if err != nil {
			tx.Rollback()
			return err
		}

	case snapshotIDStr != "":
		var err error
		repository, err = fromSnapshot(tx, env.User, snapshotIDStr)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Login to private Docker registry so that we'll be authenticated when
		// pulling the snapshot later.
		if err := loginToPrivateRegistry(tx, env.User, cl); err != nil {
			tx.Rollback()
			return err
		}
	}

	hostVols, err := hostVolumes(tx, h)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := prepareConfig(containerCfg, r.PostForm, hostVols); err != nil {
		tx.Rollback()
		return err
	}

	var containerSequence int
	if err := tx.QueryRow("SELECT NEXTVAL('containers_slug_seq')").Scan(&containerSequence); err != nil {
		logrus.Errorf("Failed to get next value of container slug sequence")
		tx.Rollback()
		return err
	}
	name := r.PostFormValue("name")
	slug := slugbuilder.BuildWithNumber(name, containerSequence)
	cn := &container.Container{
		UserID: env.User.ID,
		HostID: h.ID,
		Slug:   slug,
		Name:   sql.NullString{name, true},
		Image:  repository,
	}
	if err := cn.Validate(); err != nil {
		tx.Rollback()
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}

	// Save record to db first to reserve slug (since it's globally unique).
	if err := cn.Insert(tx); err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	// Pull image, then create container.
	go func() {
		defer tx.Rollback()

		topic := env.User.Topic()
		publishFailure := func(err error) {
			env.Events.Publish(
				controllers.NewEvent(controllers.EventContainerCreateFailed, map[string]interface{}{
					"request_id": env.UUID,
					"name":       name,
					"host_slug":  h.Slug,
					"error":      err.Error(),
				}),
				topic,
			)
		}

		env.Events.Publish(
			controllers.NewEvent(controllers.EventContainerCreating, map[string]interface{}{
				"request_id": env.UUID,
				"name":       name,
				"host_slug":  h.Slug,
			}),
			topic,
		)

		if !skipPull {
			updateImage := true
			if err := cl.PullImage(repository, updateImage, ioutil.Discard); err != nil {
				publishFailure(err)
				return
			}
		}

		// Create container.
		created, err := cl.Create(slug, repository, cn.Secret, containerCfg)
		if err != nil {
			switch err.(type) {
			case *freighterclient.StartFailedError:
				// A start failure is fine so we don't delete the container record.
			default:
				if err == freighterclient.ErrMissingCmd {
					publishFailure(errMissingCmd)
					return
				}
				if err == freighterclient.ErrImageNotFound {
					publishFailure(errImageNotFound)
					return
				}
				publishFailure(err)
				return
			}
		}

		// Update existing record with new info from Freighter.
		cn.CID = created.ID
		cn.IPAddress = created.Host
		cn.GatewayIP = created.Gateway
		cn.Cmd = created.Cmd
		cn.Entrypoint = created.Entrypoint
		cn.ExitCode = created.ExitCode
		cn.StartedAt = timePtr(created.StartedAt)
		cn.FinishedAt = timePtr(created.FinishedAt)

		// Update port mappings
		if portMappings, err := GetPortMappingsJsonFrom(created); err != nil {
			logrus.Errorf("containers.create: failed to get port mappings: %v", err)
		} else {
			cn.PortMappings = portMappings
		}

		if ok := cn.SetState(created.State); !ok { // log and continue
			logrus.Errorf("containers.create: freighter returned container with unexpected state: %v", created.State)
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
				vol, err := volume.Find(tx, cn.HostID, vslug)
				if err != nil {
					if err != sql.ErrNoRows {
						publishFailure(fmt.Errorf("failed to retrieve volume %v on host %d: %v", vol, cn.HostID, err))
						return
					}

					// Create a new volume definition if it doesn't already exist.
					vol = &volume.Volume{
						HostID: cn.HostID,
						Slug:   vslug,
						Name:   fmt.Sprintf("%s:%s", cn.Slug, vname),
					}
					if err := vol.Insert(tx); err != nil {
						publishFailure(fmt.Errorf("failed to insert volume %v: %v", vol, err))
						return
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
		}

		if err := tx.Commit(); err != nil {
			publishFailure(err)
			return
		}

		PublishContainerEvent(env, controllers.EventContainerCreated, cn, map[string]interface{}{
			"request_id": env.UUID,
		})

		updatePublicKeyAndAddToGitHub(env, h, cn, cl)
	}()

	env.Status = http.StatusAccepted
	env.Data = map[string]interface{}{
		"request_id": env.UUID, // Unique identifier for this container creation request.
	}

	return nil
}

func validateCreateAtomicForm(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	if r.PostFormValue("name") == "" {
		return errNameRequired
	}
	if r.PostFormValue("image") == "" &&
		r.PostFormValue("template") == "" &&
		r.PostFormValue("snapshot_id") == "" {
		return errImageParamsRequired
	}
	return nil
}

func fromTemplate(d ndb.QueryReceiver, templateSlug string) (string, *freighterclient.ContainerConfig, error) {
	tmpl, err := template.Find(d, templateSlug)
	if err != nil {
		return "", nil, err
	}

	// Use container config from template.
	tmplCfg, err := tmpl.ParsedConfig()
	if err != nil {
		return "", nil, err
	}
	containerCfg := &freighterclient.ContainerConfig{
		Privileged: &(tmplCfg.Privileged),
		PublishAll: &(tmplCfg.PublishAll),
	}
	for _, e := range tmplCfg.Env {
		containerCfg.Env = append(containerCfg.Env, e)
	}
	for _, p := range tmplCfg.Ports {
		containerCfg.PortBindings = append(containerCfg.PortBindings, p)
	}

	return tmpl.Image, containerCfg, nil
}

func fromSnapshot(d ndb.QueryReceiver, u *user.User, snapshotIDStr string) (string, error) {
	snapshotID, err := strconv.ParseInt(snapshotIDStr, 10, 64)
	if err != nil {
		return "", errInvalidSnapshot
	}
	snap, err := snapshot.FindByID(d, snapshotID)
	if err != nil {
		return "", errInvalidSnapshot
	}
	if !snap.OrgID.Valid {
		// For now, we require snapshots to belong to an org.
		// This might change when we want to allow personal accounts to
		// create snapshots.
		return "", errSnapshotNoOrg
	}
	o, err := org.Find(d, snap.OrgID.Int64)
	if err != nil {
		return "", err
	}

	// Check that user is a member of org, with a valid role.
	membership, err := orgmembership.FindByUsernameAndOrgSlug(d, u.Username, o.Slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", errNotOrgMember
		}
		return "", err
	}
	if !membership.SnapshotReadAllowed() {
		return "", errReadSnapshotNotAllowed
	}

	return fmt.Sprintf("%s/%s", envvars.DockerRegistryHostname, snap.Repository()), err
}

func loginToPrivateRegistry(d ndb.QueryReceiver, u *user.User, cl freighterclient.Client) error {
	// Find Docker registry token to use as the password for logging in to the
	// private Docker registry. If none exists, we create one.
	tok, err := token.FindByType(d, u.ID, token.TypeDockerRegistry)
	if err != nil {
		if err == sql.ErrNoRows {
			tok = &token.Token{
				UserID: u.ID,
				Type:   token.TypeDockerRegistry,
			}
			if err := tok.Insert(d); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return cl.RegistryLogin(&freighterclient.RegistryAuth{
		Username: u.Username,
		Password: tok.Token,
		Email:    u.Email,
		Registry: envvars.DockerRegistryHostname,
	})
}
