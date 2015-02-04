package dummy

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
)

type Host struct {
	ID             string `json:"id"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int64  `json:"freighterd_port"`
	DestroySuccess bool   `json:"destroy"`
	CreateSuccess  bool   `json:"create"`
	UpdateSuccess  bool   `json:"update"`
	RestartSuccess bool   `json:"restart"`
	ListSuccess    bool   `json:"list"`
}

var ReceivedNewHostDefaultConfig map[string]string

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	if !h.CreateSuccess {
		return nil, errors.New("failed to create host")
	}
	return h.Info()
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	if !h.UpdateSuccess {
		return nil, errors.New("failed to update host")
	}
	return h.Info()
}

func (h *Host) Destroy() error {
	if h.DestroySuccess {
		return nil
	}
	return errors.New("Failed to destroy host")
}

func (h *Host) Info() (*host.Info, error) {
	if h.FreighterdPort == 0 {
		h.FreighterdPort = 65333
	}
	return &host.Info{
		Provider:       h.DriverName(),
		ID:             h.ID,
		IPAddress:      h.IPAddress,
		FreighterdPort: uint16(h.FreighterdPort),
		State:          state.Running,
	}, nil
}

func (h *Host) Restart() (*host.Info, error) {
	if h.RestartSuccess {
		return h.Info()
	}
	return nil, errors.New("Failed to restart host")
}

func (h *Host) List() ([]*host.Info, error) {
	if !h.ListSuccess {
		return nil, errors.New("Failed to list hosts")
	}
	return []*host.Info{
		{
			Provider:       h.DriverName(),
			ID:             "abcd",
			IPAddress:      "1.2.3.4",
			FreighterdPort: uint16(60001),
			State:          state.Running,
		},
		{
			Provider:       h.DriverName(),
			ID:             "bcde",
			IPAddress:      "1.2.3.5",
			FreighterdPort: uint16(60002),
			State:          state.Running,
		},
	}, nil
}

func (h *Host) DriverName() string {
	return "dummy"
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	ReceivedNewHostDefaultConfig = defaultConfig
	var host Host
	json.Unmarshal([]byte(config), &host)
	return &host, nil
}

func init() {
	host.Hosts["dummy"] = NewHost
}
