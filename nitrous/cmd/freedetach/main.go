package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
)

const (
	Usage = `
	./freedetach <path to csv> [--live-run]

	Obtain csv with the following command:

	\copy (SELECT id, slug, ip_address, provider_identifier, region_identifier FROM hosts WHERE static_ip = true AND state = 'stopped' AND terminated_at is null AND plan_id IN (7, 8, 9)) TO '/tmp/free-detach.csv' CSV HEADER;

	We only release EIP of stopped free hosts so that the IP address doesn't change while the host is in use.
`
)

var (
	dryrun = true

	auth aws.Auth

	Regions = map[string]*ec2.EC2{}

	// map of maps
	// (region name) -> (ip address -> ec2 Address obj)
	Addresses = map[string]map[string]ec2.Address{}
)

func parseCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 5
	if _, err := r.Read(); err != nil { // headers
		return nil, err
	}
	return r.ReadAll()
}

func init() {
	auth.AccessKey = os.Getenv("AWS_ACCESS_KEY")
	auth.SecretKey = os.Getenv("AWS_SECRET_KEY")

	if auth.AccessKey == "" || auth.SecretKey == "" {
		logrus.Fatal("Please set AWS_ACCESS_KEY and AWS_SECRET_KEY in environment")
	}

	Regions[aws.APSoutheast.Name] = ec2.New(auth, aws.APSoutheast)
	Regions[aws.EUWest.Name] = ec2.New(auth, aws.EUWest)
	Regions[aws.USEast.Name] = ec2.New(auth, aws.USEast)
	Regions[aws.USWest2.Name] = ec2.New(auth, aws.USWest2)
}

func populateAddresses() error {
	for rname, ec2r := range Regions {
		filter := ec2.NewFilter()
		filter.Add("domain", "vpc")

		Addresses[rname] = make(map[string]ec2.Address)

		addrs, err := ec2r.Addresses(nil, nil, filter)
		if err != nil {
			return err
		}
		for _, addr := range addrs.Addresses {
			Addresses[rname][addr.PublicIp] = addr
		}
	}
	return nil
}

// cribbed from hostbuilder/host/amazon/elasticip.go#releaseAddress
// returns host id if released, otherwise empty string
// absence of error does not imply one EIP released -- for example
// if the address was not found, we just give a warning
func release(fields logrus.Fields) (string, error) {
	hostID, ok := fields["host_id"].(string)
	if !ok {
		return "", fmt.Errorf("host_id is not a string: %+v", fields)
	}

	addr, ok := fields["addr"].(string)
	if !ok {
		return "", fmt.Errorf("addr is not a string: %+v", fields)
	}
	region, ok := fields["region"].(string)
	if !ok {
		return "", fmt.Errorf("region is not a string: %+v", fields)
	}
	instanceID, ok := fields["instance_id"].(string)
	if !ok {
		return "", fmt.Errorf("instanceID is not a string: %+v", fields)
	}

	amap, ok := Addresses[region]
	if !ok {
		return "", fmt.Errorf("region not in address map: %s", region)
	}

	ec2addr, ok := amap[addr]
	if !ok {
		logrus.WithFields(fields).Warnf("Associated address not found")
		return hostID, nil
	}

	// Sanity check: make sure EIP is associated with the instance the DB says it is
	if ec2addr.InstanceId != instanceID {
		return "", fmt.Errorf("Address associated with different instance: %s", ec2addr.InstanceId)
	}

	logrus.WithFields(fields).Infof("Disassociating %s from instance %s", ec2addr.AssociationId, ec2addr.InstanceId)
	if !dryrun {
		if _, err := Regions[region].DisassociateAddress(ec2addr.AssociationId); err != nil {
			return "", err
		}
	}

	logrus.WithFields(fields).Infof("Releasing %s", ec2addr.AllocationId)
	if !dryrun {
		if _, err := Regions[region].ReleaseAddress(ec2addr.AllocationId); err != nil {
			return "", err
		}
	}
	return hostID, nil
}

func main() {
	argMin := 1
	if os.Args[len(os.Args)-1] == "--live-run" {
		dryrun = false
		argMin = 2
		fmt.Println("Live Run!")
	}

	if len(os.Args)-1 < argMin {
		logrus.Fatalf("Missing arguments\n%s\n", Usage)
	}

	csvPath := os.Args[1]
	rows, err := parseCSV(csvPath)
	if err != nil {
		logrus.Fatalf("Error parsing csv: %s", err)
	}

	if err := populateAddresses(); err != nil {
		logrus.Fatalf("Error populating addresses: %s", err)
	}

	actioned := []string{} // host IDs released

	for _, row := range rows {
		hostID, slug, addr, instanceID, region := row[0], row[1], row[2], row[3], row[4]
		fields := logrus.Fields{
			"host_id":     hostID,
			"instance_id": instanceID,
			"slug":        slug,
			"addr":        addr,
			"region":      region,
		}
		logrus.WithFields(fields).Info("Processing")
		if _, ok := Regions[region]; ok {
			hid, err := release(fields)
			if err != nil {
				logrus.WithFields(fields).Errorf("Error releasing: %v", err)
				continue
			}
			actioned = append(actioned, hid)
		} else {
			logrus.Warnf("Region %s not found", region)
		}
	}
	logrus.Infof("%d addresses released out of %d", len(actioned), len(rows))
	logrus.Infof("Host ids: %s", strings.Join(actioned, ", "))
}
