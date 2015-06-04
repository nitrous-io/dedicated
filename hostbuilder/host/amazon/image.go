package amazon

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type imageProvider interface {
	DescribeImages(*ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error)
}

func getImage(p imageProvider, id string) (*ec2.Image, error) {
	res, err := p.DescribeImages(&ec2.DescribeImagesInput{
		ImageIDs: []*string{aws.String(id)},
	})
	if err != nil {
		return nil, fmt.Errorf("amazon: failed to list images %s", err)
	}
	if len(res.Images) == 0 {
		return nil, fmt.Errorf("amazon: image %q not found", id)
	}
	return res.Images[0], nil
}
