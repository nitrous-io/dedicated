package github

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/google/go-github/github"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/redispool"
	"github.com/nitrous-io/dedicated/pkg/uuid"
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
		Scopes:       []string{"user:email", "repo", "admin:public_key"},
		Endpoint:     githuboauth.Endpoint,
	}

	redisPool       *redispool.Pool
	oauthScopesJson string

	errUpdateFailed     = httperror.New(http.StatusInternalServerError, "update_failed", "update failed")
	errInvalidOauthCode = httperror.New(http.StatusBadRequest, "invalid_code", "invalid code parameter")
	errInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "invalid state parameter")
	errStateExpired     = httperror.New(http.StatusBadRequest, "auth_timeout", "GitHub authorization timeout")
	errStateMismatch    = httperror.New(http.StatusBadRequest, "state_mismatch", "state provided does not match cookie state")

	errAuthorizeForbidden   = httperror.New(http.StatusForbidden, "authorize_forbidden", "You do not have permission to authorize GitHub")
	errInvalidOAuthCallback = httperror.New(http.StatusBadRequest, "invalid_oauth_callback", "unexpected call from GitHub")

	ErrTokenEmpty       = errors.New("token is empty")
	ErrTokenNotValid    = errors.New("token not valid")
	ErrKeyAlreadyExists = errors.New("key is already added to GitHub")
)

