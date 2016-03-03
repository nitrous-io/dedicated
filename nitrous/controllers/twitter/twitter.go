package twitter

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"github.com/mrjones/oauth"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthtoken"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	ConsumerKey    = envvars.TwitterOAuthConsumerKey
	ConsumerSecret = envvars.TwitterOAuthConsumerSecret

	csmr = oauth.NewConsumer(
		ConsumerKey,
		ConsumerSecret,
		oauth.ServiceProvider{
			RequestTokenUrl:   "https://api.twitter.com/oauth/request_token",
			AuthorizeTokenUrl: "https://api.twitter.com/oauth/authorize",
			AccessTokenUrl:    "https://api.twitter.com/oauth/access_token",
		})
)

var (
	errGetTokenAndUrlFailed = httperror.New(http.StatusInternalServerError, "get_token_url_failed", "failed to get token and url")
	errUserNotFound         = httperror.New(http.StatusNotFound, "user_not_found", "user eligible for bonus not found")
	errAuthorizeFailed      = httperror.New(http.StatusUnauthorized, "authorize_failed", "failed to authorize token")
	errFailedToFollow       = httperror.New(http.StatusBadRequest, "follow_failed", "failed to follow")
	errFailedToTweet        = httperror.New(http.StatusBadRequest, "tweet_failed", "failed to tweet")
	errDuplicateTweet       = httperror.New(http.StatusBadRequest, "tweet_failed", "duplicate tweet")
	errTokenNotFound        = httperror.New(http.StatusNotFound, "token_not_found", "token not found, please connect with twitter")
	errInvalidBonusAction   = httperror.New(http.StatusNotFound, "bonus_not_found", "the bonus type is invalid")
	errInvalidOAuthParam    = httperror.New(http.StatusBadRequest, "invalid_param", "invalid oauth param")
	errBonusAlreadyEarned   = httperror.New(http.StatusForbidden, "forbidden", "you have already earned this bonus")
	errForbidden            = httperror.New(http.StatusForbidden, "forbidden", "you do not have permission to access the requested resource")
)

func init() {
	csmr.Debug(true)
}

var (
	GetRequestTokenAndUrl = func(callbackUrl string) (*oauth.RequestToken, string, error) {
		return csmr.GetRequestTokenAndUrl(callbackUrl)
	}

	AuthorizeToken = func(requestToken *oauth.RequestToken, oauthVerifier string) (*oauth.AccessToken, error) {
		return csmr.AuthorizeToken(requestToken, oauthVerifier)
	}

	CreateFriendship = func(client *http.Client) (*http.Response, error) {
		return client.PostForm(
			"https://api.twitter.com/1.1/friendships/create.json",
			url.Values{"screen_name": []string{"nitrous"}})
	}

	PostStatus = func(status string, client *http.Client) (*http.Response, error) {
		return client.PostForm(
			"https://api.twitter.com/1.1/statuses/update.json",
			url.Values{"status": []string{status}})
	}
)

// /api/v0/twitter/connect
// Connect implements sign in with twitter (https://dev.twitter.com/web/sign-in/implementing)
// to obtain access token needed for performing authenticated requests, such as tweeting on
// the user's behalf.
func Connect(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	callbackUrl := fmt.Sprintf("https://%s/api/v0/twitter/oauth_cb", envvars.WebHostname)

	requestToken, authUrl, err := GetRequestTokenAndUrl(callbackUrl)
	if err != nil {
		return errGetTokenAndUrlFailed
	}

	t := &oauthtoken.OAuthToken{
		UserID:       env.User.ID,
		Service:      oauthtoken.Twitter,
		NitrousToken: sql.NullString{env.Token.Token, true},
		RequestToken: sql.NullString{requestToken.Token, true},
	}
	// Update or insert for users who click on "connect twitter" (record is inserted here), is
	// redirected to the authentication page, changes their mind, and later decides to connect
	// again
	if err := t.Upsert(env.DB); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"redirect_url": authUrl,
	}

	return nil
}

