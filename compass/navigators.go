package compass

import (
	n "github.com/nitrous-io/dedicated/compass/navigator"
	_ "github.com/nitrous-io/dedicated/compass/navigator/container"
	_ "github.com/nitrous-io/dedicated/compass/navigator/dummy"
	_ "github.com/nitrous-io/dedicated/compass/navigator/host"
	_ "github.com/nitrous-io/dedicated/compass/navigator/proxy"
)

func NewNavigator(provider string, domain string, config map[string]string) (n.Navigator, error) {
	return n.Init(provider, domain, config)
}
