package keys

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey/ckey"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	ErrContainerBlank        = httperror.New(http.StatusBadRequest, "validation_failed", "missing container id")
	ErrContainerNotFound     = httperror.New(http.StatusNotFound, "container_not_found", "container does not exist")
	ErrInvalidKey            = httperror.New(http.StatusBadRequest, "validation_failed", "invalid key")
	ErrKeyExists             = httperror.New(http.StatusBadRequest, "validation_failed", "key already exists")
	ErrPubKeyOpsNotSupported = httperror.New(http.StatusBadRequest, "invalid_request", "public key operations not supported on this container")
	ErrFingerprintNotFound   = httperror.New(http.StatusNotFound, "fingerprint_not_found", "fingerprint not found")
)

var (
	ContainerIndex     middleware.Middleware = withContainer(index)
	ContainerCreate    middleware.Middleware = containers.WithContainerAndHost(create)
	ContainerDeleteAll middleware.Middleware = containers.WithContainerAndHost(deleteAll)
	ContainerDelete    middleware.Middleware = containers.WithContainerAndHost(delete)
)

func index(cn *container.Container, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	keys, err := ckey.FindAll(env.DB, env.User.ID, cn.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"keys": serializers.SerializePublicKeys(keys),
	}
	return nil
}

func create(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	pk := &publickey.PublicKey{
		UserID:      env.User.ID,
		ContainerID: sql.NullInt64{cn.ID, true},
		Key:         r.FormValue("key"),
	}

	if err := pk.Validate(); err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	if err := ckey.Insert(tx, pk); err != nil {
		tx.Rollback()
		if err == publickey.ErrInvalidKey {
			return ErrInvalidKey
		}
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" && e.Constraint == "index_public_keys_on_container_id_and_key" {
			return ErrKeyExists
		}
		return err
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := cl.AddPublicKeys(cn.CID, []string{pk.Key}); err != nil {
		tx.Rollback()
		return freighterError(err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
	}
	return nil
}

func deleteAll(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}
	err = cl.ClearPublicKeys(cn.CID)
	if err != nil {
		return freighterError(err)
	}

	err = ckey.DestroyAll(db, u.ID, cn.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func delete(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["kslug"]

	key, err := ckey.Find(db, u.ID, cn.ID, slug)
	if err != nil {
		if err == publickey.ErrNotFound {
			return ErrFingerprintNotFound
		}
		return err
	}

	err = cl.RemovePublicKey(cn.CID, key.Fingerprint)
	if err != nil {
		return freighterError(err)
	} else {
		err = ckey.Destroy(db, u.ID, cn.ID, key.ID)
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

func withContainer(fn func(cn *container.Container, env *middleware.Env, w http.ResponseWriter, r *http.Request) error) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		db := env.DB
		u := env.User

		cn, err := container.Find(db, u, mux.Vars(r)["slug"])
		if err != nil {
			return err
		}
		if cn == nil {
			return ErrContainerNotFound
		}

		return fn(cn, env, w, r)
	}
}

func freighterError(err error) error {
	if err == freighterclient.ErrPubKeyOpsNotSupported {
		return ErrPubKeyOpsNotSupported
	}
	return err
}
