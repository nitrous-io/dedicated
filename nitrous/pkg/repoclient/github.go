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

func (gh *Github) ParseUrlPath(path string) (string, string, error) {
	pathSegments := strings.Split(path, "/")
	if len(pathSegments) < 3 {
		return "", "", ErrInvalidUrlPath
	}
	repoName := strings.Replace(pathSegments[2], ".git", "", -1)
	owner := pathSegments[1]

	return owner, repoName, nil
}
