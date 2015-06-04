package serializers

import (
	"regexp"
	"time"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
)

var (
	ignoredVolumeSuffixes = regexp.MustCompile(`:/(\.nitrous-container-slug|nitrous\/ssh):ro\z`)
)

type Container struct {
	ID              int64            `json:"id"`
	CID             string           `json:"cid"`
	UserID          int64            `json:"user_id"`
	HostID          int64            `json:"host_id"`
	Image           string           `json:"image"`
	Slug            string           `json:"slug"`
	Name            string           `json:"name,omitempty"`
	IPAddress       string           `json:"ip_address,omitempty"`
	GatewayIP       string           `json:"gateway_ip,omitempty"`
	State           string           `json:"state"`
	User            string           `json:"user"`
	Cmd             []string         `json:"cmd,omitempty"` // TODO deprecate
	Entrypoint      []string         `json:"entrypoint,omitempty"`
	ExitCode        int              `json:"exit_code"`
	Config          *ContainerConfig `json:"config,omitempty"`
	PreviewHostname string           `json:"preview_hostname"`
	SSHURL          string           `json:"ssh_url,omitempty"`

	Metadata map[string]string `json:"metadata"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

type SharedContainer struct {
	Container

	UserUsername     string `json:"user_username"`
	UserName         string `json:"user_name"`
	UserGravatarHash string `json:"user_gravatar_hash"`

	Host *Host `json:"host"`
}

func SerializeContainer(c *container.Container) *Container {
	con := &Container{
		ID:              c.ID,
		CID:             c.CID,
		UserID:          c.UserID,
		HostID:          c.HostID,
		Image:           c.Image,
		Slug:            c.Slug,
		Name:            c.Name.String,
		IPAddress:       c.IPAddress,
		GatewayIP:       c.GatewayIP,
		State:           c.ResolvedState().String(),
		Cmd:             c.Cmd,
		User:            c.User,
		Entrypoint:      c.Entrypoint,
		ExitCode:        c.ExitCode,
		PreviewHostname: c.PreviewHostname(),
		Metadata:        map[string]string{},
		StartedAt:       c.StartedAt,
		FinishedAt:      c.FinishedAt,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
		TerminatedAt:    c.TerminatedAt,
	}

	for key, value := range c.Metadata.Map {
		if value.Valid {
			con.Metadata[key] = value.String
		}
	}

	return con
}

type ContainerConfig struct {
	Cmd                []string `json:"cmd"`
	Env                []string `json:"env"`
	Ports              []string `json:"port"`
	ConfigPortBindings []string `json:"config_port_bindings"`
	Mounts             []string `json:"volume"`
	Privileged         bool     `json:"privileged"`
	PublishAll         bool     `json:"publish_all"`
}

func SerializeContainerConfig(cfg *freighterclient.ContainerConfig) *ContainerConfig {
	filterContainerConfig(cfg)
	c := &ContainerConfig{
		Cmd:                cfg.Cmd,
		Env:                cfg.Env,
		Ports:              cfg.PortBindings,
		ConfigPortBindings: cfg.ConfigPortBindings,
		Mounts:             cfg.Mounts,
	}
	if cfg.Privileged != nil {
		c.Privileged = *cfg.Privileged
	}
	if cfg.PublishAll != nil {
		c.PublishAll = *cfg.PublishAll
	}
	return c
}

func filterContainerConfig(cl *freighterclient.ContainerConfig) {
	mounts := []string{}
	for _, mnt := range cl.Mounts {
		if !ignoredVolumeSuffixes.MatchString(mnt) {
			mounts = append(mounts, mnt)
		}
	}
	cl.Mounts = mounts
}

type BySlug []*Container

func (c BySlug) Len() int           { return len(c) }
func (c BySlug) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c BySlug) Less(i, j int) bool { return c[i].Slug < c[j].Slug }
