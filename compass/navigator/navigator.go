package navigator

import (
	"errors"
	"fmt"

	"github.com/miekg/dns"
)

var ErrDomainMismatch = errors.New("navigator: domain mismatch, cannot answer DNS query")

type Navigator interface {
	Name() string // tell us what type of navigator you are.
	Lookup(name string) ([]dns.RR, error)
	SOA(domain string) ([]dns.RR, error)
}

type NewNavigator func(domain string, config map[string]string) (Navigator, error)

var Navigators = map[string]NewNavigator{}

func Init(provider string, domain string, config map[string]string) (Navigator, error) {
	init, ok := Navigators[provider]
	if !ok {
		return nil, fmt.Errorf("Invalid navigator %v", provider)
	}
	return init(domain, config)
}
