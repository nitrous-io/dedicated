package amazon

import (
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

func ParseHostbuilder(hbs string) (*host.Host, error) {
	return parseHostbuilder(hbs)
}
