package permission_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "permission")
}

var _ = Describe("permission", func() {
	It("should have permission value same as in test table", func() {
		fullToken := &token.Token{Scope: token.ScopeFull}
		thirdPartyToken := &token.Token{Scope: token.ScopeThirdParty}
		adminToken := &token.Token{Scope: token.ScopeAdmin}
		multiScopeToken := &token.Token{Scope: "full container"}
		emptyToken := &token.Token{}
		invalidToken := &token.Token{Scope: "invalid"}

		normalUser := &user.User{}
		adminUser := &user.User{Admin: true}
		serviceUser := &user.User{ServiceAccount: true}

		created := time.Now().Add(-30 * 24 * time.Hour)
		expiredUser := &user.User{CreatedAt: &created}

		testTable := []*struct {
			permFunc func(*token.Token, *user.User) bool
			u        *user.User
			t        *token.Token
			expected bool
		}{
			{permission.Basic, normalUser, fullToken, true},
			{permission.Basic, adminUser, fullToken, true},
			{permission.Basic, serviceUser, fullToken, true},

			{permission.Basic, normalUser, thirdPartyToken, true},
			{permission.Basic, normalUser, adminToken, true},
			{permission.Basic, normalUser, multiScopeToken, true},

			{permission.Basic, normalUser, emptyToken, false},
			{permission.Basic, normalUser, invalidToken, false},

			{permission.Full, normalUser, fullToken, true},
			{permission.Full, normalUser, multiScopeToken, true},

			// admin switch to user do not have full access
			{permission.Full, normalUser, adminToken, false},

			{permission.Full, normalUser, thirdPartyToken, false},
			{permission.Full, normalUser, emptyToken, false},
			{permission.Full, normalUser, invalidToken, false},

			{permission.Admin, adminUser, fullToken, true},
			{permission.Admin, adminUser, multiScopeToken, true},

			// admin switch to another admin user
			{permission.Admin, adminUser, adminToken, false},

			// admin switch to a normal user
			{permission.Admin, normalUser, adminToken, false},

			{permission.Admin, normalUser, fullToken, false},

			{permission.Service, serviceUser, fullToken, true},
			{permission.Service, adminUser, fullToken, true},

			{permission.Service, serviceUser, adminToken, false},
			{permission.Service, adminUser, adminToken, false},

			{permission.Login, expiredUser, adminToken, true},
			{permission.Login, expiredUser, fullToken, false},
		}

		for _, test := range testTable {
			Expect(test.permFunc(test.t, test.u)).To(Equal(test.expected))
		}
	})
})
