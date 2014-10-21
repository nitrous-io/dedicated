package navigator

import (
	"fmt"

	"github.com/miekg/dns"
)

type Navigator interface {
	Lookup(name string) ([]dns.RR, error)
	NewSOA() []dns.RR
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
