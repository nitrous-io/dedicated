package repoclient

import (
	"errors"
)

type RepoClient interface {
	// returns decoded file content
	FileContent(owner, repo, file string) (content string, err error)
	// returns owner and repository name of the path
	ParseUrlPath(path string) (owner string, repoName string, err error)
}

var (
	ErrContentNotFound = errors.New("content not found")
	ErrInvalidUrlPath  = errors.New("invalid repo URL")
)
