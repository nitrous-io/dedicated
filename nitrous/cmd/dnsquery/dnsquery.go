package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/route53"
	nroute53 "github.com/nitrous-io/dedicated/nitrous/pkg/route53"
)

/*
A helper for inspecting dns in route 53. Basic usage:

godep go run nitrous/cmd/dnsquery/dnsquery.go domain
e.g.:
godep go run nitrous/cmd/dnsquery/dnsquery.go n2ostagingbox.com

Lists all DNS records for a given domain.

Flag -test-create is for testing cname/a record creation/deletion.

Can als be used for easier testing of changes to nitrous/pkg/route53
*/

const (
	hostDomain    = "n2ostagingbox.com"
	proxyDomain   = "nitros.io"
	previewDomain = "n2odev.com"
)

var (
	hostZone    *route53.HostedZone
	proxyZone   *route53.HostedZone
	previewZone *route53.HostedZone
	client      *route53.Route53
)

func dumpZone(hz *route53.HostedZone, name string) {
	if hz == nil {
		log.Fatalf("missing a zone with name '%s'", name)
	}
	fmt.Printf("Hosted zone with id '%s'\n  name: '%s'\n  caller reference: '%s'\n  resource count: %d\n", hz.ID, hz.Name, hz.CallerReference, hz.ResourceCount)
}

func testHostListedZones(c *route53.Route53) {
	fmt.Printf("testHostListedZones()\n")
	resp, err := c.ListHostedZones("", 10000)
	if err != nil {
		log.Fatalf("c.ListHostedZones() failed with %s\n", err)
	}
	if resp.IsTruncated {
		fmt.Printf("response is truncated, NextMarker: '%s'\n", resp.NextMarker)
	}
	fmt.Printf("got %d records\n", len(resp.HostedZones))
	for _, hz := range resp.HostedZones {
		name := strings.TrimSuffix(hz.Name, ".")
		hzCopy := hz
		switch name {
		case hostDomain:
			hostZone = &hzCopy
		case proxyDomain:
			proxyZone = &hzCopy
		case previewDomain:
			previewZone = &hzCopy
		}
	}

	dumpZone(hostZone, hostDomain)
	dumpZone(proxyZone, proxyDomain)
	dumpZone(previewZone, previewDomain)
}

func findZone(c *route53.Route53, domain string) (*route53.HostedZone, error) {
	domain = strings.TrimSuffix(domain, ".")
	resp, err := c.ListHostedZones("", 10000)
	if err != nil {
		return nil, err
	}
	domain = strings.TrimSuffix(domain, ".")
	for _, hz := range resp.HostedZones {
		name := strings.TrimSuffix(hz.Name, ".")
		if strings.EqualFold(name, domain) {
			return &hz, nil
		}
	}
	return nil, nil
}

func buildSetChange(typ, from, to string) route53.Change {
	rr := route53.ResourceRecordSet{
		Name:    from,
		Type:    typ,
		TTL:     60,
		Records: []string{to},
	}

	change := route53.Change{
		Action: "UPSERT", // create or update
		Record: rr,
	}
	return change
}

func SetDNS(c *route53.Route53, zoneID, typ, from, to string) error {
	from = route53.FQDN(from)
	var changes = []route53.Change{
		buildSetChange(typ, from, to),
	}
	req := &route53.ChangeResourceRecordSetsRequest{
		Comment: "adding cname record",
		Changes: changes,
	}
	_, err := c.ChangeResourceRecordSets(zoneID, req)
	if err != nil {
		fmt.Printf("c.ChangeResourceRecordSets() failed with %s\n", err)
		return err
	}
	return nil
}

func isCNAMEOrA(tp string) bool {
	return strings.EqualFold(tp, "cname") || strings.EqualFold(tp, "a")
}

func shouldShowRecordWithType(tp string, types []string) bool {
	if len(types) == 0 {
		return true
	}
	for _, s := range types {
		if strings.EqualFold(s, tp) {
			return true
		}
	}
	return false
}

