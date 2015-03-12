package amazon

import (
	"github.com/mitchellh/goamz/ec2"
)

func (h *Host) ReloadInstance(id string) error {
	return h.reloadInstance(id)
}

func (h *Host) GetInstance() *ec2.Instance {
	return h.instance
}
