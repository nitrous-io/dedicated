package serializers

import "github.com/nitrous-io/nitrous/models/container"

type Container struct {
	ID        int64  `json:"id"`
	CID       string `json:"cid"`
	UserID    int64  `json:"user_id"`
	Image     string `json:"image"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	State     string `json:"state"`

	Host *Host `json:"host"`
}

func SerializeContainer(c *container.Container) *Container {
	return &Container{
		ID:        c.ID,
		CID:       c.CID,
		UserID:    c.UserID,
		Image:     c.Image,
		Slug:      c.Slug,
		Name:      c.Name.String,
		IPAddress: c.IPAddress.String,
		State:     c.State,
		Host:      SerializeHost(c.Host, nil),
	}
}