// types is a list of allowed types, like 'CNAME', 'SOA', 'NS', 'A' etc.
// if its nil, show all types
func dumpResourceRecordSet(r *route53.ResourceRecordSet, types []string) {
	if !shouldShowRecordWithType(r.Type, types) {
		return
	}
	fmt.Printf("Resource record:\n")
	fmt.Printf("  name: '%s'\n", r.Name)
	fmt.Printf("  type: '%s'\n", r.Type)
	fmt.Printf("   ttl: %d\n", r.TTL)
	for i := 0; i < len(r.Records); i++ {
		rec := r.Records[i]
		fmt.Printf(" rec %d: '%s'\n", i+1, rec)
	}
	if r.SetIdentifier != "" {
		fmt.Printf("  set identifier: '%s'\n", r.SetIdentifier)
	}
}

func testListRecourceRecordSets(c *route53.Route53, zoneID string, lopts *route53.ListOpts) {
	resp, err := c.ListResourceRecordSets(zoneID, lopts)
	if err != nil {
		log.Fatalf("c.ListResourceRecordSets('%s') failed with '%s'", zoneID, err)
	}
	if resp.IsTruncated {
		fmt.Printf("c.ListResourceRecordSets() response is truncated\n")
	}
	fmt.Printf("c.ListResourceRecordSets() returned %d records\n", len(resp.Records))
	for _, r := range resp.Records {
		dumpResourceRecordSet(&r, []string{"cname", "a"})
	}
}

func testDeleteNonExistent() {
	err := nroute53.DeleteExistingDNSRecords("does.not.exist." + hostDomain)
	if err != nil {
		fmt.Printf("deleting non-existent dns record returns error '%s'\n", err)
	} else {
		fmt.Printf("deleting non-existent dns record doesn't return an error\n")
	}
}

func testCreate() {
	testHostListedZones(client)
	//testDeleteNonExistent()
	//fmt.Printf("\nBefore adding www.example.com:\n")
	//testListRecourceRecordSets(client, hostZone.ID, nil)
	from := "test-for-cname-89345687.usw2." + hostDomain
	to := "sub.www.example.com"
	from2 := "test-for-a-1742986.usw2.n2ostagingbox.com"
	to2 := "127.0.0.1"
	nroute53.SetCNAME(from, to)
	nroute53.SetA(from2, to2)
	fmt.Printf("\nAfter adding %s:\n", from)
	testListRecourceRecordSets(client, hostZone.ID, nil)
	nroute53.DeleteExistingDNSRecords(from)
	nroute53.DeleteExistingDNSRecords(from2)
	fmt.Printf("\nAfter deleting A and CNAME records:\n")
	testListRecourceRecordSets(client, hostZone.ID, nil)
}

func usage() {
	fmt.Printf("Usage: dnsquery [-test-create] domain\n")
	fmt.Printf("  use -test-query to test create/delete cycle of test records.\n")
	fmt.Printf("  provide domain to see all records for that domain.\n")
}

func main() {
	var flgTestCreate bool
	flag.BoolVar(&flgTestCreate, "test-create", false, "test cretion/deletion of dummy ")
	flag.Parse()

	awsAccessKey := strings.TrimSpace(os.Getenv("AWS_ACCESS"))
	awsSecretKey := strings.TrimSpace(os.Getenv("AWS_SECRET"))
	if awsAccessKey == "" || awsSecretKey == "" {
		log.Fatalf("AWS_ACCESS or AWS_SECRET env variables not set")
	}
	nroute53.SetSecrets(awsAccessKey, awsSecretKey)

	auth, err := aws.GetAuth(awsAccessKey, awsSecretKey)
	if err != nil {
		log.Fatalf("aws.GetAuth() failed with %s", err)
	}
	client = route53.New(auth, aws.USWest)
	if flgTestCreate {
		testCreate()
		return
	}
	if len(flag.Args()) != 1 {
		usage()
		os.Exit(1)
	}
	domain := flag.Args()[0]
	zone, err := findZone(client, domain)
	if err != nil {
		log.Fatalf("findZone('%s') failed with '%s'\n", domain, err)
	}
	dumpZone(zone, domain)
	zoneID := zone.ID
	resp, err := client.ListResourceRecordSets(zoneID, nil)
	if err != nil {
		log.Fatalf("c.ListResourceRecordSets('%s') failed with '%s'", zoneID, err)
	}
	if resp.IsTruncated {
		fmt.Printf("c.ListResourceRecordSets() response is truncated\n")
	}
	fmt.Printf("c.ListResourceRecordSets() returned %d records\n", len(resp.Records))
	for _, r := range resp.Records {
		dumpResourceRecordSet(&r, nil)
	}
}
