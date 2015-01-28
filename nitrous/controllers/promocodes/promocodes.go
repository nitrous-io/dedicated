package promocodes

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"

	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var errNotFound = httperror.New(http.StatusNotFound, "invalid_request", "could not find valid promo code with a given name")

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	code := mux.Vars(r)["code"]

	c, err := credittype.FindValidCreditTypeByName(db, code)
	if err != nil {
		return err
	}

	if c == nil {
		return errNotFound
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"credit_amount": c.DefaultValue,
		"expires_at":    c.ExpiresAt,
	}
	return nil
}
