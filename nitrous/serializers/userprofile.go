package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
)

type UserProfile struct {
	UserID                    int64  `json:"user_id"`
	MaxHosts                  int64  `json:"max_hosts"`
	Advanced                  bool   `json:"advanced"`
	AuthorizedGitHubUserName  string `json:"authorized_github_user_name,omitempty"`
	CanDeleteGitHubPublicKeys bool   `json:"can_delete_github_public_keys"`
}

func SerializeUserProfile(p *userprofile.UserProfile) *UserProfile {
	return &UserProfile{
		UserID:                    p.UserID,
		MaxHosts:                  p.MaxHosts,
		Advanced:                  p.Advanced,
		AuthorizedGitHubUserName:  userprofile.AuthorizedGitHubUserName(p),
		CanDeleteGitHubPublicKeys: userprofile.CanDeleteGitHubPublicKeys(p),
	}
}
