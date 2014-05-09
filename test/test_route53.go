package main

import (
	"log"
	"os"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/route53"
)

const (
	accessKey = ""
	secretKey = ""
	domain    = ""
	ip        = ""
	zoneId    = ""
)

func main() {
	auth, err := aws.GetAuth(accessKey, secretKey)
	if err != nil {
		log.Println("failed to get aws auth")
		os.Exit(1)
	}
	router := route53.NewRoute53(auth)

	recordSet := &route53.ChangeResourceRecordSetsRequest{
		Action: "CREATE",
		Name:   domain,
		Type:   "A",
		Value:  ip,
	}
	resp, err := router.ChangeResourceRecordSet(recordSet, zoneId)
	log.Println(resp, err)
}
