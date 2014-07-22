package keys

import (
	"errors"
	"net/http"

	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/publickey"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrContainerBlank        = httperror.New(http.StatusBadRequest, "validation_failed", "missing container id")
	ErrContainerNotFound     = httperror.New(http.StatusBadRequest, "validation_failed", "container not found")
	ErrInvalidKey            = httperror.New(http.StatusBadRequest, "validation_failed", "invalid key")
	ErrKeyExists             = httperror.New(http.StatusBadRequest, "validation_failed", "key already exists")
	ErrPubKeyOpsNotSupported = httperror.New(http.StatusBadRequest, "invalid_request", "public key operations not supported on this container")
	ErrFingerprintNotFound   = httperror.New(http.StatusNotFound, "validation_failed", "fingerprint not found")
	ErrHostIPUnknown         = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
)

var (
	Index               middlewares.Middleware = withContainer(index)
	Create              middlewares.Middleware = withContainer(create)
	DeleteAll           middlewares.Middleware = withContainer(deleteAll)
	DeleteByFingerprint middlewares.Middleware = withContainer(deleteByFingerprint)
)

// FreighterClient is a shared freighterclient.Client for making requests to Freighter
// API hosts.
var FreighterClient freighterclient.Client

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func index(cnr *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	keys := []publickey.PublicKey{}
	keys, err := publickey.FindAllByUserIDAndContainerID(db, u.ID, cnr.ID)

	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
		"keys":   keys,
	}
	return nil
}

func create(cnr *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	pk := publickey.PublicKey{
		UserID:      u.ID,
		ContainerID: cnr.ID,
		Key:         r.FormValue("keytext"),
	}

	err := pk.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = pk.Insert(tx)
	if err != nil {
		tx.Rollback()
		if err == publickey.ErrInvalidKey {
			return ErrInvalidKey
		}
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" && e.Constraint == "index_public_keys_on_container_id_and_key" {
			return ErrKeyExists
		}
		return err
	} else {
		fHost, err := findAndMakeFreighterHost(db, cnr.HostID)
		if err != nil {
			return err
		}
		err = FreighterClient.AddPublicKeys(fHost, cnr.CID, []string{pk.Key})
		if err != nil {
			tx.Rollback()
			return freighterError(err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
	}
	return nil
}

func deleteAll(cnr *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	fHost, err := findAndMakeFreighterHost(db, cnr.HostID)
	if err != nil {
		return err
	}
	err = FreighterClient.ClearPublicKeys(fHost, cnr.CID)
	if err != nil {
		return freighterError(err)
	}

	err = publickey.DestroyAllByUserIDAndContainerID(db, u.ID, cnr.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func deleteByFingerprint(cnr *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	fingerprint := env.Vars["md5_fingerprint"]

	_, err := publickey.FindByUserIDAndContainerIDAndFingerprint(db, u.ID, cnr.ID, fingerprint)
	if err != nil {
		if err == publickey.ErrNotFound {
			return ErrFingerprintNotFound
		}
		return err
	}

	fHost, err := findAndMakeFreighterHost(db, cnr.HostID)
	if err != nil {
		return err
	}
	err = FreighterClient.RemovePublicKey(fHost, cnr.CID, fingerprint)
	if err != nil {
		return freighterError(err)
	} else {
		err = publickey.DestroyByUserIDAndContainerIDAndFingerprint(db, u.ID, cnr.ID, fingerprint)
		if err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func withContainer(fn func(cnr *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return middlewares.Vars(func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		u := env.User
		if u == nil {
			return errors.New("user is nil")
		}

		cSlug := env.Vars["container"]
		cnr, err := container.FindByUserAndSlug(db, u, cSlug)
		if err != nil {
			return err
		}
		if cnr == nil {
			return ErrContainerNotFound
		}

		return fn(cnr, env, c, w, r)
	})
}

func getContainerID(d db.QueryReceiver, r *http.Request, userID int64, optional bool) (int64, error) {
	cid := r.FormValue("container_id")
	if cid == "" {
		if optional {
			return 1, nil
		} else {
			return 1, ErrContainerBlank
		}
	}
	c := &container.Container{}
	if c == nil || c.UserID != userID {
		return 1, ErrContainerNotFound
	}
	return c.ID, nil
}

func findAndMakeFreighterHost(db *db.Database, hostID int64) (*freighterclient.Host, error) {
	h, err := host.FindByID(db, hostID)
	if err != nil {
		return nil, err
	}
	if h.IPAddress.String == "" {
		return nil, ErrHostIPUnknown
	}
	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
}

func freighterError(err error) error {
	if err == freighterclient.ErrPubKeyOpsNotSupported {
		return ErrPubKeyOpsNotSupported
	}
	return err
}
