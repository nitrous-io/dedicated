package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
)

type User struct {
	UserProfile    *UserProfile    `json:"user_profile,omitempty"`
	BillingProfile *BillingProfile `json:"billing_profile,omitempty"`

	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	Admin            bool   `json:"admin"`
	UnconfirmedEmail string `json:"unconfirmed_email,omitempty"`
	PromoCode        string `json:"promo_code,omitempty"`
	Phone            string `json:"phone"`
	PhoneCountryCode string `json:"phone_country_code"`
	PhoneAuthed      bool   `json:"phone_authed"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeUser(u *user.User) *User {
	if u == nil {
		return nil
	}
	return &User{
		ID:               u.ID,
		Name:             u.Name,
		Username:         u.Username,
		Email:            u.Email,
		Admin:            u.Admin,
		UnconfirmedEmail: u.UnconfirmedEmail.String,
		PromoCode:        u.PromoCode.String,
		Phone:            u.Phone.String,
		PhoneCountryCode: u.PhoneCountryCode.String,
		PhoneAuthed:      u.PhoneAuthed,

		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
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
