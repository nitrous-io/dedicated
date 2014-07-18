package keys

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/publickey"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrContainerBlank      = httperror.New(http.StatusBadRequest, "validation_failed", "missing container id")
	ErrContainerNotFound   = httperror.New(http.StatusBadRequest, "validation_failed", "container not found")
	ErrInvalidKey          = httperror.New(http.StatusBadRequest, "validation_failed", "invalid key")
	ErrKeyExists           = httperror.New(http.StatusBadRequest, "validation_failed", "key already exists")
	ErrFingerprintNotFound = httperror.New(http.StatusNotFound, "validation_failed", "fingerprint not found")
)

var (
	Index               middlewares.Middleware = withContainer(index)
	Create              middlewares.Middleware = withContainer(create)
	DeleteAll           middlewares.Middleware = withContainer(deleteAll)
	DeleteByFingerprint middlewares.Middleware = withContainer(deleteByFingerprint)
)

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

	err := publickey.DestroyAllByUserIDAndContainerID(db, u.ID, cnr.ID)
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

	fingerprint := mux.Vars(r)["md5_fingerprint"]

	err := publickey.DestroyByUserIDAndContainerIDAndFingerprint(db, u.ID, cnr.ID, fingerprint)

	if err != nil {
		if err == publickey.ErrNotFound {
			return ErrFingerprintNotFound
		}
		return err
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
		cnr, err := container.FindByUserIDAndSlug(db, u.ID, cSlug)
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
