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

func FindByUsername(db *db.Database, username string) (*User, error) {
	user := User{}
	err := db.Get(&user, "SELECT * FROM users WHERE username = $1", username)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &user, nil
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
	q := "INSERT INTO users (email, username, encrypted_password, name) VALUES (:email, :username, :encrypted_password, :name);"
	_, err := d.NamedExec(q, u)
	return err
}
