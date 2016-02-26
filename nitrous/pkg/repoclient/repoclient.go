package repoclient

import (
	"errors"
)

type RepoClient interface {
	// returns decoded file content
	FileContent(owner, repo, file string) (string, error)
	// returns owner and repository name of the path
	ParseUrlPath(path string) (string, string, error)
}

var (
	ErrContentNotFound = errors.New("content not found")
	ErrInvalidUrlPath  = errors.New("invalid repo URL")
)
