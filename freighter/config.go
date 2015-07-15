package freighter

var (
	// default header to read forwarded fort for preview. Tender should write
	// the port to this header
	ForwardPortHeader = "X-FORWARDED-PORT"
	Version           string
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
	DataPath string `json:"data_path"`

	TLSVerify bool   `json:"tls_verify"` // whether we want to use TLS mode for all HTTP hosts
	CA        string `json:"tls_ca"`     // path to CA
	Cert      string `json:"tls_cert"`   // path to cert
	Key       string `json:"tls_key"`    // path to key
	Secret    string `json:"secret"`     // path to secret

	NotifyURL string `json:"notify_url"`

	Listen []HostAddress `json:"listen"`

	ProfileDumpFolder string `json:"profile_dump_folder"` // Folder for CPU profiling
}
