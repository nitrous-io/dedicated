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
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
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
const RedisTempDataTTL = 600 // 600 seconds

var (
	Authorize middleware.Middleware = middleware.RateLimit(authorize)
	Repos     middleware.Middleware = middleware.RateLimit(repos)
)

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

	errNotConnectedWithGitHub = httperror.New(http.StatusForbidden, "not_connected_with_github", "You need to connect with GitHub")

	ErrTokenEmpty    = errors.New("token is empty")
	ErrTokenNotValid = errors.New("token not valid")

	errGitHubTokenEmpty    = httperror.New(http.StatusBadRequest, "github_token_empty", "connect your GitHub account")
	errGitHubTokenNotValid = httperror.New(http.StatusBadRequest, "github_token_not_valid", "GitHub token is not valid")

	ErrKeyAlreadyExists = errors.New("key is already added to GitHub")

	errBonusAlreadyEarned     = httperror.New(http.StatusForbidden, "forbidden", "you have already earned this bonus")
	errUserIneligibleForBonus = httperror.New(http.StatusForbidden, "ineligible_for_bonus", "you are not eligible for bonus")
	errAlreadyConnectedGithub = httperror.New(http.StatusForbidden, "already_connected_github", "you have already connected with github")
)

type OAuthState struct {
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token,omitempty"`
	Bonus     bool      `json:"bonus"`
	Purpose   string    `json:"purpose,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type GitHubUserInfo struct {
	UserName  string `json:"username"`
	TokenJSON string `json:"token_json"`
}

// This is subset of git.Repository.
// Refer to https://github.com/google/go-github/blob/master/github/repos.go#L19.
// Please don't change variable names.
type GitHubRepository struct {
	Name    string `json:"name"`
	GitURL  string `json:"git_url"`
	Private bool   `json:"private"`
}

const (
	OAuthStatePurposeConnect = "connect"
	OAuthStatePurposeLogin   = "login"
	OAuthStatePurposeSignUp  = "signup"
)

// return gitHubUserName and gitHubTokenJson
var FetchGitHubUser = func(code string) (string, string, string, error) {
	token, err := oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		return "", "", "", err
	}

	tokenJSON, err := tokenToJSON(token)
	if err != nil {
		return "", "", "", err
	}

	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	client := github.NewClient(oauthClient)

	// get github user name of authenticated user
	user, _, err := client.Users.Get("")
	if err != nil {
		return "", "", "", err
	}

	gitHubUserName := *user.Login
	gitHubUserEmail := ""
	if user.Email != nil {
		gitHubUserEmail = *user.Email
	}
	return gitHubUserName, tokenJSON, gitHubUserEmail, nil
}

var AddKeyToGitHub = func(env *middleware.Env, h *host.Host, cn *container.Container, publicKey string) error {
	token, err := userprofile.GetTokenIfValid(env.DB, env.User.ID)
	if err != nil {
		if err == userprofile.ErrTokenEmpty {
			return errGitHubTokenEmpty
		} else if err == userprofile.ErrTokenNotValid {
			return errGitHubTokenNotValid
		}
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

var FetchRepos = func(env *middleware.Env) ([]GitHubRepository, error) {
	token, err := userprofile.GetTokenIfValid(env.DB, env.User.ID)
	if err != nil {
		return nil, err
	}

	gitHubClient := GitHubClientFromToken(token)
	// If username is not given, it returns current user's repositories
	// Search with default options ( it returns all repos by default )
	repos, _, err := gitHubClient.Repositories.List("", nil)
	if err != nil {
		return nil, err
	}

	var repositories []GitHubRepository
	for _, repo := range repos {
		repository := GitHubRepository{
			Name:    *repo.Name,
			GitURL:  *repo.SSHURL,
			Private: *repo.Private,
		}

		repositories = append(repositories, repository)
	}

	return repositories, nil
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
// authorize returns GitHub authorization url that front-end needs to redirect to.
func authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if !permission.AuthorizeGitHub(env.Token, env.User) {
		return errAuthorizeForbidden
	}

	earnBonusRequest := r.FormValue("bonus") == "true"
	connected, err := userprofile.ConnectedToGithub(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	if connected {
		if !earnBonusRequest {
			return errAlreadyConnectedGithub
		}

		// Grant earned bonus hours to user
		bn, err := bonus.FindBySlug(env.DB, "connect-github")
		if err != nil {
			if err == sql.ErrNoRows {
				bn, err = bonus.Insert(env.DB, "connect-github", 18000)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
		if err := env.User.GrantBonus(env.DB, bn); err != nil {
			if err == bonus.ErrBonusAlreadyEarned {
				return errBonusAlreadyEarned
			}
			return errUserIneligibleForBonus
		}

		ebns, err := bonus.FindEarnedByUser(env.DB, env.User.ID)
		if err != nil {
			return err
		}

		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"earned_bonuses": serializers.SerializeEarnedBonuses(ebns),
		}
		return nil
	}

	state, err := setOrReuseState(w, r, env.User.ID, OAuthStatePurposeConnect, earnBonusRequest)
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
// AuthorizeToLogin returns GitHub authorization url that front-end needs to redirect to for login with GitHub.
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

// /api/v0/github/repos
// repos returns user's public/private github repositories
func repos(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	repositories, err := FetchRepos(env)
	if err == ErrTokenEmpty {
		return errNotConnectedWithGitHub
	} else if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"repos": repositories,
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

	gitHubUserName, tokenJSON, gitHubUserEmail, err := FetchGitHubUser(code)
	if err != nil {
		logrus.Errorf("failed to fetch GitHub user: '%s'", err)
		redirectError(w, r, "failed to connect with GitHub")
		return mh.Error(err, errLabel)
	}

	switch state.Purpose {
	case OAuthStatePurposeLogin, OAuthStatePurposeSignUp:
		loginPath := "/app/#/login"
		signupPath := "/app/#/signup"

		// If `Purpose` is `login` or `signup`, we proceed based on whether the user exists or not
		// If the user exists, it proceeds to login otherwise redirects to signup page
		redirectUrlForError := ""
		if state.Purpose == OAuthStatePurposeLogin {
			redirectUrlForError = fmt.Sprintf("%s?oauthError=true", loginPath)
		} else {
			redirectUrlForError = fmt.Sprintf("%s?oauthError=true", signupPath)
		}

		_, err := userprofile.FindByGitHubUserName(db, gitHubUserName)
		if err == sql.ErrNoRows {
			// if an existing user exists with the same email
			u, err := user.FindByEmail(db, gitHubUserEmail)
			if err != nil && err != sql.ErrNoRows {
				http.Redirect(w, r, redirectUrlForError, http.StatusFound)
				return mh.Error(err, errLabel)
			}
			if u != nil {
				url := fmt.Sprintf("%s?existingGitHubEmail=%s", loginPath, gitHubUserEmail)
				http.Redirect(w, r, url, http.StatusFound)
				return nil
			}

			// Fetch data from github and keep in Redis since there is no user profile record
			// It will be used when a user create an account with github username
			err = storeGitHubInfoInRedis(state, gitHubUserName, tokenJSON)
			if err != nil {
				http.Redirect(w, r, redirectUrlForError, http.StatusFound)
				return mh.Error(err, errLabel)
			}

			url := fmt.Sprintf("%s?gitHubUserName=%s&gitHubUserEmail=%s", signupPath, gitHubUserName, gitHubUserEmail)
			http.Redirect(w, r, url, http.StatusFound)
		} else if err == nil {
			err = loginWithGitHub(env, w, state, gitHubUserName, tokenJSON)
			if err != nil {
				http.Redirect(w, r, redirectUrlForError, http.StatusFound)
				return mh.Error(err, errLabel)
			}

			t, err := token.FindByUser(db, env.User.ID)
			if err != nil {
				http.Redirect(w, r, redirectUrlForError, http.StatusFound)
				return mh.Error(err, errLabel)
			}

			urlWithToken := fmt.Sprintf("%s?token=%s", loginPath, t.Token)
			http.Redirect(w, r, urlWithToken, http.StatusFound)
		} else {
			http.Redirect(w, r, redirectUrlForError, http.StatusFound)
			return mh.Error(err, errLabel)
		}
	default:
		if err := connectToGitHub(env, state, gitHubUserName, tokenJSON); err != nil {
			redirectError(w, r, "failed to connect with GitHub")
			return mh.Error(err, errLabel)
		}

		if state.Bonus {
			userID := state.UserID
			u, err := user.FindByID(db, userID)
			if err != nil {
				return err
			}

			// Grant earned bonus hours to user
			bn, err := bonus.FindBySlug(env.DB, "connect-github")
			if err != nil {
				if err == sql.ErrNoRows {
					bn, err = bonus.Insert(env.DB, "connect-github", 18000)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
			if err := u.GrantBonus(env.DB, bn); err != nil {
				if err == bonus.ErrBonusAlreadyEarned {
					return errBonusAlreadyEarned
				}
				return errUserIneligibleForBonus
			}
		}

		redirect := "/app/#/dashboard"
		if state.Bonus {
			redirect = "/app/#/earn-bonuses?connected_github=true"
		}

		http.Redirect(w, r, redirect, http.StatusFound)
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

	p, err := userprofile.FindByGitHubUserName(db, gitHubUserName)
	if err != nil {
		return err
	}

	if err := validateOAuthCallback(state); err != nil {
		logrus.Errorf("Unexpected callback request. GitHub authorize call could take more than 10 minutes or a user sent this manually: %s, err: %+v", gitHubUserName, err)
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

func storeGitHubInfoInRedis(state *OAuthState, gitHubUserName string, tokenJSON string) error {
	if err := validateOAuthCallback(state); err != nil {
		logrus.Errorf("Unexpected callback request. GitHub authorize call could take more than 10 minutes or a user sent this manually: %s, err: %+v", gitHubUserName, err)
		return err
	}

	conn := redisPool.Get()
	defer conn.Close()

	gitHubInfo := GitHubUserInfo{
		UserName:  gitHubUserName,
		TokenJSON: tokenJSON,
	}

	info, err := json.Marshal(gitHubInfo)
	if err != nil {
		return err
	}

	key := getRedisKeyForGitHubInfo(gitHubUserName)
	if _, err := conn.Do("SETEX", key, RedisTempDataTTL, info); err != nil {
		return err
	}

	return nil
}

func makeOAuthState(userID int64, token string, purpose string, bonus bool) (string, error) {
	createdAt := time.Now().UTC()
	return verifier.Generate(&OAuthState{
		UserID:    userID,
		Token:     token,
		Bonus:     bonus,
		Purpose:   purpose,
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(ValidityPeriod),
	})
}

func validateOAuthCallback(state *OAuthState) error {
	conn := redisPool.Get()
	defer conn.Close()
	val, err := redis.Int64(conn.Do("DEL", GetRedisKeyForOAuth(state.Token)))
	if err != nil {
		return err
	}

	// When the key does not exist.
	// The key could be expired or user request to this url manually
	if val != 1 {
		return errInvalidOAuthCallback
	}

	return nil
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
	state, err := makeOAuthState(0, tempKey, purpose, false)
	if err != nil {
		return "", err
	}

	// Since we don't have userID here, it create a random UUID and keep in Redis.
	// It will check if the request is vaild with this random UUID on callback.
	// This is to prevent user's manully call. It will be expired after 10 minutes.
	conn := redisPool.Get()
	defer conn.Close()
	if _, err := conn.Do("SETEX", GetRedisKeyForOAuth(tempKey), RedisTempDataTTL, "1"); err != nil {
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
func setOrReuseState(w http.ResponseWriter, r *http.Request, userID int64, purpose string, bonus bool) (string, error) {
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
	state, err := makeOAuthState(userID, "", purpose, bonus)
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
	uri := fmt.Sprintf("/app/#/account?errmsg=%s", errMsg)
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
func GitHubClientFromToken(token *oauth2.Token) *github.Client {
	oauthClient := oauthConf.Client(oauth2.NoContext, token)
	return github.NewClient(oauthClient)
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

func UpdateGitHubInfoFromRedis(db db.QueryReceiver, userID int64, gitHubUserName string) error {
	key := getRedisKeyForGitHubInfo(gitHubUserName)
	conn := redisPool.Get()
	defer conn.Close()

	val, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return err
	}

	// Delete after get the value
	if _, err := conn.Do("DEL", key); err != nil {
		return err
	}

	var gitHubInfo GitHubUserInfo
	if err := json.Unmarshal(val, &gitHubInfo); err != nil {
		return err
	}

	p, err := userprofile.Find(db, userID)
	if err != nil {
		return err
	}

	p.GitHubUserName = sql.NullString{gitHubInfo.UserName, true}
	p.GitHubOAuthTokenJson = sql.NullString{gitHubInfo.TokenJSON, true}
	p.GitHubUserScopesJson = sql.NullString{oauthScopesJson, true}
	return p.UpdateGithubInfo(db)
}

func getRedisKeyForGitHubInfo(gitHubUserName string) string {
	return fmt.Sprintf("github:userinfo:%s", gitHubUserName)
}
