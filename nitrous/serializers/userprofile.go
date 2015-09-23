package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
)

type UserProfile struct {
	MaxHosts                  int64  `json:"max_hosts"`
	Advanced                  bool   `json:"advanced"`
	AuthorizedGitHubUserName  string `json:"authorized_github_user_name,omitempty"`
	CanDeleteGitHubPublicKeys bool   `json:"can_delete_github_public_keys"`
}

func SerializeUserProfile(p *userprofile.UserProfile) *UserProfile {
	return &UserProfile{
		MaxHosts:                  p.MaxHosts,
		Advanced:                  p.Advanced,
		AuthorizedGitHubUserName:  github.AuthorizedGitHubUserName(p),
		CanDeleteGitHubPublicKeys: github.CanDeleteGitHubPublicKeys(p),
	}
}
