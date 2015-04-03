package amazon

import (
	"fmt"

	"github.com/mitchellh/goamz/ec2"
)

type imageProvider interface {
	Images(ids []string, filter *ec2.Filter) (*ec2.ImagesResp, error)
}

func getImage(p imageProvider, id string) (*ec2.Image, error) {
	res, err := p.Images([]string{id}, ec2.NewFilter())
	if err != nil {
		return nil, fmt.Errorf("amazon: failed to list images %s", err)
	}
	if len(res.Images) == 0 {
		return nil, fmt.Errorf("amazon: image %q not found", id)
	}
	return &res.Images[0], nil
}
