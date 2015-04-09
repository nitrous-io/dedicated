package containerkeys

import (
	"net/http"

	githubapi "github.com/google/go-github/github"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	Index       middleware.Middleware = containers.WithContainerAndHost(list, true)
	AddToGitHub middleware.Middleware = containers.WithContainerAndHost(addToGitHub, true)

	// separated out for mocking
	AddGitHubKeysFunc func(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) = addGitHubKeys

	errGitHubTokenEmpty      = httperror.New(http.StatusBadRequest, "github_token_empty", "connect your GitHub account")
	errGitHubTokenNotValid   = httperror.New(http.StatusBadRequest, "github_token_not_valid", "GitHub token is not valid")
	ErrMultipleKeysExist     = httperror.New(http.StatusBadRequest, "multiple_keys_exist", "multiple container keys exist")
	ErrKeyAlreadyExists      = httperror.New(http.StatusConflict, "key_already_exists", "key already exists")
	errContainerNotRunning   = httperror.New(http.StatusConflict, "container_not_running", "container should be running to perform this operation")
	errPubKeyOpsNotSupported = httperror.New(http.StatusBadRequest, "pub_key_ops_not_supported", "public key operations are not supported")
)

// list returns a list of public keys in a given container
func list(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h, env.UUID)
	if err != nil {
		return err
	}
	keys, err := cl.ListContainerKeys(cn.CID)
	if err != nil {
		switch err {
		case freighterclient.ErrContainerNotRunning:
			return errContainerNotRunning
		case freighterclient.ErrPubKeyOpsNotSupported:
			return errPubKeyOpsNotSupported
		default:
			return err
		}
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"keys": keys,
	}
	return nil
}

// we only compare the value because we don't want to create duplicate key
// under a different name
func keyAlreadyExists(existingKeys []githubapi.Key, keyValue string) bool {
	for _, k := range existingKeys {
		if k.Key != nil && keyValue == *k.Key {
			return true
		}
	}
	return false
}

// returns an error or a message to return to the client if everything went well
func addGitHubKeys(cn *container.Container, h *host.Host, env *middleware.Env) (string, error) {
	keyTitle := "nitrous-" + h.Name.String + "-" + cn.Name.String
	token, err := github.GetTokenIfValid(env)
	if err != nil {
		return "", err
	}
	// get current keys on GitHub to avoid adding duplicates
	gitHubClient := github.GitHubClientFromToken(token)
	// we pass empty user name even though we know it because that will call
	// https://developer.github.com/v3/users/keys/#list-your-public-keys
	// rather than
	// https://developer.github.com/v3/users/keys/#list-public-keys-for-a-user
	// which returns more information. User name will be deducted from
	// oauth token
	existingKeys, _, err := gitHubClient.Users.ListKeys("", nil)
	if err != nil {
		return "", err
	}

	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return "", err
	}
	keys, err := cl.ListContainerKeys(cn.CID)
	if err != nil {
		return "", err
	}
	// ListContainerKeys() returns array of public keys, but currently we only
	// expect one key per container
	if len(keys) != 1 {
		return "", ErrMultipleKeysExist
	}

	keyToAdd := keys[0].Key
	if keyAlreadyExists(existingKeys, keyToAdd) {
		return "", ErrKeyAlreadyExists
	}

	newKey := &githubapi.Key{
		Title: &keyTitle,
		Key:   &keyToAdd,
	}

	_, _, err = gitHubClient.Users.CreateKey(newKey)
	if err != nil {
		return "", err
	}
	return "github key added", nil
}

// addToGitHub adds public key in a given container (.ssh/id_rsa.pub) to GitHub's
// list of public keys for a given user.
// if a key already exists in GitHub, we don't add it (but still return a success).
func addToGitHub(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	msg, err := AddGitHubKeysFunc(cn, h, env)
	if err != nil {
		switch err {
		case github.ErrTokenEmpty:
			return errGitHubTokenEmpty
		case github.ErrTokenNotValid:
			return errGitHubTokenNotValid
		default:
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"msg": msg,
	}
	return nil
}
