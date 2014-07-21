package controllers

import (
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
)

type JSONResponse struct {
	Code int
	Data map[string]interface{}
}

type ContainerAPI struct {
	ID        int64  `json:"id"`
	CID       string `json:"cid"`
	UserID    int64  `json:"user_id"`
	Image     string `json:"image"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	State     string `json:"state"`

	Host HostAPI `json:"host"`
}

type HostAPI struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int64  `json:"freighterd_port"`
}

func NewContainerAPI(c *container.Container, h *host.Host) ContainerAPI {
	return ContainerAPI{
		ID:        c.ID,
		CID:       c.CID,
		UserID:    c.UserID,
		Image:     c.Image,
		Slug:      c.Slug,
		Name:      c.Name.String,
		IPAddress: c.IPAddress.String,
		State:     c.State,

		Host: HostAPI{
			ID:             h.ID,
			Slug:           h.Slug,
			IPAddress:      h.IPAddress.String,
			FreighterdPort: h.FreighterdPort.Int64,
		},
	}
}
