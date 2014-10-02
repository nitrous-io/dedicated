package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/registry"
)

type Registry struct {
	ID       int64  `json:"id"`
	HostID   int64  `json:"host_id"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Email    string `json:"email"`

	UserID    int64      `json:"-"`
	CreatedAt *time.Time `json:"-"`
	UpdatedAt *time.Time `json:"-"`
}

func SerializeRegistry(r *registry.Registry) *Registry {
	if r == nil {
		return nil
	}
	return &Registry{
		ID:       r.ID,
		HostID:   r.HostID,
		URL:      r.URL,
		Username: r.Username,
		Email:    r.Email,
	}
}

func SerializeRegistries(registries []*registry.Registry) []*Registry {
	ret := make([]*Registry, len(registries))
	for i, r := range registries {
		ret[i] = SerializeRegistry(r)
	}
	return ret
}
