package dns

import "errors"

type Provider interface {
	SetCNAME(src, dst string) error
	DeleteCNAME(domain string) error
}

var ErrDomainDoesNotExist = errors.New("domain does not exist")
