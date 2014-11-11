package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/userprofile"

type UserProfile struct {
	Advanced bool `json:"advanced"`
}

func SerializeUserProfile(p *userprofile.UserProfile) *UserProfile {
	return &UserProfile{
		Advanced: p.Advanced,
	}
}
