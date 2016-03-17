package repoclient

import (
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type Github struct {
	Client *github.Client
}

func NewGithubClient(accessToken string) RepoClient {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)

	githubClient := &Github{
		Client: github.NewClient(tc),
	}
	return githubClient
}

// FIXME This is missing a test.
func (gh *Github) FileContent(owner, repo, file string) (string, error) {
	fileContent, _, _, err := gh.Client.Repositories.GetContents(owner, repo, file, &github.RepositoryContentGetOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return "", ErrContentNotFound
		}
		return "", err
	}
	decodedContent, err := fileContent.Decode()
	if err != nil {
		return "", err
	}
	return string(decodedContent), nil
}

func (gh *Github) ParseURL(u string) (string, string, error) {
	path := strings.TrimPrefix(u, "https://github.com/")
	path = strings.TrimPrefix(path, "git@github.com:")
	pathSegments := strings.Split(path, "/")
	if len(pathSegments) != 2 {
		return "", "", ErrInvalidURLPath
	}
	repoName := strings.TrimSuffix(pathSegments[1], ".git")
	owner := pathSegments[0]

	return owner, repoName, nil
}
