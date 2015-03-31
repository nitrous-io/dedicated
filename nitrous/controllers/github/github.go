package github

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/go-github/github"
	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

var (
	mu        sync.Mutex
	oauthConf *oauth2.Config
	// random string for oauth2 API calls to protect against CSRF
	oauthStateString = ""

	errInvalidOauthState = errors.New("invalid oauth state")
	errUpdateFailed      = errors.New("update failed")

	ErrTokenEmpty    = errors.New("token is empty")
	ErrTokenNotValid = errors.New("token not valid")
)

func init() {
	var src [8]byte
	_, err := rand.Read(src[:])
	if err != nil {
		log.Fatalf("failed to generate oauth secret %v", err)
	}
	oauthStateString = hex.EncodeToString(src[:])
	oauthConf = &oauth2.Config{
		ClientID:     appconfig.GitHubOAuthClientID,
		ClientSecret: appconfig.GitHubOAuthClientSecret,
		Scopes:       []string{"user:email", "repo", "write:public_key"},
		Endpoint:     githuboauth.Endpoint,
	}
}

func tokenToJSON(token *oauth2.Token) (string, error) {
	if d, err := json.Marshal(token); err != nil {
		return "", err
	} else {
		return string(d), nil
	}
}

func tokenFromJSON(jsonStr string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func getTokenIfValidFromUserProfile(p *userprofile.UserProfile) (*oauth2.Token, error) {
	if p.GitHubOAuthTokenJson.String == "" {
		return nil, ErrTokenEmpty
	}
	token, err := tokenFromJSON(p.GitHubOAuthTokenJson.String)
	if err != nil {
		return nil, err
	}
	// it's valid for token to expire and become invalid
	if !token.Valid() {
		return nil, ErrTokenNotValid
	}
	return token, nil
}

// returns empty string if not authorized
func AuthorizedGitHubUserName(p *userprofile.UserProfile) string {
	userName := p.GitHubUserName.String
	if userName == "" || !p.GitHubUserName.Valid {
		return ""
	}
	token, err := getTokenIfValidFromUserProfile(p)
	if err != nil || !token.Valid() {
		return ""
	}
	return userName
}

func GetTokenIfValid(env *middleware.Env) (*oauth2.Token, error) {
	p, err := userprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return nil, err
	}
	return getTokenIfValidFromUserProfile(p)
}

func MakeOauthSecretFromUserId(userId int64) string {
	return fmt.Sprintf("%s %d", oauthStateString, userId)
}

// /api/v0/github/authorize
// Authorize returns GitHub authorization url that front-end needs to redirect to
func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// hackish but the simplest way to achieve what we want: we embed
	// user id as part of OAuth secret so that we can have it OAuthCb (which
	// is called by github and cannot be authenticated)
	secret := MakeOauthSecretFromUserId(env.User.ID)
	url := oauthConf.AuthCodeURL(secret, oauth2.AccessTypeOnline)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"redirect_url": url,
	}
	return nil
}

// returns -1 on error
func getUserIdFromOauthSecret(state string) int64 {
	var secret string
	var userId int64
	_, err := fmt.Sscanf(state, "%s %d", &secret, &userId)
	if err != nil {
		return -1
	}
	if secret != oauthStateString {
		return -1
	}
	return userId
}

func redirectAppAccountErrMsg(w http.ResponseWriter, r *http.Request, errMsg string) {
	// note: don't url.QueryEscape() errMsg, Redirect will do it
	uri := "/app#/account?errmsg=" + errMsg
	http.Redirect(w, r, uri, http.StatusTemporaryRedirect)
}

func GitHubClientFromToken(token *oauth2.Token) *github.Client {
	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	return github.NewClient(oauthClient)
}

// return gitHubUserName and gitHubTokenJson
var GetGithubUserFunc = func(env *middleware.Env, w http.ResponseWriter, r *http.Request, code string) (string, string, error) {

	token, err := oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		env.Logger.Errorf("oauthConf.Exchange() failed with '%s'\n", err)
		redirectAppAccountErrMsg(w, r, "failed to connect with GitHub")
		return "", "", err
	}

	tokenJSON, err := tokenToJSON(token)
	if err != nil {
		redirectAppAccountErrMsg(w, r, "failed to connect with GitHub")
		return "", "", err
	}
	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	client := github.NewClient(oauthClient)

	// get github user name of authenticated user
	user, _, err := client.Users.Get("")
	if err != nil {
		env.Logger.Errorf("client.Users.Get() faled with '%s'\n", err)
		redirectAppAccountErrMsg(w, r, "failed to connect with GitHub")
		return "", "", err
	}
	gitHubUserName := *user.Login
	return gitHubUserName, tokenJSON, nil
}

// /api/v0/github/oauth_cb
// When user grants our app permission to use his GitHub account on GitHub
// website, GitHub redirects the user here
func OAuthCb(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Logger.Debugf("OAuthCb called, url: '%s', rawQuery: '%s'\n", r.URL.RequestURI(), r.URL.RawQuery)

	errLabel := "github.authcb"
	// verify 'state' and extract user id from it
	state := r.FormValue("state")
	userId := getUserIdFromOauthSecret(state)
	if userId == -1 {
		env.Logger.Errorf("invalid oauth state, expected '%s ${userId}', got '%s'\n", oauthStateString, state)
		redirectAppAccountErrMsg(w, r, "failed to connect with GitHub")
		return mh.Error(errInvalidOauthState, errLabel)
	}

	p, err := userprofile.Find(env.DB, userId)
	if err != nil {
		return mh.Error(err, errLabel)
	}
	code := r.FormValue("code")
	if code == "" {
		redirectAppAccountErrMsg(w, r, "failed to connect with GitHub")
		return mh.Error(errInvalidOauthState, errLabel)
	}
	gitHubUserName, tokenJSON, err := GetGithubUserFunc(env, w, r, code)
	if err != nil {
		return mh.Error(err, errLabel)
	}

	env.Logger.Debugf("github token: '%s'\n", tokenJSON)
	env.Logger.Debugf("github user: %s\n", gitHubUserName)
	if p.GitHubUserName.Valid {
		if gitHubUserName != p.GitHubUserName.String {
			// shouldn't happen, we log but ignore
			env.Logger.Errorf("inconsistent github user names, old: %s, new: %s", p.GitHubUserName.String, gitHubUserName)
		}
	}

	p.GitHubUserName = sql.NullString{gitHubUserName, true}
	p.GitHubOAuthTokenJson = sql.NullString{tokenJSON, true}
	if err = p.UpdateGithubInfo(env.DB); err != nil {
		env.Logger.Errorf("p.UpdateGithubInfo() faled with '%s'\n", err)
		redirectAppAccountErrMsg(w, r, "failed to connect with GitHub")
		return mh.Error(errUpdateFailed, errLabel)
	}
	http.Redirect(w, r, "/app#/account", http.StatusTemporaryRedirect)
	return nil
}
