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
	awsRegion   aws.Region = aws.USWest
	mu          sync.Mutex
	zonesCached []route53.HostedZone
)

// SetSecrets must be called before any other call
func SetSecrets(access, secret string) {
	mu.Lock()
	defer mu.Unlock()
	awsAccess = access
	awsSecret = secret
}

func isCNAMEOrA(tp string) bool {
	return strings.EqualFold(tp, "cname") || strings.EqualFold(tp, "a")
}

// delete all CNAME and A records for a given hostName
// One would think that I could just sent DELETE request with hostName but
// it seems that to delete a record DELETE has to send exactly the same
// values that it currently has so the reliable way seems to be: get current
// info and then delete them
func deleteExistingDNSRecords(c *route53.Route53, zoneID, hostName string) error {
	hostName = route53.FQDN(hostName)
	opts := route53.ListOpts{
		Name:     hostName,
		MaxItems: 10000,
	}
	resp, err := c.ListResourceRecordSets(zoneID, &opts)
	if err != nil {
		return err
	}
	var changes []route53.Change
	for _, r := range resp.Records {
		if isCNAMEOrA(r.Type) {
			r.RecordsXML = "" // not
			change := route53.Change{
				Action: "DELETE",
				Record: r,
			}
			changes = append(changes, change)
		}
	}
	// nothing to delete
	if len(changes) == 0 {
		return nil
	}
	req := &route53.ChangeResourceRecordSetsRequest{
		Comment: "deleting DNS record",
		Changes: changes,
	}
	_, err = c.ChangeResourceRecordSets(zoneID, req)
	if err != nil {
		return err
	}
	return nil
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

func setDNS(c *route53.Route53, zoneID, typ, from, to string) error {
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

func setCNAME(c *route53.Route53, zoneID, from, to string) error {
	return setDNS(c, zoneID, "CNAME", from, to)
}

func setA(c *route53.Route53, zoneID, from, to string) error {
	return setDNS(c, zoneID, "A", from, to)
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
	resp, err := c.ListHostedZones("", 10000)
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

// CheckZonesExist returns an error if route53 doesn't have a zone for one of
// the domains. Should be called at startup for sanity checking
func CheckZonesExist(domains []string) error {
	for _, domain := range domains {
		_, _, err := getClientAndZoneIDFromHostName(domain)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetCNAME adds or updates CNAME record for a given zoneID that maps src into dst
func SetCNAME(from, to string) error {
	c, zoneID, err := getClientAndZoneIDFromHostName(from)
	if err != nil {
		return err
	}
	return setCNAME(c, zoneID, from, to)
}

// SetA adds or updates A record for a given zoneID that maps src into dst
func SetA(from, to string) error {
	c, zoneID, err := getClientAndZoneIDFromHostName(from)
	if err != nil {
		return err
	}
	return setA(c, zoneID, from, to)
}

// DeleteExistingDNSRecords deletes CNAME or A record (only one of them can
// exist at any given time for a given host name)
func DeleteExistingDNSRecords(hostName string) error {
	c, zoneID, err := getClientAndZoneIDFromHostName(hostName)
	if err != nil {
		return err
	}
	return deleteExistingDNSRecords(c, zoneID, hostName)
}
