package users

import (
	"errors"
	"net/http"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	validations = map[string]error{
		"index_users_on_username": httperror.New(http.StatusBadRequest, "validation_failed", "username is taken"),
		"index_users_on_email":    httperror.New(http.StatusBadRequest, "validation_failed", "email is taken"),
	}
)

func Create(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	email := strings.ToLower(r.FormValue("email"))
	username := strings.ToLower(r.FormValue("username"))
	password := r.FormValue("password")
	name := r.FormValue("name")

	u := &user.User{
		Email:    email,
		Username: username,
		Name:     name,
		Password: password,
	}

	err := u.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = u.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func GetAuthenticatedUser(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	if u == nil {
		return errors.New("user is nil")
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": u,
	}
	return nil
}
