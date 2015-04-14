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
	PromoCode        string          `json:"promo_code,omitempty"`
	FirstTime        bool            `json:"first_time"`

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
		FirstTime:        u.FirstTime,

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
