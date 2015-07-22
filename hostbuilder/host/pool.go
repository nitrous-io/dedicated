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
	Stop()
}

type DockerPool struct {
	MaxHosts      int
	Driver        Host
	CheckInterval time.Duration

	quit chan struct{}
}

func NewPool(driverName string, driverOptions map[string]string) (Pool, error) {
	driver, err := NewDriver(driverName, driverOptions, "{}")
	if err != nil {
		return nil, err
	}

	switch driverName {
	case "docker":
		return &DockerPool{
			Driver:        driver,
			MaxHosts:      1,
			CheckInterval: 2 * time.Second,
			quit:          make(chan struct{}),
		}, nil
		//case "amazon":
	}

	return nil, fmt.Errorf("pool: unsupported driver %q", driverName)
}

func (p *DockerPool) Acquire() (string, error) {
	return "", nil
}

func (p *DockerPool) Release(id string) error {
	return nil
}

func (p *DockerPool) currentNumberOfHosts() (int, error) {
	// Count number of Docker containers that have label "free=true".
	return p.Driver.Count(map[string][]string{"label": []string{"free=true"}})
}

func (p *DockerPool) Run() error {
	ticker := time.NewTicker(p.CheckInterval)
	for {
		select {
		case <-p.quit:
			logrus.Debugf("DockerPool: quitting")
			ticker.Stop()
			close(p.quit)
			return nil
		case <-ticker.C:
			n, err := p.currentNumberOfHosts()
			if err != nil {
				logrus.Infof("Failed to get count: %+v", err)
			}
			switch {
			case n < p.MaxHosts:
				logrus.Infof("create more instances in the pool")

			case n > p.MaxHosts:
				logrus.Infof("terminate extra hosts in pool")
			default:
				logrus.Info("do nothing")
			}
		}
	}

	return nil
}

func (p *DockerPool) Stop() {
	p.quit <- struct{}{}
	<-p.quit
}
