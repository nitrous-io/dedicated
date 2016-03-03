package hack

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	mw "github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/repoclient"
)

var (
	GithubRepoClient repoclient.RepoClient

	errRepoInfo            = httperror.New(http.StatusForbidden, "invalid_request", "Invalid repository info")
	errInvalidUrl          = httperror.New(http.StatusForbidden, "invalid_request", "Invalid repository URL")
	errNitrousJSONNotFound = httperror.New(http.StatusForbidden, "invalid_request", "nitrous.json not found")
)

type repository struct {
	Owner    string  `json:"owner"`
	RepoName string  `json:"repo_name"`
	Template string  `json:"template"`
	Ports    []int64 `json:"ports"`
}

func init() {
	GithubRepoClient = repoclient.NewGithubClient(envvars.GithubAccessToken)
}

func New(env *mw.Env, w http.ResponseWriter, r *http.Request) error {
	query := url.Values{}
	query.Set("repo", r.FormValue("repo"))
	redirectURL := fmt.Sprintf("/app/#/dashboard/hack?%s", query.Encode())
	http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
	return nil
}

func RepoInfo(env *mw.Env, w http.ResponseWriter, r *http.Request) error {
	repoUrl := r.FormValue("repo")
	parsedURL, err := url.Parse(repoUrl)
	if err != nil {
		return err
	}
	urlHost := parsedURL.Host
	urlPath := parsedURL.Path
	var owner, content, repoName string
	switch urlHost {
	case "github.com":
		owner, repoName, err = GithubRepoClient.ParseUrlPath(urlPath)
		if err == repoclient.ErrInvalidUrlPath {
			return errInvalidUrl
		}
		content, err = GithubRepoClient.FileContent(owner, repoName, "nitrous.json")
		if err != nil {
			if err == repoclient.ErrContentNotFound {
				return errNitrousJSONNotFound
			}
			return err
		}
		break
	default:
		return errInvalidUrl
	}

	repoInfo := repository{
		Owner:    owner,
		RepoName: repoName,
		Template: "",
		Ports:    nil,
	}

	if err := json.NewDecoder(strings.NewReader(content)).Decode(&repoInfo); err != nil {
		return err
	}

	if err := validateRepoInfo(&repoInfo); err != nil {
		return err
	}

	// We confirm user email here so that we can create a new host for
	// new user with quickstart button intent. Currently user will not be able
	// to create a new host if their email is not confirmed.
	if err := confirmEmail(env); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hack": repoInfo,
	}

	return nil
}

func validateRepoInfo(repoInfo *repository) error {
	if repoInfo.Template == "" {
		return errRepoInfo
	}
	return nil
}

func confirmEmail(env *mw.Env) error {
	u := env.User
	db := env.DB

	if u.ConfirmedAt.Valid {
		return nil
	}

	if err := u.ConfirmEmail(db); err != nil {
		return err
	}

	bp, err := u.BillingProfile(db)
	if err != nil {
		return err
	}
	if bp.State == billingprofile.StateNoCard {
		bp.State = billingprofile.StateFree
		if err := bp.Update(db); err != nil {
			return err
		}
	}

	return nil
}
