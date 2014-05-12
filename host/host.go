package host

import "fmt"

type Info struct {
	Provider  string `json:"provider"`
	Id        string `json:"id"`
	IpAddress string `json:"ip_address"`
}

type Host interface {
	Create() (*Info, error) // create a new host
	Destroy() error         // destroy the current host
	Stop() error            // stop the current host
	Start() error           // start the current host
	Restart() error         // reboot the current host
	Info() (*Info, error)   // get current info of the host
}

type NewHost func(defaultConfig map[string]string, config string) (Host, error)

var (
	Hosts = map[string]NewHost{}
)

// accept:
// - provider: amazon, digitalocean, etc...
// - defaultConfig: key-value strings. This is typically where we set the
// default value that we use for all boxhosts i.e access/secret key etc...
// - config: json string of user customization
func InitHost(provider string, defaultConfig map[string]string, config string) (Host, error) {
	init, ok := Hosts[provider]
	if !ok {
		return nil, fmt.Errorf("Invalid provider %v", provider)
	}
	return init(defaultConfig, config)
}
