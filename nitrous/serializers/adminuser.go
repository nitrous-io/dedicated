package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
)

type AdminUser struct {
	User

	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	BannedAt  *time.Time `json:"banned_at,omitempty"`
	BannedBy  string     `json:"banned_by,omitempty"`
}

func AdminSerializeUserWithProfiles(u *user.User, up *userprofile.UserProfile, bp *billingprofile.BillingProfile) *AdminUser {
	serU := SerializeUser(u)
	if serU == nil {
		return nil
	}
	serU.UserProfile = SerializeUserProfile(up)
	serU.BillingProfile = SerializeBillingProfile(bp)

	serialized := &AdminUser{}
	serialized.User = *serU
	if u.DeletedAt.Valid {
		serialized.DeletedAt = &u.DeletedAt.Time
	}
	if u.BannedAt.Valid {
		serialized.BannedAt = &u.BannedAt.Time
	}

	return serialized
}
