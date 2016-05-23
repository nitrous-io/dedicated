package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("userprofile serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &userprofile.UserProfile{
			ID:                   9, // Not serialized.
			UserID:               10,
			MaxHosts:             15,
			Advanced:             false,
			GitHubUserName:       sql.NullString{"foo", true},
			GitHubOAuthTokenJson: sql.NullString{`{"access_token": "token"}`, true},
			GitHubUserScopesJson: sql.NullString{`["user:email","repo","admin:public_key"]`, true},
			GitName:              sql.NullString{"gitfoo", true},
			GitEmail:             sql.NullString{"git@foo.com", true},
			CreatedAt:            &createdAt,
			UpdatedAt:            &updatedAt,
		}
	)

	Describe("SerializeUserProfile", func() {
		It("serializes to JSON", func() {
			p := serializers.SerializeUserProfile(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"user_id": 10,
				"max_hosts": 15,
				"advanced": false,
				"authorized_github_user_name": "foo",
				"can_delete_github_public_keys": true,
				"git_name": "gitfoo",
				"git_email": "git@foo.com"
			}`))
		})
	})
})
