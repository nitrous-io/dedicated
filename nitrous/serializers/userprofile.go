package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/userprofile"

type UserProfile struct {
	MaxHosts int64 `json:"max_hosts"`
	Advanced bool  `json:"advanced"`
}

func SerializeUserProfile(p *userprofile.UserProfile) *UserProfile {
	return &UserProfile{
		MaxHosts: p.MaxHosts,
		Advanced: p.Advanced,
	}
}
