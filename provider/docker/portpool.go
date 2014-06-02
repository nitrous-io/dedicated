package docker

import (
	"errors"
	"fmt"
	"sync"

	"github.com/nitrous-io/freighter/datastore"
)

var (
	ErrNoFreePort = errors.New("docker: no free port left in port pool")
	mutex         = &sync.Mutex{}
)

type PortPool struct {
	store *datastore.Datastore
}

func NewPortPool(store *datastore.Datastore) *PortPool {
	return &PortPool{store: store}
}

func (self *PortPool) Acquire() (uint16, error) {
	mutex.Lock()
	var port uint16
	kv, err := self.store.FindOneWhere("port:%", "0")
	if err == nil {
		if kv == nil {
			err = ErrNoFreePort
		} else {
			err = self.store.Set(kv.Key, "1")
			if err == nil {
				fmt.Sscanf(kv.Key, "port:%d", &port)
			}
		}
	}
	mutex.Unlock()
	return port, err
}

func (self *PortPool) Release(port uint16) error {
	err := self.store.Update(fmt.Sprintf("port:%v", port), "0")
	return err
}
