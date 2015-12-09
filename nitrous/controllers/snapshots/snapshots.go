package snapshots

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
)

var (
	errOrgNotFound                  = httperror.New(http.StatusNotFound, "not_found", "org does not exist")
	errNotOrgMember                 = httperror.New(http.StatusForbidden, "forbidden", "you are not a member of this org")
	errPublishSnapshotNotAllowed    = httperror.New(http.StatusForbidden, "forbidden", "you are not allowed to publish snapshots")
	errHostNotFound                 = httperror.New(http.StatusNotFound, "not_found", "workstation not found")
	errHostNotInOrg                 = httperror.New(http.StatusBadRequest, "bad_request", "workstation does not belong to this org")
	errSnapshotNotFound             = httperror.New(http.StatusNotFound, "not_found", "snapshot not found")
	errSnapshotAlreadyPublished     = httperror.New(http.StatusConflict, "conflict", "snapshot already published")
	errSnapshotPublishingInProgress = httperror.New(http.StatusConflict, "conflict", "snapshot is currently being published")
	errRegistryUnreachable          = httperror.New(http.StatusServiceUnavailable, "unavailable", "the snapshot service is temporarily unavailable")
)

func ForOrg(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	orgSlug := mux.Vars(r)["org_slug"]
	o, err := org.FindBySlug(db, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errOrgNotFound
		}
		return err
	}

	_, err = orgmembership.FindByUsernameAndOrgSlug(db, env.User.Username, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotOrgMember
		}
		return err
	}

	snapshots, err := snapshot.FindAllByOrgID(db, env.User.ID, o.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"snapshots": serializers.SerializeSnapshotsWithOwner(snapshots),
	}

	return nil
}

func Publish(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	orgSlug := mux.Vars(r)["org_slug"]
	o, err := org.FindBySlug(db, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errOrgNotFound
		}
		return err
	}

	// TODO Refactor "look up org, then look up orgmembership" to middleware.
	membership, err := orgmembership.FindByUsernameAndOrgSlug(db, env.User.Username, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotOrgMember
		}
		return err
	}
	if !membership.SnapshotPublishAllowed() {
		return errPublishSnapshotNotAllowed
	}

	h, err := host.Find(db, env.User, r.PostFormValue("host_id"))
	if err != nil {
		if err == sql.ErrNoRows {
			return errHostNotFound
		}
		return err
	}

	// Make sure that the host belongs to the org.
	if h.OrgID.Int64 != o.ID {
		return errHostNotInOrg
	}

	img, err := image.Find(db, env.User, h, r.PostFormValue("image_id"))
	if err != nil {
		if err == sql.ErrNoRows {
			return errSnapshotNotFound
		}
		return err
	}

	// Check whether image has already been published as a snapshot.
	if _, err := snapshot.FindByImageID(db, img.ID); err == nil {
		return errSnapshotAlreadyPublished
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	// Don't allow image to be published concurrently.
	gotLock, err := ndb.TransactionLock(tx, ndb.IMAGE_PUBLISH_LOCK, int(img.ID))
	if err != nil {
		tx.Rollback()
		return err
	}
	if !gotLock {
		tx.Rollback()
		return errSnapshotPublishingInProgress
	}

	// Find Docker registry token to use as the password for logging in to the
	// private Docker registry. If none exists, we create one.
	tok, err := token.FindByType(tx, env.User.ID, token.TypeDockerRegistry)
	if err != nil {
		if err == sql.ErrNoRows {
			tok = &token.Token{
				UserID: env.User.ID,
				Type:   token.TypeDockerRegistry,
			}
			if err := tok.Insert(env.DB); err != nil {
				tx.Rollback()
				return err
			}
		} else {
			tx.Rollback()
			return err
		}
	}

	cl, err := controllers.NewFreighterClient(tx, h, env.UUID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := cl.RegistryLogin(&freighterclient.RegistryAuth{
		Username: env.User.Username,
		Password: tok.Token,
		Email:    env.User.Email,
		Registry: envvars.DockerRegistryHostname,
	}); err != nil {
		tx.Rollback()
		if err == freighterclient.ErrInvalidParams {
			logrus.Errorf("unexpected failure logging into Nitrous private Docker registry %q for user %s, err: %v",
				envvars.DockerRegistryHostname, env.User.Username, err)
			// Indicate that registry is unreachable because we unexpectedly
			// cannot login to it.
			return errRegistryUnreachable
		}
		return err
	}

	// Snapshot repository name should be of the form:
	//   <registry hostname>/<org slug>/<slug>:latest
	snapshotName := slugbuilder.BuildWithNumber(img.Slug, int(img.ID))
	snapshotSlug := fmt.Sprintf("%s/%s", o.Slug, snapshotName)
	snapshotRepo := fmt.Sprintf("%s/%s:latest", envvars.DockerRegistryHostname, snapshotSlug)

	// Push snapshot to Docker registry.
	go func() {
		defer tx.Rollback()

		topic := env.User.Topic()
		publishFailure := func(err error) {
			env.Events.Publish(
				controllers.NewEvent(controllers.EventSnapshotPublishFailed, map[string]interface{}{
					"org_slug": o.Slug,
					"host_id":  h.ID,
					"image_id": img.ID,
					"error":    err.Error(),
				}),
				topic,
			)
		}

		env.Events.Publish(
			controllers.NewEvent(controllers.EventSnapshotPublishing, map[string]interface{}{
				"org_slug": o.Slug,
				"host_id":  h.ID,
				"image_id": img.ID,
			}),
			topic,
		)

		if err := cl.PushImage(img.FreighterID, snapshotRepo, ioutil.Discard); err != nil {
			publishFailure(err)
			return
		}

		// Create snapshot record.
		s := &snapshot.Snapshot{
			ImageID:       img.ID,
			UserID:        env.User.ID,
			OrgID:         sql.NullInt64{o.ID, true},
			Slug:          snapshotSlug,
			Namespace:     o.Slug,
			Name:          snapshotName,
			Tag:           "latest",
			DisplayName:   img.Name,
			OriginalImage: img.Slug, // FIXME This should be the image repo of the container.
		}
		if err := s.Insert(tx); err != nil {
			publishFailure(err)
			return
		}

		swo, err := snapshot.FindByImageID(tx, s.ImageID)
		if err != nil {
			publishFailure(err)
			return
		}

		if err := tx.Commit(); err != nil {
			publishFailure(err)
			return
		}

		env.Events.Publish(
			controllers.NewEvent(controllers.EventSnapshotPublished, map[string]interface{}{
				"org_slug": o.Slug,
				"host_id":  h.ID,
				"image_id": img.ID,
				"snapshot": serializers.SerializeSnapshotWithOwner(swo),
			}),
			topic,
		)
	}()

	env.SetEmptyResponseBody()

	return nil
}
