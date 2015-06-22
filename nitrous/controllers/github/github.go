package github

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
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
	verifier  = msgverifier.New(envvars.SecretToken)
	oauthConf = &oauth2.Config{
		ClientID:     envvars.GitHubOAuthClientID,
		ClientSecret: envvars.GitHubOAuthClientSecret,
		Scopes:       []string{"user:email", "repo", "write:public_key"},
		Endpoint:     githuboauth.Endpoint,
	}

	errUpdateFailed     = httperror.New(http.StatusInternalServerError, "update_failed", "update failed")
	errInvalidOauthCode = httperror.New(http.StatusBadRequest, "invalid_code", "invalid code parameter")
	errInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "invalid state parameter")
	errStateExpired     = httperror.New(http.StatusBadRequest, "auth_timeout", "GitHub authorization timeout")
	errStateMismatch    = httperror.New(http.StatusBadRequest, "state_mismatch", "state provided does not match cookie state")

	ErrTokenEmpty    = errors.New("token is empty")
	ErrTokenNotValid = errors.New("token not valid")

	errAuthorizeForbidden = httperror.New(http.StatusForbidden, "authorize_forbidden",
		"your are not granted sufficient permission to request for GitHub authorization")
)

type OAuthState struct {
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// return gitHubUserName and gitHubTokenJson
var FetchGithubUser = func(code string) (string, string, error) {
	token, err := oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		return "", "", err
	}

	tokenJSON, err := tokenToJSON(token)
	if err != nil {
		return "", "", err
	}

	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	client := github.NewClient(oauthClient)

	// get github user name of authenticated user
	user, _, err := client.Users.Get("")
	if err != nil {
		return "", "", err
	}

	gitHubUserName := *user.Login
	return gitHubUserName, tokenJSON, nil
}

// /api/v0/github/authorize
// Authorize returns GitHub authorization url that front-end needs to redirect to
func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if !env.Token.ProfileSettingsAllowed() {
		return errAuthorizeForbidden
	}

	// FIXME: allow this API to be called only if user is not yet
	// linked with a GitHub account

	state, err := setOrReuseState(w, r, env.User.ID)
	if err != nil {
		return err
	}

	url := oauthConf.AuthCodeURL(state, oauth2.AccessTypeOnline)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"redirect_url": url,
	}
	return nil
}

// /api/v0/github/oauth_cb
// When user grants our app permission to use his GitHub account on GitHub
// website, GitHub redirects the user here
func OAuthCallback(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	tracker := env.Tracker
	errLabel := "github.authcb"

	signedState := r.FormValue("state")
	state, err := readOAuthState(signedState)
	if err != nil {
		return err
	}

	// State must match previously set cookie value
	// to prevent call being made by victim's user agent
	cookie, err := r.Cookie("github_state")
	if err != nil || cookie.Value != signedState {
		return errStateMismatch
	}

	// Erase state cookie to prevent repeated requests
	http.SetCookie(w, &http.Cookie{
		Name:   "github_state",
		Value:  "",
		MaxAge: -1,
	})

	userID := state.UserID
	p, err := userprofile.Find(db, userID)
	if err != nil {
		return mh.Error(err, errLabel)
	}

	code := r.FormValue("code")
	if code == "" {
		redirectError(w, r, "failed to connect with GitHub")
		return mh.Error(errInvalidOauthCode, errLabel)
	}

	gitHubUserName, tokenJSON, err := FetchGithubUser(code)
	if err != nil {
		logrus.Errorf("failed to fetch GitHub user: '%s'", err)
		redirectError(w, r, "failed to connect with GitHub")
		return mh.Error(err, errLabel)
	}

	if p.GitHubUserName.Valid && gitHubUserName != p.GitHubUserName.String {
		logrus.Errorf("replacing github user for %s - old: %s, new: %s",
			userID, p.GitHubUserName.String, gitHubUserName)
	}

	p.GitHubUserName = sql.NullString{gitHubUserName, true}
	p.GitHubOAuthTokenJson = sql.NullString{tokenJSON, true}
	if err = p.UpdateGithubInfo(db); err != nil {
		logrus.Errorf("p.UpdateGithubInfo() faled with '%s'", err)
		redirectError(w, r, "failed to connect with GitHub")
		return mh.Error(errUpdateFailed, errLabel)
	}

	tracker.SetProfileProps("set", userID, map[string]interface{}{
		"github_username": gitHubUserName,
	})

	tracker.SetProfileProps("setnx", userID, map[string]interface{}{
		"github_authorized_date": time.Now(),
	})

	http.Redirect(w, r, "/app#/account", http.StatusFound)
	return nil
}

func makeOAuthState(userID int64) (string, error) {
	createdAt := time.Now().UTC()
	return verifier.Generate(&OAuthState{
		UserID:    userID,
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(ValidityPeriod),
	})
}

func readOAuthState(signedState string) (*OAuthState, error) {
	if signedState == "" {
		return nil, errInvalidState
	}

	state := &OAuthState{}
	err := verifier.Verify(signedState, state)
	if err != nil {
		return nil, errInvalidState
	}

	if state.ExpiresAt.Before(time.Now()) {
		return nil, errStateExpired
	}
	return state, nil
}

// Reuse existing cookie state if it has been set previously by the
// same API call. Otherwise generate new signed state.
func setOrReuseState(w http.ResponseWriter, r *http.Request, userID int64) (string, error) {
	// Check if existing github_state cookie is set.
	cookie, err := r.Cookie("github_state")
	if err == nil {
		signedState := cookie.Value
		state, err := readOAuthState(signedState)
		if err == nil && state.UserID == userID {
			// If old state is valid, reuse it
			return signedState, nil
		}
	}

	// Otherwise generate new state
	state, err := makeOAuthState(userID)
	if err != nil {
		return "", err
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "github_state",
		Value:  state,
		MaxAge: ValidityPeriodSeconds,
	})

	return state, nil
}

func redirectError(w http.ResponseWriter, r *http.Request, errMsg string) {
	// note: don't url.QueryEscape() errMsg, Redirect will do it
	uri := "/app#/account?errmsg=" + errMsg
	http.Redirect(w, r, uri, http.StatusFound)
}

func tokenToJSON(token *oauth2.Token) (string, error) {
	// FIXME: token.expiry is incorrecty serialized to epoch time
	// when it is supposed to be empty, because of the way
	// time field is defined in oauth2.Token
	if d, err := json.Marshal(token); err != nil {
		return "", err
	} else {
		return string(d), nil
	}
}

// FIXME: The functions below should not belong to this controller package.
// They are used by few external packages.

func tokenFromJSON(jsonStr string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func GitHubClientFromToken(token *oauth2.Token) *github.Client {
	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	return github.NewClient(oauthClient)
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

// Get GitHub username from user profile, if the GitHub token
// is not yet expired. Returns empty string if not authorized
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
