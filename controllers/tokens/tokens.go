package tokens

import (
	"net/http"

	"github.com/nitrous-io/nitrous/models/token"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
)

func Post(c controllers.Helper, w http.ResponseWriter, r *http.Request) {
	db := c.DB()
	logger := c.Logger()

	grantType := r.FormValue("grant_type")
	username := r.FormValue("username")
	password := r.FormValue("password")

	if grantType != "password" {
		c.Respond(w, http.StatusBadRequest, []byte(`{"error":"unsupported_grant_type"}`))
		return
	}

	if username == "" {
		c.Respond(w, http.StatusBadRequest, []byte(`{"error":"invalid_request","error_description":"username is required"}`))
		return
	}

	if password == "" {
		c.Respond(w, http.StatusBadRequest, []byte(`{"error":"invalid_request","error_description":"password is required"}`))
		return
	}

	u, err := user.FindByUsername(db, username)
	if err != nil {
		logger.Error(err)
		c.RespondWith500(w, "db_read_failed")
		return
	}
	if u == nil {
		c.Respond(w, http.StatusBadRequest, []byte(`{"error":"invalid_grant","error_description":"incorrect username or password"}`))
		return
	}

	if !bcrypt.Verify(u.EncryptedPassword, password) {
		c.Respond(w, http.StatusBadRequest, []byte(`{"error":"invalid_grant","error_description":"incorrect username or password"}`))
		return
	}

	t := &token.Token{UserID: u.ID}

	err = t.Insert(db)
	if err != nil {
		logger.Error(err)
		c.RespondWith500(w, "db_write_failed")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	c.RespondEncodeJson(w, http.StatusOK, map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "Bearer",
	})
}
