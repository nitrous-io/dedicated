package serializers

import (
	"time"

	"github.com/nitrous-io/nitrous/models/container"
)

type Container struct {
	ID         int64    `json:"id"`
	CID        string   `json:"cid"`
	UserID     int64    `json:"user_id"`
	HostID     int64    `json:"host_id"`
	Image      string   `json:"image"`
	Slug       string   `json:"slug"`
	Name       string   `json:"name,omitempty"`
	IPAddress  string   `json:"ip_address,omitempty"`
	State      string   `json:"state"`
	Cmd        []string `json:"cmd,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
	ExitCode   int      `json:"exit_code"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   time.Time  `json:"finished_at"`
}

func SerializeContainer(c *container.Container) *Container {
	return &Container{
		ID:           c.ID,
		CID:          c.CID,
		UserID:       c.UserID,
		HostID:       c.HostID,
		Image:        c.Image,
		Slug:         c.Slug,
		Name:         c.Name.String,
		IPAddress:    c.IPAddress,
		State:        c.State,
		Cmd:          c.Cmd,
		Entrypoint:   c.Entrypoint,
		ExitCode:     c.ExitCode,
		StartedAt:    c.StartedAt,
		FinishedAt:   c.FinishedAt,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		TerminatedAt: c.TerminatedAt,
	}
}
