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
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/repoclient"
)

var (
	GithubRepoClient repoclient.RepoClient

	errRepoInfo            = httperror.New(http.StatusForbidden, "invalid_request", "Invalid repository info")
	errInvalidUrl          = httperror.New(http.StatusForbidden, "invalid_request", "Invalid repository URL")
	errFileContentNotFound = httperror.New(http.StatusForbidden, "invalid_request", "Failed to retrieve repository info")
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
	var repo string
	if r.FormValue("repo") != "" {
		repo = r.FormValue("repo")
	} else if r.Referer() != "" {
		parsedUrl, err := url.Parse(r.Referer())
		if err != nil {
			return err
		}
		paths := strings.Split(parsedUrl.Path, "/")
		switch parsedUrl.Host {
		case "github.com":
			if len(paths) >= 3 {
				repo = fmt.Sprintf("https://github.com/%s/%s", paths[1], paths[2])
			} else {
				return errInvalidUrl
			}
		default:
			return errInvalidUrl
		}
	} else {
		return errInvalidUrl
	}

	query := url.Values{}
	query.Set("repo", repo)
	redirectURL := fmt.Sprintf("/app/#/dashboard/hack?%s", query.Encode())
	http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
	return nil
}

func RepoInfo(env *mw.Env, w http.ResponseWriter, r *http.Request) error {
	repoUrl := r.FormValue("repo")
	parsedUrl, err := url.Parse(repoUrl)
	if err != nil {
		return err
	}
	var owner, content, repoName string
	switch parsedUrl.Host {
	case "github.com":
		// We try to use user's access token to make Github API request
		// to access their private repo. If user have not connected their
		// Github account (don't have access token), it will use
		// nitrous-public-repo-access Github user's token which is in envvars.
		accessToken, err := getGithubAccessToken(env)
		if err != nil {
			return err
		}
		if accessToken != envvars.GithubAccessToken {
			GithubRepoClient = repoclient.NewGithubClient(accessToken)
		}
		owner, repoName, err = GithubRepoClient.ParseUrlPath(parsedUrl.Path)
		if err == repoclient.ErrInvalidUrlPath {
			return errInvalidUrl
		}
		content, err = GithubRepoClient.FileContent(owner, repoName, "nitrous.json")
		if err != nil {
			if err == repoclient.ErrContentNotFound {
				return errFileContentNotFound
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

func getGithubAccessToken(env *mw.Env) (string, error) {
	profile, err := userprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return "", nil
	}
	githubOAuthTokenJson := profile.GitHubOAuthTokenJson.String
	if githubOAuthTokenJson == "" {
		return envvars.GithubAccessToken, nil
	}

	var tokenJson struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(githubOAuthTokenJson), &tokenJson); err != nil {
		return "", err
	}
	return tokenJson.AccessToken, nil
}
