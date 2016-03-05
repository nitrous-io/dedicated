package linkedin

import (
	"database/sql"
	//	"encoding/json"
	"fmt"
	//	"io/ioutil"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthtoken"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	//	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/linkedin"
)

var (
	clientID     = envvars.LinkedinClientID
	clientSecret = envvars.LinkedinClientSecret
	redirectUrl  = fmt.Sprintf("https://%s/api/v0/linkedin/oauth_cb", envvars.WebHostname)
)

var (
	errInvalidBonusAction = httperror.New(http.StatusNotFound, "bonus_not_found", "the bonus type is invalid")
	errBonusAlreadyEarned = httperror.New(http.StatusForbidden, "forbidden", "you have already earned this bonus")
	errUserNotFound       = httperror.New(http.StatusNotFound, "user_not_found", "user eligible for bonus not found")
	errForbidden          = httperror.New(http.StatusForbidden, "forbidden", "you do not have permission to access the requested resource")
)

var lnConfig = &oauth2.Config{
	ClientID:     clientID,
	ClientSecret: clientSecret,
	RedirectURL:  redirectUrl,
	// TODO: replace r_basicprofile with r_fullprofile when we get access
	Scopes:   []string{"r_basicprofile"},
	Endpoint: linkedin.Endpoint,
}

// FIXME: Fill up user data we want to store in db when we get full profile access
// approved by linkedin
type user struct {
	Id string `json:"id"`
}

// /api/v0/linkedin/connect
// Connect implements sign in with linkedin (https://developer.linkedin.com/docs/signin-with-linkedin) and retrieves and stores user's basic profile data
func Connect(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	state, err := securerandom.Alphanumeric(24)
	if err != nil {
		return err
	}

	redirectUrl := fmt.Sprintf("%v&state=%v", lnConfig.AuthCodeURL(""), state)

	t := &oauthtoken.OAuthToken{
		UserID:       env.User.ID,
		Service:      oauthtoken.Linkedin,
		NitrousToken: sql.NullString{env.Token.Token, true},
		State:        sql.NullString{state, true},
	}
	// Update or insert for users who click on "connect linkedin" (record is inserted here), is
	// redirected to the authentication page, changes their mind, and later decides to connect
	// again
	if err := t.Upsert(env.DB); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"redirect_url": redirectUrl,
	}

	return nil
}

// api/v0/linkedin/oauth_cb
// OAuthCallback is called after the user authenticates on the authentication page after being
// redirected to it, on clicking "connect with linkedin".
func OAuthCallback(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	state := r.FormValue("state")

	if state == "" {
		if r.FormValue("error") != "" {
			logrus.Errorf("linked in oauth callback failed. error: %v, error_description: %v", r.FormValue("error"), r.FormValue("error_description"))
		}
		return errForbidden
	}

	t, err := oauthtoken.FindByStateAndService(env.DB, state, oauthtoken.Linkedin)
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

	tok, err := lnConfig.Exchange(oauth2.NoContext, r.FormValue("code"))
	if err != nil {
		return err
	}

	// Store linked in user data in linkedin table
	t.AccessToken = sql.NullString{tok.AccessToken, true}
	if err := t.Update(env.DB); err != nil {
		fmt.Println("failed 4")
		return err
	}

	response, err := http.Get("https://api.linkedin.com/v1/people/~?format=json&oauth2_access_token=" + tok.AccessToken)
	if err != nil {
		return err
	}

	//	var userData user
	//	body, err := ioutil.ReadAll(response.Body)
	//	fmt.Println("body", string(body))
	//	err = json.Unmarshal(body, &userData)
	//	if err != nil {
	//		return err
	//	}

	// Insert into linked in table

	// Grant earned bonus hours to user
	bn, err := bonus.FindBySlug(env.DB, "connect-linkedin")
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

	redirectUrl := fmt.Sprintf("https://%s/#/dashboard/earn-bonuses?connected_linkedin=true", envvars.WebHostname)

	http.Redirect(w, r, redirectUrl, http.StatusFound)

	env.Status = http.StatusFound
	env.Data = map[string]interface{}{}

	return nil
}