type OAuthState struct {
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token,omitempty"`
	Purpose   string    `json:"purpose,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

const (
	OAuthStatePurposeConnect = "connect"
	OAuthStatePurposeLogin   = "login"
	OAuthStatePurposeSignUp  = "signup"
)

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

var AddKeyToGitHub = func(env *middleware.Env, h *host.Host, cn *container.Container, publicKey string) error {
	token, err := GetTokenIfValid(env)
	if err != nil {
		return err
	}

	// get current keys on GitHub to avoid adding duplicates
	gitHubClient := GitHubClientFromToken(token)
	existingKeys, err := githubKeys(gitHubClient)
	if err != nil {
		return err
	}

	for _, k := range existingKeys {
		if k.Key != nil && publicKey == *k.Key {
			return ErrKeyAlreadyExists
		}
	}

	title := fmt.Sprintf("nitrous-%s-%s", h.Name.String, cn.Name.String)
	newKey := &github.Key{
		Title: &title,
		Key:   &publicKey,
	}

	if _, _, err = gitHubClient.Users.CreateKey(newKey); err != nil {
		return err
	}

	return nil
}

var GetRedisKeyForOAuth = func(token string) string {
	return fmt.Sprintf("github:login:uuid:%s", token)
}

func init() {
	redisPool = redispool.New(net.JoinHostPort(envvars.RedisHost, envvars.RedisPort))
	oauthScopesJsonBytes, err := json.Marshal(oauthConf.Scopes)
	if err != nil {
		logrus.Errorf("failed to marshal oauth scope: %+v", err)
	}
	oauthScopesJson = string(oauthScopesJsonBytes)
}

// /api/v0/github/authorize
// Authorize returns GitHub authorization url that front-end needs to redirect to.
// FIXME: allow this API to be called only if user is not yet linked with a GitHub account
func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if !permission.AuthorizeGitHub(env.Token, env.User) {
		return errAuthorizeForbidden
	}

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

// /api/v0/github/authorize_to_login
// Authorize returns GitHub authorization url that front-end needs to redirect to for login with GitHub.
func AuthorizeToLogin(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	state, err := setState(w, r, OAuthStatePurposeLogin)
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

// /api/v0/github/authorize_to_signup
// AuthorizeToSignUp returns GitHub authorization url that front-end needs to redirect to for sign up with GitHub.
func AuthorizeToSignUp(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	state, err := setState(w, r, OAuthStatePurposeSignUp)
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

	switch state.Purpose {
	case OAuthStatePurposeLogin:
		if err := loginWithGitHub(env, w, state, gitHubUserName, tokenJSON); err != nil {
			http.Redirect(w, r, "/app#/login?oauthError=true", http.StatusFound)
			return mh.Error(err, errLabel)
		}

		t, err := token.FindByUser(db, env.User.ID)
		if err != nil {
			return err
		}

		urlWithToken := fmt.Sprintf("/app#/login?token=%s", t.Token)
		http.Redirect(w, r, urlWithToken, http.StatusFound)
	default:
		if err := connectToGitHub(env, state, gitHubUserName, tokenJSON); err != nil {
			redirectError(w, r, "failed to connect with GitHub")
			return mh.Error(err, errLabel)
		}

		http.Redirect(w, r, "/app#/account", http.StatusFound)
	}

	return nil
}

func connectToGitHub(env *middleware.Env, state *OAuthState, gitHubUserName string, tokenJSON string) error {
	db := env.DB
	userID := state.UserID
	tracker := env.Tracker
	p, err := userprofile.Find(db, userID)
	if err != nil {
		return err
	}

	if p.GitHubUserName.Valid && gitHubUserName != p.GitHubUserName.String {
		logrus.Errorf("replacing github user for %s - old: %s, new: %s",
			userID, p.GitHubUserName.String, gitHubUserName)
	}

	p.GitHubUserName = sql.NullString{gitHubUserName, true}
	p.GitHubOAuthTokenJson = sql.NullString{tokenJSON, true}
	p.GitHubUserScopesJson = sql.NullString{oauthScopesJson, true}
	if err := p.UpdateGithubInfo(db); err != nil {
		logrus.Errorf("p.UpdateGithubInfo() failed with '%s'", err)
		return errUpdateFailed
	}

	tracker.SetProfileProps("set", userID, map[string]interface{}{
		"github_username": gitHubUserName,
	})

	tracker.SetProfileProps("setnx", userID, map[string]interface{}{
		"github_authorized_date": time.Now(),
	})

	return nil
}

func loginWithGitHub(env *middleware.Env, w http.ResponseWriter, state *OAuthState, gitHubUserName string, tokenJSON string) error {
	db := env.DB

	conn := redisPool.Get()
	defer conn.Close()
	val, err := redis.Int64(conn.Do("DEL", GetRedisKeyForOAuth(state.Token)))
	if err != nil {
		return err
	}

	// When the key does not exist.
	// The key could be expired or user request to this url manually
	if val != 1 {
		logrus.Errorf("Unexpected callback request. GitHub authorize call could take more than 10 minutes or a user sent this manually: %s", gitHubUserName)
		return errInvalidOAuthCallback
	}

	p, err := userprofile.FindByGitHubUserName(db, gitHubUserName)
	if err != nil {
		return err
	}

	userID := p.UserID
	p.GitHubUserName = sql.NullString{gitHubUserName, true}
	p.GitHubOAuthTokenJson = sql.NullString{tokenJSON, true}
	p.GitHubUserScopesJson = sql.NullString{oauthScopesJson, true}
	if err := p.UpdateGithubInfo(db); err != nil {
		logrus.Errorf("p.UpdateGithubInfo() failed with '%s'", err)
		return errUpdateFailed
	}

	u, err := user.FindByID(db, userID)
	if err != nil {
		return err
	}

	if err := env.Login(u, w); err != nil {
		return err
	}

	env.User = u
	return nil
}

func makeOAuthState(userID int64, token string, purpose string) (string, error) {
	createdAt := time.Now().UTC()
	return verifier.Generate(&OAuthState{
		UserID:    userID,
		Token:     token,
		Purpose:   purpose,
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

func setState(w http.ResponseWriter, r *http.Request, purpose string) (string, error) {
	tempKey := uuid.Make()
	state, err := makeOAuthState(0, tempKey, purpose)
	if err != nil {
		return "", err
	}

	// Since we don't have userID here, it create a random UUID and keep in Redis.
	// It will check if the request is vaild with this random UUID on callback.
	// This is to prevent user's manully call. It will be expired after 10 minutes.
	conn := redisPool.Get()
	defer conn.Close()
	ttl := 10 * 60 // 600 seconds
	if _, err := conn.Do("SETEX", GetRedisKeyForOAuth(tempKey), ttl, "1"); err != nil {
		return "", err
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "github_state",
		Value:  state,
		MaxAge: ValidityPeriodSeconds,
	})

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
	state, err := makeOAuthState(userID, "", OAuthStatePurposeConnect)
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

func CanDeleteGitHubPublicKeys(p *userprofile.UserProfile) bool {
	if !p.GitHubUserScopesJson.Valid || p.GitHubUserScopesJson.String == "" {
		return false
	}

	var scopes []string
	err := json.Unmarshal([]byte(p.GitHubUserScopesJson.String), &scopes)
	if err != nil {
		logrus.Infof("Failed to parse github scopes for user %+v: %+v", p, err)
		return false
	}

	for _, scope := range scopes {
		if scope == "admin:public_key" {
			return true
		}
	}

	return false
}

func githubKeys(gitHubClient *github.Client) ([]github.Key, error) {
	// we pass empty user name even though we know it because that will call
	// https://developer.github.com/v3/users/keys/#list-your-public-keys
	// rather than
	// https://developer.github.com/v3/users/keys/#list-public-keys-for-a-user
	// oauth token
	// which returns more information. User name will be deduced from
	existingKeys, _, err := gitHubClient.Users.ListKeys("", nil)
	if err != nil {
		return nil, err
	}

	return existingKeys, nil
}
