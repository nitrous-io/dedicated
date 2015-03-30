package host

import (
	"fmt"
	"io"

	"github.com/nitrous-io/dedicated/hostbuilder/state"
)

type Info struct {
	Provider       string      `json:"provider"`
	ID             string      `json:"id"`
	Region         string      `json:"region_id"`
	Name           string      `json:"name"`
	IPAddress      string      `json:"ip_address"`
	FreighterdPort uint16      `json:"freighterd_port"`
	State          state.State `json:"state"`
}

type Host interface {
	Create(io.Writer) (*Info, error) // create a new host
	Update(io.Writer) (*Info, error)
	Destroy() error        // destroy the current host
	Stop() (*Info, error)  // stop the current host
	Start() (*Info, error) // start the current host

	Restart() (*Info, error) // reboot the current host
	Info() (*Info, error)    // get current info of the host
	List() ([]*Info, error)

	DriverName() string
}

type Logger interface {
	// MetricsLabel returns the string that can be used to identify the
	// current provider/region combination for the current operation.
	MetricsLabel(prefix string) string

	// Logf logs the data together with host's specific information such as the
	// current instance ID and/or the current request UUID
	Logf(format string, v ...interface{})
}

type NewHost func(defaultConfig map[string]string, config string) (Host, error)

var (
	Hosts = map[string]NewHost{}
)

// accept:
// - provider: amazon, digitalocean, etc...
// - defaultConfig: key-value strings. This is typically where we set the
// default value that we use for all container hosts i.e access/secret key etc...
// - config: json string of user customization
func InitHost(provider string, defaultConfig map[string]string, config string) (Host, error) {
	init, ok := Hosts[provider]
	if !ok {
		return nil, fmt.Errorf("Invalid provider %v", provider)
	}
	return init(defaultConfig, config)
}
