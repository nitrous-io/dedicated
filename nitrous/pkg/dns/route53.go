package dns

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/route53"
	"github.com/nitrous-io/dedicated/compass/navigator"
)

type Route53 struct {
	AWSAccessKey string
	AWSSecretKey string

	zonesCached []route53.HostedZone
	mu          sync.Mutex
}

// SetCNAME adds or updates a CNAME record that maps src into dst.
func (r *Route53) SetCNAME(src, dst string) error {
	cl, err := r.newRoute53()
	if err != nil {
		return err
	}

	zone, err := r.getHostedZone(cl, src)
	if err != nil {
		return err
	}

	return r.setDNS(cl, zone.ID, "CNAME", src, dst)
}

// DeleteCNAME deletes CNAME for a given domain.
func (r *Route53) DeleteCNAME(domain string) error {
	domain = route53.FQDN(domain)

	cl, err := r.newRoute53()
	if err != nil {
		return err
	}

	zone, err := r.getHostedZone(cl, domain)
	if err != nil {
		return err
	}

	// Get resource record for domain so we can delete it.
	// https://docs.aws.amazon.com/Route53/latest/APIReference/API_ListResourceRecordSets.html
	opts := route53.ListOpts{
		Name:     domain,
		Type:     "CNAME",
		MaxItems: 1, // only retrieve the name we want
	}
	resp, err := cl.ListResourceRecordSets(zone.ID, &opts)
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
		return ErrDomainDoesNotExist
	}
	if len(changes) > 1 {
		return fmt.Errorf("Expected one change for domain %s, got %d: %+v", domain, len(changes), changes)
	}

	fmt.Printf("[DEBUG] deleteCNAMERecord for domain %s: changes are %+v\n", domain, changes)
	req := &route53.ChangeResourceRecordSetsRequest{
		Comment: fmt.Sprintf("deleting CNAME record for %s", domain),
		Changes: changes,
	}
	if _, err := cl.ChangeResourceRecordSets(zone.ID, req); err != nil {
		return err
	}
	return nil
}

func (r *Route53) setDNS(cl *route53.Route53, zoneID, typ, from, to string) error {
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

	if _, err := cl.ChangeResourceRecordSets(zoneID, req); err != nil {
		fmt.Printf("ChangeResourceRecordSets() failed with %s\n", err)
		return err
	}

	return nil
}

func (r *Route53) cacheZones(cl *route53.Route53) error {
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

	r.mu.Lock()
	r.zonesCached = allZones
	r.mu.Unlock()

	return nil
}

func (r *Route53) getHostedZone(cl *route53.Route53, domain string) (*route53.HostedZone, error) {
	if r.zonesCached == nil {
		if err := r.cacheZones(cl); err != nil {
			return nil, err
		}
	}

	rootDomain := navigator.RootDomain(route53.FQDN(domain))

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, zone := range r.zonesCached {
		fqdn := route53.FQDN(zone.Name)
		if strings.EqualFold(fqdn, rootDomain) {
			return &zone, nil
		}
	}

	return nil, fmt.Errorf("no hosted zone for domain %q", domain)
}

func (r *Route53) newRoute53() (*route53.Route53, error) {
	auth, err := aws.GetAuth(r.AWSAccessKey, r.AWSSecretKey)
	if err != nil {
		return nil, err
	}
	return route53.New(auth, aws.USWest), nil
}