// api/v0/twitter/oauth_cb
// OAuthCallback is called after the user authenticates on the authentication page after being
// redirected to it, on clicking "connect with twitter".
func OAuthCallback(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	oauthToken := r.FormValue("oauth_token")
	oauthVerifier := r.FormValue("oauth_verifier")

	if oauthToken == "" || oauthVerifier == "" {
		return errInvalidOAuthParam
	}

	t, err := oauthtoken.FindByTokenAndService(env.DB, oauthToken, oauthtoken.Twitter)
	if err != nil {
		if err == sql.ErrNoRows {
			return errForbidden
		}
		return err
	}

	// Authenticate user by token (nitrous)
	if err := middleware.Authenticate(t.NitrousToken.String, env, w, r); err != nil {
		return err
	}

	// Authorize request token and verifier to get oauth access token (twitter)
	requestToken := &oauth.RequestToken{Token: oauthToken}
	accessToken, err := AuthorizeToken(requestToken, oauthVerifier)
	if err != nil {
		return errAuthorizeFailed
	}

	// Store access token and secret so that we can perform authenticated request on behalf of
	// user in future
	t.Secret = sql.NullString{accessToken.Secret, true}
	t.AccessToken = sql.NullString{accessToken.Token, true}
	if err := t.Update(env.DB); err != nil {
		return err
	}

	// Grant earned bonus hours to user
	bn, err := bonus.FindBySlug(env.DB, "connect-twitter")
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidBonusAction
		}
		return err
	}
	if err := env.User.GrantBonus(env.DB, bn); err != nil {
		if err == bonus.ErrBonusAlreadyEarned {
			return errBonusAlreadyEarned
		}
		return errUserNotFound
	}

	http.Redirect(w, r, "/#/dashboard/earn-bonuses?connected_twitter=true", http.StatusFound)

	env.Status = http.StatusFound
	env.Data = map[string]interface{}{}

	return nil
}

// /api/v0/twitter/follow
// Follow implements follow nitrous on twitter with the stored access token and
// secret (https://dev.twitter.com/rest/reference/post/friendships/create)
func Follow(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	t, err := oauthtoken.FindByUserAndService(env.DB, env.User.ID, oauthtoken.Twitter)
	if err != nil {
		if err == sql.ErrNoRows {
			return errTokenNotFound
		}
		return err
	}

	accessToken := &oauth.AccessToken{
		Token:  t.AccessToken.String,
		Secret: t.Secret.String,
	}
	client, err := csmr.MakeHttpClient(accessToken)
	if err != nil {
		return err
	}

	resp, err := CreateFriendship(client)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errFailedToFollow
	}

	// Grant earned bonus hours to user
	bn, err := bonus.FindBySlug(env.DB, "follow-twitter")
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidBonusAction
		}
		return err
	}
	if err := env.User.GrantBonus(env.DB, bn); err != nil {
		if err == bonus.ErrBonusAlreadyEarned {
			return errBonusAlreadyEarned
		}
		return errUserNotFound
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

// /api/v0/twitter/tweet
// Tweet posts a status on user's timeline on user's behalf with the stored access token and
// secret (https://dev.twitter.com/rest/reference/post/statuses/update)
func Tweet(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	t, err := oauthtoken.FindByUserAndService(env.DB, env.User.ID, oauthtoken.Twitter)
	if err != nil {
		if err == sql.ErrNoRows {
			return errTokenNotFound
		}
		return err
	}

	accessToken := &oauth.AccessToken{
		Token:  t.AccessToken.String,
		Secret: t.Secret.String,
	}
	client, err := csmr.MakeHttpClient(accessToken)
	if err != nil {
		return err
	}

	status := "I'm using @nitrous, the fastest way to code in the cloud! Claim your free account here: https://www.nitrous.io"

	resp, err := PostStatus(status, client)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return errDuplicateTweet
		}
		return errFailedToTweet
	}

	// Grant earned bonus hours to user
	bn, err := bonus.FindBySlug(env.DB, "tweet")
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidBonusAction
		}
		return err
	}
	if err := env.User.GrantBonus(env.DB, bn); err != nil {
		if err == bonus.ErrBonusAlreadyEarned {
			return errBonusAlreadyEarned
		}
		return errUserNotFound
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
