package host

import (
	"errors"
	"fmt"
	"io"

	"github.com/nitrous-io/dedicated/freighter/config"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
)

const (
	DefaultPort uint16 = 65333
)

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
	ConfigLocation  = "/etc/freighterd/config.json"

	ErrTimeout = errors.New("operation timedout")
)

// for setting up repository on the host
type AptConfig struct {
	AptUsername string `json:"apt_username"` // host id on nitrousd
	AptPassword string `json:"apt_password"` // host secret
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

// for setting up TLS certificates on the host
type TLSConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// settings of the remote host. This includes contents that should be uploaded
// to the remote host
type Settings struct {
	*TLSConfig
	*AptConfig
	*config.Config
	Home       string // Home directory of the current user
	Secret     string
	Version    string
	PublicKeys []string
}

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
	// Settings returns the current dynamic settings for a host, which
	// includes the secret key, TLS certificates, apt information etc..
	Settings() *Settings
}

type Logger interface {
	// MetricsLabel returns the string that can be used to identify the
	// current provider/region combination for the current operation.
	MetricsLabel(prefix string) string

	// Logf logs the data together with host's specific information such as the
	// current instance ID and/or the current request UUID
	Logf(format string, v ...interface{})
}

// executor represents the connection to the remote host. We can use this to
// perform setup tasks such as uploading files, running commands on the remote
// host. The connection is only available during the host creation phase. The
// host's filesystem should be considered immutable otherwise.
type Executor interface {
	// Upload transfers the content of the reader to the path on a remote server
	Upload(path string, input io.Reader) error
	// Execute runs the command on the remote server and returns its error
	Execute(cmd string) error
	// CombinedOutput executes the command on the remote server and returns its
	// combined standard stdout/stderr
	CombinedOutput(cmd string) ([]byte, error)
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
