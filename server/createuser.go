package server

import (
	"net/http"

	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
)

var jsonErrorInvalidParams = []byte(`{"error":"invalid params"}`)

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	username := r.FormValue("username")
	password := r.FormValue("password")
	name := r.FormValue("name")

	if email == "" || username == "" || password == "" {
		s.respond(w, 422, jsonErrorInvalidParams)
		return
	}

	encryptedPassword, err := bcrypt.Hash(password)
	if err != nil {
		s.respondWith500(w, "password encryption failed")
		return
	}

	tx, err := s.db.Beginx()
	if err != nil {
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
		s.respondWith500(w, "failed to write to db")
		return
	}

	err = tx.Commit()
	if err != nil {
		s.respondWith500(w, "failed to write to db")
		return
	}

	s.respond(w, http.StatusOK, jsonStatusOK)
}
