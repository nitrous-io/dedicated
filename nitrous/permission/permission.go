package permission

import (
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func Basic(t *token.Token, u *user.User) bool {
	return t.HasAtLeastScope([]string{token.ScopeFull, token.ScopeThirdParty, token.ScopeAdmin})
}

func Full(t *token.Token, u *user.User) bool {
	return t.HasScope(token.ScopeFull)
}

func Admin(t *token.Token, u *user.User) bool {
	return u.Admin && Full(t, u)
}

func Service(t *token.Token, u *user.User) bool {
	return (u.Admin || u.ServiceAccount) && Full(t, u)
}

// Derivative permissions

func PayInvoice(t *token.Token, u *user.User) bool {
	return Full(t, u)
}

func ProfileSettings(t *token.Token, u *user.User) bool {
	return Full(t, u)
}

func DeleteUser(t *token.Token, u *user.User) bool {
	return Full(t, u)
}

func Login(t *token.Token, u *user.User) bool {
	return u.CanLogin() || t.HasScope(token.ScopeAdmin)
}

func AuthorizeGitHub(t *token.Token, u *user.User) bool {
	return Full(t, u)
}

func AuthorizationGrant(t *token.Token, u *user.User) bool {
	return Full(t, u)
}
