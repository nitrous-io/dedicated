package navigator

import (
	"fmt"

	"github.com/miekg/dns"
)

type Navigator interface {
	Name() string // tell us what type of navigator you are.

	// Lookup returns DNS records for the given DNS name. Callers must ensure
	// that any Unicode characters outside of the ASCII set are converted to
	// Punycode.
	Lookup(name string) ([]dns.RR, error)
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
