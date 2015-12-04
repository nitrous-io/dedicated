package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
)

type User struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	Username         string          `json:"username"`
	Email            string          `json:"email"`
	Admin            bool            `json:"admin"`
	UserProfile      *UserProfile    `json:"user_profile,omitempty"`
	BillingProfile   *BillingProfile `json:"billing_profile,omitempty"`
	UnconfirmedEmail string          `json:"unconfirmed_email,omitempty"`
	FirstTime        bool            `json:"first_time"`
	Phone            string          `json:"phone"`
	PhoneCountryCode string          `json:"phone_country_code"`
	PhoneAuthed      bool            `json:"phone_authed"`

	MaxContainers    int64 `json:"max_containers,omitempty"`
	CanUseDesktopApp *bool `json:"can_use_desktop_app,omitempty"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeUser(u *user.User) *User {
	if u == nil {
		return nil
	}
	s := &User{
		ID:               u.ID,
		Name:             u.Name,
		Username:         u.Username,
		Email:            u.Email,
		Admin:            u.Admin,
		UnconfirmedEmail: u.UnconfirmedEmail.String,
		FirstTime:        u.FirstTime,
		Phone:            u.Phone.String,
		PhoneCountryCode: u.PhoneCountryCode.String,
		PhoneAuthed:      u.PhoneAuthed,
		MaxContainers:    u.MaxContainers.Int64,

		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}

	if u.CanUseDesktopApp.Valid {
		s.CanUseDesktopApp = &u.CanUseDesktopApp.Bool
	}

	return s
}

func SerializeUserWithProfiles(u *user.User, up *userprofile.UserProfile, bp *billingprofile.BillingProfile) *User {
	serialized := SerializeUser(u)
	if serialized == nil {
		return nil
	}
	serialized.UserProfile = SerializeUserProfile(up)
	serialized.BillingProfile = SerializeBillingProfile(bp)
	return serialized
}
