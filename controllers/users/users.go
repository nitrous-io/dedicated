package users

import (
	"net/http"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/user"
)

func Create(c controllers.Helper, w http.ResponseWriter, r *http.Request) {
	db := c.DB()
	logger := c.Logger()

	email := r.FormValue("email")
	username := r.FormValue("username")
	password := r.FormValue("password")
	name := r.FormValue("name")

	if email == "" {
		c.Respond(w, 422, []byte(`{"error":"email is required"}`))
		return
	}

	if username == "" {
		c.Respond(w, 422, []byte(`{"error":"username is required"}`))
		return
	}

	if password == "" {
		c.Respond(w, 422, []byte(`{"error":"password is required"}`))
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		logger.Error(err.Error())
		c.RespondWith500(w, "failed to write to db")
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
		logger.Error(err.Error())
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			switch e.Constraint {
			case "index_users_on_username":
				c.Respond(w, 422, []byte(`{"error":"username is taken"}`))
				return
			case "index_users_on_email":
				c.Respond(w, 422, []byte(`{"error":"email is taken"}`))
				return
			}
		}
		c.RespondWith500(w, "failed to write to db")
		return
	}

	err = tx.Commit()
	if err != nil {
		logger.Error(err.Error())
		c.RespondWith500(w, "failed to write to db")
		return
	}

	c.Respond(w, http.StatusOK, []byte(`{"status":"ok"}`))
}

func GetAuthenticatedUser(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) {
	c.RespondEncodeJson(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"username":   u.Username,
			"email":      u.Email,
			"created_at": u.CreatedAt,
			"updated_at": u.UpdatedAt,
		},
	})
}
