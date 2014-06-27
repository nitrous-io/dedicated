package user

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
)

type User struct {
	ID                int64      `db:"id"`
	Username          string     `db:"username"`
	Email             string     `db:"email"`
	EncryptedPassword string     `db:"encrypted_password"`
	Name              string     `db:"name"`
	CreatedAt         *time.Time `db:"created_at"`
	UpdatedAt         *time.Time `db:"updated_at"`

	Password string
}

func FindById(d db.QueryReceiver, id int64) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func FindByUsername(d db.QueryReceiver, username string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE username = $1", username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (u *User) Insert(d db.QueryReceiver) error {
	if u.Password != "" {
		encrypted, err := bcrypt.Hash(u.Password)
		if err != nil {
			return err
		}
		u.EncryptedPassword = encrypted
		u.Password = ""
	}
	_, err := d.NamedExec("INSERT INTO users (email, username, encrypted_password, name) VALUES (:email, :username, :encrypted_password, :name);", u)
	if err != nil {
		return err
	}
	return d.Get(u, "SELECT * FROM users WHERE username = $1;", u.Username)
}
