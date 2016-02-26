package hack

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	mw "github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/repoclient"
)

var (
	GithubRepoClient repoclient.RepoClient

	errInvalidUrl          = httperror.New(http.StatusForbidden, "invalid_request", "Invalid repo URL")
	errNitrousJSONNotFound = httperror.New(http.StatusForbidden, "invalid_request", "nitrous.json not found")
)

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

	repoInfo := struct {
		Owner    string  `json:"owner"`
		RepoName string  `json:"repo_name"`
		Template string  `json:"template"`
		Ports    []int64 `json:"ports"`
	}{
		owner,
		repoName,
		"",
		nil,
	}

	err = json.NewDecoder(strings.NewReader(content)).Decode(&repoInfo)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hack": repoInfo,
	}

	return nil
}
