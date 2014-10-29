package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

type User struct {
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Admin     bool       `json:"admin"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeUser(u *user.User) *User {
	if u == nil {
		return nil
	}
	return &User{
		Username:  u.Username,
		Email:     u.Email,
		Admin:     u.Admin,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
