package route53

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/route53"
	"github.com/nitrous-io/dedicated/compass/navigator"
)

var (
	awsAccess   string
	awsSecret   string
	zonesCached []route53.HostedZone
	mu          sync.Mutex

	awsRegion aws.Region = aws.USWest
)

// SetSecrets must be called before any other call
func SetSecrets(access, secret string) {
	mu.Lock()
	defer mu.Unlock()
	awsAccess = access
	awsSecret = secret
}

// delete CNAME record for a given hostName
// One would think that I could just sent DELETE request with hostName but
// it seems that to delete a record DELETE has to send exactly the same
// values that it currently has so the reliable way seems to be: get current
// info and then delete them
func deleteCNAMERecord(c *route53.Route53, zoneID, hostName string) error {
	hostName = route53.FQDN(hostName)
	// https://docs.aws.amazon.com/Route53/latest/APIReference/API_ListResourceRecordSets.html
	opts := route53.ListOpts{
		Name:     hostName, // first name in the lexicographic ordering of domain names to be retrieved
		MaxItems: 1,        // only retrieve the name we want
	}
	resp, err := c.ListResourceRecordSets(zoneID, &opts)
	if err != nil {
		return err
	}
	var changes []route53.Change
	for _, r := range resp.Records {
		if !strings.EqualFold(r.Type, "cname") {
			continue
		}
		if r.Name != hostName {
			continue
		}
		r.RecordsXML = "" // need to blank out in order to delete
		changes = append(changes, route53.Change{
			Action: "DELETE",
			Record: r,
		})
	}
	if len(changes) == 0 { // nothing to delete
		fmt.Printf("[DEBUG] deleteCNAMERecord for hostName %s: no record found", hostName)
		return nil
	}
	if len(changes) > 1 {
		return fmt.Errorf("Expected one change for hostname %s, got %d: %+v", hostName, len(changes), changes)
	}

	fmt.Printf("[DEBUG] deleteCNAMERecord for hostName %s: changes are %+v\n", hostName, changes)
	req := &route53.ChangeResourceRecordSetsRequest{
		Comment: fmt.Sprintf("deleting DNS record for %s", hostName),
		Changes: changes,
	}
	if _, err := c.ChangeResourceRecordSets(zoneID, req); err != nil {
		return err
	}
	return nil
}

func setDNS(c *route53.Route53, zoneID, typ, from, to string) error {
	change := route53.Change{
		Action: "UPSERT", // create or update
		Record: route53.ResourceRecordSet{
			Name:    route53.FQDN(from),
			Type:    typ,
			TTL:     60,
			Records: []string{to},
		},
	}

	req := &route53.ChangeResourceRecordSetsRequest{
		Changes: []route53.Change{change},
	}

	_, err := c.ChangeResourceRecordSets(zoneID, req)
	if err != nil {
		fmt.Printf("c.ChangeResourceRecordSets() failed with %s\n", err)
		return err
	}
	return nil
}

func findZone(domain string) *route53.HostedZone {
	domain = strings.TrimSuffix(domain, ".")
	mu.Lock()
	defer mu.Unlock()
	for _, hz := range zonesCached {
		name := strings.TrimSuffix(hz.Name, ".")
		if strings.EqualFold(name, domain) {
			return &hz
		}
	}
	return nil
}

func cacheZonesIfNecessary(c *route53.Route53) error {
	mu.Lock()
	hasZones := zonesCached != nil
	mu.Unlock()

	if hasZones {
		return nil
	}
	// FIXME API only supports up to a maximum of 100 zones, so we should
	// iterate.
	// See https://docs.aws.amazon.com/Route53/latest/APIReference/API-list-hosted-zones-private.html.
	resp, err := c.ListHostedZones("", 100)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	zonesCached = resp.HostedZones
	return nil
}

// find zoneID for a given host name
func getClientAndZoneIDFromHostName(hostName string) (*route53.Route53, string, error) {
	mu.Lock()
	auth, err := aws.GetAuth(awsAccess, awsSecret)
	mu.Unlock()

	if err != nil {
		return nil, "", err
	}
	client := route53.New(auth, awsRegion)
	err = cacheZonesIfNecessary(client)
	if err != nil {
		return nil, "", err
	}
	domain := navigator.RootDomain(hostName)
	zone := findZone(domain)
	if zone == nil {
		return nil, "", fmt.Errorf("no zone for domain '%s' (hostName: '%s')", domain, hostName)
	}
	return client, zone.ID, nil
}

// SetCNAME adds or updates CNAME record for a given zoneID that maps src into dst
func SetCNAME(from, to string) error {
	c, zoneID, err := getClientAndZoneIDFromHostName(from)
	if err != nil {
		return err
	}
	return setDNS(c, zoneID, "CNAME", from, to)
}

// DeleteCNAMERecord deletes CNAME or A record (only one of them can
// exist at any given time for a given host name)
func DeleteCNAMERecord(hostName string) error {
	c, zoneID, err := getClientAndZoneIDFromHostName(hostName)
	if err != nil {
		return err
	}
	return deleteCNAMERecord(c, zoneID, hostName)
}
