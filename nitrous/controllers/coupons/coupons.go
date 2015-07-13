package coupons

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/coupon"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	code := r.PostFormValue("code")
	if code == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "code must be provided")
	}

	cp := coupon.Coupon{
		UserID: env.User.ID,
		Code:   code,
	}
	if err := cp.Insert(env.DB); err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
	}
	return nil
}
