package host

import (
	"fmt"
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
	Driver   Host

	// FIXME Add channel to cancel run() goroutine.
}

func NewPool(driverName string, driverOptions map[string]string) (Pool, error) {
	driver, err := NewDriver(driverName, driverOptions, "{}")
	if err != nil {
		return nil, err
	}

	switch driverName {
	case "docker":
		return &DockerPool{Driver: driver, MaxHosts: 1}, nil
		//case "amazon":
	}

	return nil, fmt.Errorf("pool: unsupported driver %q", driverName)
}

func (m *DockerPool) Acquire() (string, error) {
	return "", nil
}

func (m *DockerPool) Release(id string) error {
	return nil
}

func (m *DockerPool) currentNumberOfHosts() (int, error) {
	// Count number of Docker containers that have label "free=true".
	return m.Driver.Count(map[string][]string{"label": []string{"free=true"}})
}

func (m *DockerPool) Run() error {
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
