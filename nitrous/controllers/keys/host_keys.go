package keys

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey/hkey"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func HostIndex(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	keys, err := hkey.FindAll(env.DB, env.User.ID, h.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"keys": serializers.SerializePublicKeys(keys),
	}
	return nil
}

func HostCreate(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	pk := &publickey.PublicKey{
		UserID: env.User.ID,
		HostID: sql.NullInt64{h.ID, true},
		Key:    r.FormValue("key"),
	}

	if err := pk.Validate(); err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	if err := hkey.Insert(tx, pk); err != nil {
		tx.Rollback()
		if err == publickey.ErrInvalidKey {
			return ErrInvalidKey
		}
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" && e.Constraint == "index_public_keys_on_host_id_and_key" {
			return ErrKeyExists
		}
		return err
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := cl.AddPublicKeys(freighterclient.HostKeyOperation, []string{pk.Key}); err != nil {
		tx.Rollback()
		return err
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

func HostDeleteAll(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}
	if err := cl.ClearPublicKeys(freighterclient.HostKeyOperation); err != nil {
		return err
	}

	if err := hkey.DestroyAll(db, u.ID, h.ID); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func HostDelete(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["kslug"]

	key, err := hkey.Find(db, u.ID, h.ID, slug)
	if err != nil {
		if err == publickey.ErrNotFound {
			return ErrFingerprintNotFound
		}
		return err
	}

	if err := cl.RemovePublicKey(freighterclient.HostKeyOperation, key.Fingerprint); err != nil {
		return err
	}

	if err := hkey.Destroy(db, u.ID, h.ID, key.ID); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}
