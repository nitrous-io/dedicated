package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/userprofile"

type UserProfile struct {
	UserID                    int64  `json:"user_id"`
	MaxHosts                  int64  `json:"max_hosts"`
	Advanced                  bool   `json:"advanced"`
	AuthorizedGitHubUserName  string `json:"authorized_github_user_name,omitempty"`
	CanDeleteGitHubPublicKeys bool   `json:"can_delete_github_public_keys"`
	GitName                   string `json:"git_name"`
	GitEmail                  string `json:"git_email"`
}

func SerializeUserProfile(p *userprofile.UserProfile) *UserProfile {
	up := &UserProfile{
		UserID:                    p.UserID,
		MaxHosts:                  p.MaxHosts,
		Advanced:                  p.Advanced,
		AuthorizedGitHubUserName:  p.AuthorizedGitHubUserName(),
		CanDeleteGitHubPublicKeys: p.CanDeleteGitHubPublicKeys(),
	}

	if p.GitName.Valid {
		up.GitName = p.GitName.String
	}

	if p.GitEmail.Valid {
		up.GitEmail = p.GitEmail.String
	}

	return up
}
