package users

import (
	"net/http"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/user"
)

func Create(c controllers.Helper, w http.ResponseWriter, r *http.Request) {
	db := c.DB()

	email := r.FormValue("email")
	username := r.FormValue("username")
	password := r.FormValue("password")
	name := r.FormValue("name")

	if email == "" {
		c.Respond(w, 422, []byte(`{"error":"validation_failed","error_description":"email is required"}`))
		return
	}

	if username == "" {
		c.Respond(w, 422, []byte(`{"error":"validation_failed","error_description":"username is required"}`))
		return
	}

	if password == "" {
		c.Respond(w, 422, []byte(`{"error":"validation_failed","error_description":"password is required"}`))
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	u := &user.User{
		Email:    email,
		Username: username,
		Name:     name,
		Password: password,
	}

	err = u.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			switch e.Constraint {
			case "index_users_on_username":
				c.Respond(w, 422, []byte(`{"error":"validation_failed","error_description":"username is taken"}`))
				return
			case "index_users_on_email":
				c.Respond(w, 422, []byte(`{"error":"validation_failed","error_description":"email is taken"}`))
				return
			}
		}
		c.RespondWithError(w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.Respond(w, http.StatusOK, []byte(`{"status":"ok"}`))
}

func GetAuthenticatedUser(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) {
	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"username":   u.Username,
			"email":      u.Email,
			"created_at": u.CreatedAt,
			"updated_at": u.UpdatedAt,
		},
	})
}
