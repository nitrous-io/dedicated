package main

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	Usage = `
	./hostrename <staging|production> <host info csv> [--live-run]

	Get the host info csv by running
	\copy (SELECT hosts.id, slug, provider_identifier, config -> 'region_id' as region FROM hosts join provider_templates pts ON hosts.provider_template_id = pts.id WHERE pts.provider = 'amazon' AND hosts.terminated_at IS NULL AND hosts.provider_identifier IS NOT NULL ORDER BY region, hosts.id) TO '/tmp/hosts-to-rename.csv' CSV HEADER;
`
)

var (
	dryrun = true

	Regions = map[string]*ec2.EC2{}
)

type Instance struct {
	HostID, InstanceID, Name string
}

// TODO: dryRun should be passed as input so we can check for permission to
// create tags
func setName(conn *ec2.EC2, instanceID, name string) error {
	logrus.Printf("Naming instance ID %s as %s", instanceID, name)
	if dryrun {
		return nil
	}
	_, err := conn.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{aws.String(instanceID)},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(name),
			},
		},
	})
	return err
}

func parseCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 4
	if _, err := r.Read(); err != nil { // headers
		return nil, err
	}
	return r.ReadAll()
}

func init() {
	accessKey := os.Getenv("AWS_ACCESS_KEY")
	secretKey := os.Getenv("AWS_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		logrus.Fatal("Please set AWS_ACCESS_KEY and AWS_SECRET_KEY in environment")
	}
	credentials := credentials.NewStaticCredentials(accessKey, secretKey, "")

	for _, region := range []string{
		"ap-southeast-1",
		"eu-west-1",
		"us-east-1",
		"us-west-2",
	} {
		Regions[region] = ec2.New(&aws.Config{Region: region, Credentials: credentials})
	}
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
	env := os.Args[1]
	if env != "staging" && env != "production" {
		logrus.Fatalf("Wrong environment\n%s\n", Usage)
	}

	csvPath := os.Args[2]
	rows, err := parseCSV(csvPath)
	if err != nil {
		logrus.Fatalf("Error parsing csv: %s", err)
	}

	for _, row := range rows {
		hostID, slug, instanceID, region := row[0], row[1], row[2], row[3]
		logrus.Printf("Row: hostID %s, slug %s, instanceID %s, region %s\n", hostID, slug, instanceID, region)
		newName := fmt.Sprintf("userhost-%s-%s", env, hostID)
		if ec2Region, ok := Regions[region]; ok {
			if err := setName(ec2Region, instanceID, newName); err != nil {
				logrus.Errorf("Setting name for hostID %s: err %s", hostID, err)
			}
		} else {
			logrus.Warnf("Region %s not found", region)
		}
	}
}
