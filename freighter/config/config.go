// store shared configuration that can be read by other packages
package config

var (
	// default ports to be considered for preview. Tender should be configured
	// to forward those ports.
	MinPreviewPort int64 = 1024
	MaxPreviewPort int64 = 59999

	// default header to read forwarded fort for preview. Tender should write
	// the port to this header
	ForwardPortHeader = "X-FORWARDED-PORT"
)
