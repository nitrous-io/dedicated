package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type AdminHost struct {
	Host
	Preprovisioned bool `json:"preprovisioned"`
}

func SerializeAdminHost(h *host.Host) *AdminHost {
	if h == nil {
		return nil
	}
	serializedHost := SerializeHost(h)
	return &AdminHost{
		Host:           *serializedHost,
		Preprovisioned: false,
	}
}

func SerializeAdminHosts(hosts []*host.Host) []*AdminHost {
	ret := make([]*AdminHost, len(hosts))
	for i, h := range hosts {
		ret[i] = SerializeAdminHost(h)
	}
	return ret
}
