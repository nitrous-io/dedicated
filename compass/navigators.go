package compass

import (
	n "github.com/nitrous-io/compass/navigator"
	_ "github.com/nitrous-io/compass/navigator/container"
	_ "github.com/nitrous-io/compass/navigator/dummy"
	_ "github.com/nitrous-io/compass/navigator/host"
	_ "github.com/nitrous-io/compass/navigator/proxy"
)

func NewNavigator(provider string, domain string, config map[string]string) (n.Navigator, error) {
	return n.Init(provider, domain, config)
}
