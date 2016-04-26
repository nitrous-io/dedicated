package freighter

var (
	// These headers are set in Atlas.
	// TODO: Remove this header.
	// TODO: Support current port configuration (i.e. via hostport:containerport mapping)
	// as an advanced option and to allow non-HTTP traffic to be forwarded to containers.

	// User don't have to configure port mapping anymore because we support
	// port 1024 to 9999 for preview by default now.
	// ForwardPortHeader is used for Websocket connection from browser to freighterd.
	ForwardPortHeader = "X-FORWARDED-PORT"
	// ContainerSlugHeader and ContainerPortHeader are used for preview apps/sites.
	ContainerSlugHeader = "X-NITROUS-PROXY-CONTAINER-SLUG"
	ContainerPortHeader = "X-NITROUS-PROXY-CONTAINER-PORT"

	Version string
)

// HostAddress is an address that the server will listen at. For example
// { "proto": "unix", "address": "/var/run/docker.sock" }
// { "proto": "tcp", "address": "0.0.0.0:65333" }
type HostAddress struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`
}

// Config stores shared configuration that can be read by other packages
type Config struct {
	Debug bool `json:"debug"`

	DataPath string `json:"data_path"`

	TLSVerify bool   `json:"tls_verify"` // whether we want to use TLS mode for all HTTP hosts
	CA        string `json:"tls_ca"`     // path to CA
	Cert      string `json:"tls_cert"`   // path to cert
	Key       string `json:"tls_key"`    // path to key
	Secret    string `json:"secret"`     // path to secret

	NotifyURL string `json:"notify_url"`

	Listen []HostAddress `json:"listen"`

	ProfileDumpFolder string `json:"profile_dump_folder"` // Folder for CPU profiling

	IdleThreshold int `json:"idle_threshold"` // number of seconds a console is idle before deeming host inactive
}
