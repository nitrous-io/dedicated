package main

import (
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
)

func awsConnect() {
	var auth aws.Auth

	auth.AccessKey = appConfig.AccessKey
	auth.SecretKey = appConfig.SecretKey

	for _, r := range appConfig.Regions {
		println(r.ID)
		ec2conns[r.ID] = ec2.New(auth, aws.Regions[r.ID])
	}
}
