// store shared configuration that can be read by other packages
package config

var (
	// default header to read forwarded fort for preview. Tender should write
	// the port to this header
	ForwardPortHeader = "X-FORWARDED-PORT"
	Version           string
)
