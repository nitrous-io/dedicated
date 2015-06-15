package github

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/go-github/github"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/nitrous-common/msgverifier"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

// Give user 5 minutes time to authorize at GitHub
const ValidityPeriod = 5 * time.Minute
const ValidityPeriodSeconds = int(ValidityPeriod / time.Second)

var (
	oauthConf = &oauth2.Config{
		ClientID:     envvars.GitHubOAuthClientID,
		ClientSecret: envvars.GitHubOAuthClientSecret,
		Scopes:       []string{"user:email", "repo", "write:public_key"},
		Endpoint:     githuboauth.Endpoint,
	}

	errUpdateFailed      = httperror.New(http.StatusInternalServerError, "update failed")
	errInvalidOauthState = httperror.New(http.StatusBadRequest, "invalid oauth state")
	errStateExpired      = httperror.New(http.StatusBadRequest, "GitHub authorization timeout")
	errInvalidUserID     = httperror.New(http.StatusBadRequest, "cannot assign GitHub credentials to another user")
	errStateMismatch     = httperror.New(http.StatusBadRequest, "state provided does not match cookie state")

	ErrTokenEmpty    = errors.New("token is empty")
	ErrTokenNotValid = errors.New("token not valid")

	verifier = msgverifier.New(envvars.SecretToken)
)

type OAuthState struct {
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
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

func MakeOauthState(userID int64) (string, error) {
	createdAt := time.Now().UTC()
	return verifier.Generate(&OAuthState{
		UserID:    userID,
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(ValidityPeriod),
	})
}

func ReadOAuthState(signedState string) (*OAuthState, error) {
	state := &OAuthState{}
	err := verifier.Verify(signedState, state)
	if err != nil {
		return nil, err
	}

	if state.ExpiresAt.Before(time.Now()) {
		return nil, errStateExpired
	}
	return state, nil
}

// /api/v0/github/authorize
// Authorize returns GitHub authorization url that front-end needs to redirect to
func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	state, err := MakeOauthState(env.User.ID)
	if err != nil {
		return err
	}

	// Set state into cookie to prevent CSRF attack
	http.SetCookie(w, &http.Cookie{
		Name:   "github_state",
		Value:  state,
		MaxAge: ValidityPeriodSeconds,
	})

	url := oauthConf.AuthCodeURL(state, oauth2.AccessTypeOnline)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"redirect_url": url,
	}
	return nil
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

	signedState := r.FormValue("state")
	state, err := ReadOAuthState(signedState)
	if err != nil {
		return err
	}

	// State must match previously set cookie value
	// to prevent call being made by victim's user agent
	cookie, err := r.Cookie("github_state")
	if err != nil || cookie.Value != signedState {
		return errStateMismatch
	}

	userID := state.UserID
	p, err := userprofile.Find(env.DB, userID)
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

	// Erase state cookie to prevent repeated requests
	http.SetCookie(w, &http.Cookie{
		Name:   "github_state",
		Value:  "",
		MaxAge: -1,
	})

	env.Tracker.SetProfileProps("set", userID, map[string]interface{}{
		"github_username": gitHubUserName,
	}, env.Logger)
	env.Tracker.SetProfileProps("setnx", userID, map[string]interface{}{
		"github_authorized_date": time.Now(),
	}, env.Logger)

	http.Redirect(w, r, "/app#/account", http.StatusFound)
	return nil
}
