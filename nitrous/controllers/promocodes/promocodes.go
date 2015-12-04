package promocodes

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var errNotFound = httperror.New(http.StatusNotFound, "invalid_request", "could not find valid promo code with a given name")

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	code := mux.Vars(r)["code"]

	c, err := credittype.FindValidCreditTypeByName(db, code)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	var expiresAt *time.Time
	if c.ExpiresAt.Valid {
		expiresAt = &c.ExpiresAt.Time
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"credit_amount": c.DefaultValue,
		"expires_at":    expiresAt,
	}
	return nil
}

func ShowSubscriptionPromocode(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	slug := mux.Vars(r)["slug"]

	pc, err := promocode.Find(db, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"promo_code": serializers.SerializePromoCode(pc),
	}
	return nil
}

func PendingPromoCodes(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	pcs, err := promocode.FindAllPending(db, env.User.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"promo_codes": serializers.SerializePromoCodes(pcs),
	}
	return nil
}
