package repoclient

import (
	"errors"
)

type RepoClient interface {
	// returns decoded file content
	FileContent(owner, repo, file string) (content string, err error)
	// returns owner and repository name of a repository, given its URL
	ParseURL(u string) (owner string, repoName string, err error)
}

var (
	ErrContentNotFound = errors.New("content not found")
	ErrInvalidURLPath  = errors.New("invalid repo URL")
)
