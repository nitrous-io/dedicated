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

// SetCNAME adds or updates a CNAME record that maps src into dst.
func SetCNAME(src, dst string) error {
	cl, err := newRoute53()
	if err != nil {
		return err
	}

	zone, err := getHostedZone(cl, src)
	if err != nil {
		return err
	}

	return setDNS(cl, zone.ID, "CNAME", src, dst)
}

// DeleteCNAMERecord deletes CNAME for a given domain.
func DeleteCNAMERecord(domain string) error {
	cl, err := newRoute53()
	if err != nil {
		return err
	}

	zone, err := getHostedZone(cl, domain)
	if err != nil {
		return err
	}

	return deleteCNAMERecord(cl, zone.ID, domain)
}

func deleteCNAMERecord(c *route53.Route53, zoneID, domain string) error {
	domain = route53.FQDN(domain)

	// Get resource record for domain so we can delete it.
	// https://docs.aws.amazon.com/Route53/latest/APIReference/API_ListResourceRecordSets.html
	opts := route53.ListOpts{
		Name:     domain,
		Type:     "CNAME",
		MaxItems: 1, // only retrieve the name we want
	}
	resp, err := c.ListResourceRecordSets(zoneID, &opts)
	if err != nil {
		return err
	}

	var changes []route53.Change
	for _, r := range resp.Records {
		if !strings.EqualFold(r.Type, "CNAME") {
			continue
		}
		if r.Name != domain {
			continue
		}
		r.RecordsXML = "" // need to blank out in order to delete
		changes = append(changes, route53.Change{
			Action: "DELETE",
			Record: r,
		})
	}
	if len(changes) == 0 { // nothing to delete
		fmt.Printf("[DEBUG] deleteCNAMERecord for domain %s: no record found", domain)
		return nil
	}
	if len(changes) > 1 {
		return fmt.Errorf("Expected one change for domain %s, got %d: %+v", domain, len(changes), changes)
	}

	fmt.Printf("[DEBUG] deleteCNAMERecord for domain %s: changes are %+v\n", domain, changes)
	req := &route53.ChangeResourceRecordSetsRequest{
		Comment: fmt.Sprintf("deleting CNAME record for %s", domain),
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

	if _, err := c.ChangeResourceRecordSets(zoneID, req); err != nil {
		fmt.Printf("c.ChangeResourceRecordSets() failed with %s\n", err)
		return err
	}

	return nil
}

func cacheZones(cl *route53.Route53) error {
	allZones := []route53.HostedZone{}
	marker := ""
	for {
		// API only returns up to a maximum of 100 zones per request, so we iterate.
		// See https://docs.aws.amazon.com/Route53/latest/APIReference/API-list-hosted-zones-private.html.
		resp, err := cl.ListHostedZones(marker, 100)
		if err != nil {
			return err
		}

		allZones = append(allZones, resp.HostedZones...)

		if !resp.IsTruncated {
			break
		}
		if resp.NextMarker == marker {
			// Sanity check to avoid infinite loop.
			break
		}

		marker = resp.NextMarker
	}

	mu.Lock()
	zonesCached = allZones
	mu.Unlock()

	return nil
}

func getHostedZone(cl *route53.Route53, domain string) (*route53.HostedZone, error) {
	if zonesCached == nil {
		if err := cacheZones(cl); err != nil {
			return nil, err
		}
	}

	rootDomain := navigator.RootDomain(route53.FQDN(domain))

	mu.Lock()
	defer mu.Unlock()
	for _, zone := range zonesCached {
		fqdn := route53.FQDN(zone.Name)
		if strings.EqualFold(fqdn, rootDomain) {
			return &zone, nil
		}
	}

	return nil, fmt.Errorf("no hosted zone for domain %q", domain)
}

func newRoute53() (*route53.Route53, error) {
	mu.Lock()
	auth, err := aws.GetAuth(awsAccess, awsSecret)
	mu.Unlock()

	if err != nil {
		return nil, err
	}

	return route53.New(auth, awsRegion), nil
}
