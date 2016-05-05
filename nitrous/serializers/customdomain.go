package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/customdomain"
)

type CustomDomain struct {
	ID            int64      `json:"id"`
	ContainerID   int64      `json:"container_id"`
	Domain        string     `json:"domain"`
	Port          int64      `json:"port"`
	ContainerPort int64      `json:"container_port"`
	CreatedAt     *time.Time `json:"created_at"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

func SerializeCustomDomain(cd *customdomain.CustomDomain) *CustomDomain {
	if cd == nil {
		return nil
	}
	return &CustomDomain{
		ID:            cd.ID,
		ContainerID:   cd.ContainerID,
		Domain:        cd.Domain,
		Port:          cd.Port,
		ContainerPort: cd.ContainerPort,
		CreatedAt:     cd.CreatedAt,
		UpdatedAt:     cd.UpdatedAt,
	}
}

func SerializeCustomDomains(cds []*customdomain.CustomDomain) []*CustomDomain {
	ret := make([]*CustomDomain, len(cds))
	for i, d := range cds {
		ret[i] = SerializeCustomDomain(d)
	}
	return ret
}
