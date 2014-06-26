package server

import (
	"net/http"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
)

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	username := r.FormValue("username")
	password := r.FormValue("password")
	name := r.FormValue("name")

	if email == "" {
		s.respond(w, 422, []byte(`{"error":"email is required"}`))
		return
	}

	if username == "" {
		s.respond(w, 422, []byte(`{"error":"username is required"}`))
		return
	}

	if password == "" {
		s.respond(w, 422, []byte(`{"error":"password is required"}`))
		return
	}

	encryptedPassword, err := bcrypt.Hash(password)
	if err != nil {
		s.logger.Error(err.Error())
		s.respondWith500(w, "password encryption failed")
		return
	}

	tx, err := s.db.Beginx()
	if err != nil {
		s.logger.Error(err.Error())
		s.respondWith500(w, "failed to write to db")
		return
	}
	user := &user.User{
		Email:             email,
		Username:          username,
		EncryptedPassword: encryptedPassword,
		Name:              name,
	}

	_, err = tx.NamedExec("INSERT INTO users (email, username, encrypted_password, name) VALUES (:email, :username, :encrypted_password, :name);", &user)
	if err != nil {
		tx.Rollback()
		s.logger.Error(err.Error())
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			switch e.Constraint {
			case "index_users_on_username":
				s.respond(w, 422, []byte(`{"error":"username is taken"}`))
				return
			case "index_users_on_email":
				s.respond(w, 422, []byte(`{"error":"email is taken"}`))
				return
			}
		}
		s.respondWith500(w, "failed to write to db")
		return
	}

	err = tx.Commit()
	if err != nil {
		s.logger.Error(err.Error())
		s.respondWith500(w, "failed to write to db")
		return
	}

	s.respond(w, http.StatusOK, jsonStatusOK)
}
