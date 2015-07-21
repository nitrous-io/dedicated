package host

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
)

type Pool interface {
	Acquire() (id string, err error)
	Release(id string) error
	Run() error
}

type DockerPool struct {
	MaxHosts int
	Provider Host

	// FIXME Add channel to cancel run() goroutine.
}

func NewPool(providerName string, providerConfig map[string]string) (Pool, error) {
	maxHosts, err := strconv.ParseInt(providerConfig["max_hosts"], 10, 64)
	if err != nil {
		return nil, err
	}

	provider, err := InitHost(providerName, providerConfig, "{}")
	if err != nil {
		return nil, err
	}

	switch providerName {
	case "docker":
		return &DockerPool{Provider: provider, MaxHosts: int(maxHosts)}, nil
		//case "amazon":
	}

	return nil, fmt.Errorf("unsupported provider %s", provider)
}

func (m *DockerPool) Acquire() (string, error) {
	return "", nil
}

func (m *DockerPool) Release(id string) error {
	return nil
}

func (m *DockerPool) currentNumberOfHosts() (int, error) {
	// Get list of docker cotnainer that have label free=true
	return m.Provider.Count(map[string][]string{"label": []string{"free", "true"}})
}

func (m *DockerPool) Run() error {
	logrus.Info("run")
	for {
		n, err := m.currentNumberOfHosts()
		if err != nil {
			logrus.Infof("Failed to get count: %+v", err)
		}
		switch {
		case n < m.MaxHosts:
			logrus.Infof("create more instances in the pool")

		case n > m.MaxHosts:
			logrus.Infof("terminate extra hosts in pool")
		default:
			logrus.Info("do nothing")
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}
