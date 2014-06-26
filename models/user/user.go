package user

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type User struct {
	ID                int32      `db:"id"`
	Username          string     `db:"username"`
	Email             string     `db:"email"`
	EncryptedPassword string     `db:"encrypted_password"`
	Name              string     `db:"name"`
	CreatedAt         *time.Time `db:"created_at"`
	UpdatedAt         *time.Time `db:"updated_at"`
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
