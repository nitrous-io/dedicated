package main

// This command renames volumes according to the following convention
// - userdata-production-1234: the hotswappable data volume for host id 1234
// - userroot-production-3456: the non-hotswappable root volume for host id 3456
// Root volumes of hotswapped instances are left unnamed

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
)

const (
	Usage = `
	./volrename <staging|production> <host info csv> [--live-run]

	Get the host info csv by running
	\copy (SELECT hosts.id, slug, provider_identifier, provider_storage_identifier, region_identifier, hosts.hotswap FROM hosts join provider_templates pts ON hosts.provider_template_id = pts.id WHERE pts.provider = 'amazon' AND hosts.terminated_at IS NULL ORDER BY region, hosts.id) TO '/tmp/hosts-to-rename.csv' CSV HEADER;
`
)

var (
	dryrun = true
	env    string

	auth    aws.Auth
	Regions = map[string]*ec2.EC2{}
)

func parseCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 6
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

	logrus.SetLevel(logrus.DebugLevel)
}

type Host struct {
	ID, slug                  string
	instanceID, volID, region string
	hotswap                   bool
}

func rename(vol *ec2.Volume, h *Host) error {
	lf := logfields(vol, h)
	prefix := "userroot"
	if h.hotswap {
		prefix = "userdata"
	}
	name := fmt.Sprintf("%s-%s-%s", prefix, env, h.ID)

	nameTag := tagValue("Name", vol.Tags)
	volHostTag := tagValue("dedicated:host_id", vol.Tags)

	// new-style tagging
	if nameTag != "" {
		if nameTag == name {
			logrus.WithFields(lf).Debugf("Name is already correct")
			return nil
		}
		return fmt.Errorf("Expected Name tag '%s' to equal '%s'", nameTag, name)
	}

	if h.hotswap {
		if volHostTag != h.ID {
			return fmt.Errorf("Expected vol host_id tag '%s' to equal host id '%s'", volHostTag, h.ID)
		}
	} else {
		// volume should be attached to instance
		if len(vol.Attachments) == 0 {
			return fmt.Errorf("userroot volume not attached")
		}
		if vol.Attachments[0].InstanceId != h.instanceID {
			return fmt.Errorf("Expected vol attachment %s to equal provider_identifier %s", vol.Attachments[0].InstanceId, h.instanceID)
		}
	}

	logrus.WithFields(lf).Debugf("Renaming to %s", name)

	return setName(h.region, h.volID, name)
}

func logfields(vol *ec2.Volume, h *Host) logrus.Fields {
	lf := logrus.Fields{}
	if vol != nil {
		lf["vol_id"] = vol.VolumeId
	}
	if h != nil {
		lf["host_id"] = h.ID
		lf["slug"] = h.slug
		lf["region"] = h.region
		lf["instance_id"] = h.instanceID
	}
	return lf
}

func main() {
	argMin := 3
	if os.Args[len(os.Args)-1] == "--live-run" {
		dryrun = false
		argMin = 4
		fmt.Println("Live Run!")
	}

	if len(os.Args) < argMin {
		logrus.Fatalf("Missing arguments\n%s\n", Usage)
	}
	env = os.Args[1]
	if env != "staging" && env != "production" {
		logrus.Fatalf("Wrong environment\n%s\n", Usage)
	}

	vols, err := volumes()
	if err != nil {
		logrus.Fatal(err)
	}

	csvPath := os.Args[2]
	rows, err := parseCSV(csvPath)
	if err != nil {
		logrus.Fatalf("Error parsing csv: %s", err)
	}

	for _, row := range rows {
		h := &Host{
			ID:         row[0],
			slug:       row[1],
			instanceID: row[2],
			volID:      row[3],
			region:     row[4],
			hotswap:    row[5] == "t",
		}
		if vol, ok := vols[h.volID]; ok {
			if err := rename(&vol, h); err != nil {
				logrus.WithFields(logfields(&vol, h)).Errorf("error renaming: %v", err)
			}
		} else {
			logrus.WithFields(logfields(nil, h)).Warn("Volume not found")
		}
	}
}
