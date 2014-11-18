package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("user serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &user.User{
			ID:                10,
			Name:              "Emma Watson",                 // Not serialized.
			EncryptedPassword: "!!11such!11!secure!!11wow!!", // Not serialized.
			Password:          "doge4evar",                   // Not serialized.
			Username:          "emmawatson",
			Email:             "h.granger@hogwarts.edu",
			Admin:             true,
			CreatedAt:         &createdAt,
			UpdatedAt:         &updatedAt,
		}
	)

	Describe("SerializeUser", func() {
		It("serializes to JSON", func() {
			u := serializers.SerializeUser(model)
			b, err := json.Marshal(u)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"username": "emmawatson",
				"email": "h.granger@hogwarts.edu",
				"admin": true,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})
})
