package dummy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
)

type Host struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	IPAddress        string `json:"ip_address"`
	FreighterdPort   int64  `json:"freighterd_port"`
	DestroySuccess   bool   `json:"destroy"`
	CreateSuccess    bool   `json:"create"`
	UpdateSuccess    bool   `json:"update"`
	StopSuccess      bool   `json:"stop"`
	StartSuccess     bool   `json:"start"`
	RestartSuccess   bool   `json:"restart"`
	ListSuccess      bool   `json:"list"`
	ListPartialError bool   `json:"list_partial"`

	Version string
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

func (h *Host) CreateWithQueue(qp *queue.QueuePublisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	if !h.CreateSuccess {
		return nil, errors.New("failed to create host")
	}

	msg := &queue.Message{
		Action:       queue.Create.String(),
		Event:        string(event.HostCreated),
		ResourceName: host.EventResourceName,
		ResourceID:   h.ID,
	}
	err := qp.Publish(msg)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to send a message: %+v", err))
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
		Region:         "dummy",
		Name:           h.Name,
		IPAddress:      h.IPAddress,
		FreighterdPort: uint16(h.FreighterdPort),
		State:          state.Running,
	}, nil
}

func (h *Host) Stop() (*host.Info, error) {
	if h.StopSuccess {
		return h.Info()
	}
	return nil, errors.New("Failed to stop host")
}

func (h *Host) Start() (*host.Info, error) {
	if h.StartSuccess {
		return h.Info()
	}
	return nil, errors.New("Failed to start host")
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
	var err error
	if h.ListPartialError {
		err = errors.New("not enough dummies")
	}
	return []*host.Info{
		{
			Provider:       h.DriverName(),
			ID:             "abcd",
			Region:         "dummy",
			Name:           "ABCD",
			IPAddress:      "1.2.3.4",
			FreighterdPort: uint16(60001),
			State:          state.Running,
		},
		{
			Provider:       h.DriverName(),
			ID:             "bcde",
			Region:         "dummy",
			Name:           "BCDE",
			IPAddress:      "1.2.3.5",
			FreighterdPort: uint16(60002),
			State:          state.Running,
		},
	}, err
}

func (h *Host) DriverName() string {
	return "dummy"
}

func (h *Host) Settings() *host.Settings {
	return host.NewSettings(
		&host.TLSConfig{CA: "ca", Cert: "cert", Key: "key"},
		&host.AptConfig{AptUsername: "distro", AptPassword: "passwd", AptLocation: "master", AptDist: "precise"},
		"/home/action",
		"http://google.com",
		"secret",
		h.Version,
		[]string{"pubkey1", "pubkey2"},
	)
}

func (h *Host) SSHConfig() *host.SSHConfig {
	key, _ := sshutil.GenerateSSHKey()
	return &host.SSHConfig{
		IPAddress:  "10.0.0.1",
		Port:       6222,
		Username:   "action",
		PrivateKey: key,
	}
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
