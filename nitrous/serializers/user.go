package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

type User struct {
	ID             int64           `json:"id"`
	Username       string          `json:"username"`
	Email          string          `json:"email"`
	Admin          bool            `json:"admin"`
	UserProfile    *UserProfile    `json:"user_profile,omitempty"`
	BillingProfile *BillingProfile `json:"billing_profile,omitempty"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeUser(u *user.User) *User {
	if u == nil {
		return nil
	}
	return &User{
		ID:       u.ID,
		Username: u.Username,
		Email:    u.Email,
		Admin:    u.Admin,

		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
